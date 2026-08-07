package fetch

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/go-resty/resty/v2"
	"github.com/sealdice/goja_ext/eventloop"
)

const (
	eventSourceConnecting = 0
	eventSourceOpen       = 1
	eventSourceClosed     = 2

	defaultEventSourceRetry = 3000
)

type eventSourceData struct {
	rt         *goja.Runtime
	loop       *eventloop.EventLoop
	client     *resty.Client
	obj        *goja.Object
	url        string
	readyState int

	mu          sync.Mutex
	lastEventID string
	retry       int
	closed      bool
	body        io.ReadCloser
	cancel      context.CancelFunc
	timer       *eventloop.Timer
	listeners   map[string][]goja.Value

	// SSE parser state; only touched by the single pump goroutine.
	dataBuf  strings.Builder
	curEvent string
	hasEvent bool
}

func newEventSourceCtor(rt *goja.Runtime, loop *eventloop.EventLoop, client *resty.Client) func(call goja.ConstructorCall) *goja.Object {
	return func(call goja.ConstructorCall) *goja.Object {
		rawURL := ""
		if arg := call.Argument(0); arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
			rawURL = arg.String()
		}
		es := &eventSourceData{
			rt:         rt,
			loop:       loop,
			client:     client,
			url:        rawURL,
			readyState: eventSourceConnecting,
			retry:      defaultEventSourceRetry,
			listeners:  make(map[string][]goja.Value),
		}
		obj := call.This
		es.obj = obj
		_ = obj.Set("url", rawURL)
		_ = obj.Set("readyState", eventSourceConnecting)
		_ = obj.Set("withCredentials", false)
		_ = obj.Set("close", es.close)
		_ = obj.Set("addEventListener", es.addEventListener)
		_ = obj.Set("removeEventListener", es.removeEventListener)

		es.connect()
		return obj
	}
}

func (es *eventSourceData) setReadyState(state int) {
	es.mu.Lock()
	es.readyState = state
	es.mu.Unlock()
	_ = es.obj.Set("readyState", state)
}

func (es *eventSourceData) isClosed() bool {
	es.mu.Lock()
	defer es.mu.Unlock()
	return es.closed
}

func (es *eventSourceData) addEventListener(call goja.FunctionCall) goja.Value {
	eventType := call.Argument(0).String()
	fn := call.Argument(1)
	if _, ok := goja.AssertFunction(fn); !ok {
		return goja.Undefined()
	}
	es.mu.Lock()
	es.listeners[eventType] = append(es.listeners[eventType], fn)
	es.mu.Unlock()
	return goja.Undefined()
}

func (es *eventSourceData) removeEventListener(call goja.FunctionCall) goja.Value {
	eventType := call.Argument(0).String()
	target := call.Argument(1)
	es.mu.Lock()
	kept := es.listeners[eventType][:0]
	for _, l := range es.listeners[eventType] {
		if !l.SameAs(target) {
			kept = append(kept, l)
		}
	}
	if len(kept) == 0 {
		delete(es.listeners, eventType)
	} else {
		es.listeners[eventType] = kept
	}
	es.mu.Unlock()
	return goja.Undefined()
}

func (es *eventSourceData) close(goja.FunctionCall) goja.Value {
	es.mu.Lock()
	if es.closed {
		es.mu.Unlock()
		return goja.Undefined()
	}
	es.closed = true
	body := es.body
	cancel := es.cancel
	timer := es.timer
	es.body = nil
	es.cancel = nil
	es.timer = nil
	es.mu.Unlock()

	if body != nil {
		_ = body.Close()
	}
	if cancel != nil {
		cancel()
	}
	if timer != nil {
		es.loop.ClearTimeout(timer)
	}
	es.setReadyState(eventSourceClosed)
	return goja.Undefined()
}

// connect opens (or reopens) the SSE connection. Must be called on the loop.
func (es *eventSourceData) connect() {
	if es.isClosed() {
		return
	}
	es.setReadyState(eventSourceConnecting)

	ctx, cancel := context.WithCancel(context.Background())
	es.mu.Lock()
	es.cancel = cancel
	lastEventID := es.lastEventID
	es.mu.Unlock()

	go func() {
		req := es.client.R().SetContext(ctx).SetDoNotParseResponse(true)
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Cache-Control", "no-cache")
		if lastEventID != "" {
			req.Header.Set("Last-Event-ID", lastEventID)
		}
		resp, err := req.Get(es.url)

		es.loop.RunOnLoop(func(rt *goja.Runtime) {
			if es.isClosed() {
				if resp != nil && resp.RawBody() != nil {
					_ = resp.RawBody().Close()
				}
				if cancel != nil {
					cancel()
				}
				return
			}
			if err != nil || resp.StatusCode() != http.StatusOK {
				if resp != nil && resp.RawBody() != nil {
					_ = resp.RawBody().Close()
				}
				if cancel != nil {
					cancel()
				}
				es.dispatch(rt, "error", newSSEEventObject(rt, "error", "", ""))
				es.scheduleReconnect()
				return
			}
			es.setReadyState(eventSourceOpen)
			es.mu.Lock()
			es.body = resp.RawBody()
			es.mu.Unlock()
			es.dispatch(rt, "open", newSSEEventObject(rt, "open", "", ""))
			go es.ssePump(resp.RawBody())
		})
	}()
}

func (es *eventSourceData) scheduleReconnect() {
	es.mu.Lock()
	if es.closed || es.timer != nil {
		es.mu.Unlock()
		return
	}
	retry := es.retry
	es.mu.Unlock()
	es.timer = es.loop.SetTimeout(func(*goja.Runtime) {
		es.mu.Lock()
		es.timer = nil
		es.mu.Unlock()
		es.connect()
	}, time.Duration(retry)*time.Millisecond)
}

func (es *eventSourceData) ssePump(body io.ReadCloser) {
	parser := &sseLineBuffer{}
	buf := make([]byte, 64*1024)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			for _, line := range parser.feed(buf[:n]) {
				es.processLine(line)
			}
		}
		if err != nil {
			break
		}
	}
	// Flush any trailing partial line, then dispatch a final buffered event.
	if remaining := parser.remaining(); len(remaining) > 0 {
		es.processLine(string(remaining))
	}
	es.dispatchPending()
	_ = body.Close()
	es.loop.RunOnLoop(func(rt *goja.Runtime) {
		if es.isClosed() {
			return
		}
		es.dispatch(rt, "error", newSSEEventObject(rt, "error", "", ""))
		es.scheduleReconnect()
	})
}

func (es *eventSourceData) processLine(line string) {
	line = strings.TrimSuffix(line, "\r")
	if line == "" {
		es.dispatchPending()
		return
	}
	if strings.HasPrefix(line, ":") {
		return
	}
	field, value, found := strings.Cut(line, ":")
	if !found {
		field = line
		value = ""
	}
	value = strings.TrimPrefix(value, " ")
	switch field {
	case "data":
		es.dataBuf.WriteString(value)
		es.dataBuf.WriteByte('\n')
	case "event":
		es.curEvent = value
		es.hasEvent = true
	case "id":
		if !strings.ContainsRune(value, '\u0000') {
			es.mu.Lock()
			es.lastEventID = value
			es.mu.Unlock()
		}
	case "retry":
		if ms, err := strconv.Atoi(value); err == nil && ms > 0 {
			es.mu.Lock()
			es.retry = ms
			es.mu.Unlock()
		}
	}
}

func (es *eventSourceData) dispatchPending() {
	if es.dataBuf.Len() == 0 {
		es.curEvent = ""
		es.hasEvent = false
		return
	}
	data := strings.TrimSuffix(es.dataBuf.String(), "\n")
	eventType := es.curEvent
	if !es.hasEvent || eventType == "" {
		eventType = "message"
	}
	es.dataBuf.Reset()
	es.curEvent = ""
	es.hasEvent = false

	es.mu.Lock()
	lastEventID := es.lastEventID
	es.mu.Unlock()

	es.loop.RunOnLoop(func(rt *goja.Runtime) {
		if es.isClosed() {
			return
		}
		es.dispatch(rt, eventType, newSSEEventObject(rt, eventType, data, lastEventID))
	})
}

func (es *eventSourceData) dispatch(rt *goja.Runtime, eventType string, event *goja.Object) {
	es.mu.Lock()
	listeners := append([]goja.Value(nil), es.listeners[eventType]...)
	es.mu.Unlock()
	eventValue := rt.ToValue(event)
	for _, l := range listeners {
		if fn, ok := goja.AssertFunction(l); ok {
			_, _ = fn(goja.Undefined(), eventValue)
		}
	}
	prop := "on" + eventType
	if v := es.obj.Get(prop); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		if fn, ok := goja.AssertFunction(v); ok {
			_, _ = fn(goja.Undefined(), eventValue)
		}
	}
}

func newSSEEventObject(rt *goja.Runtime, eventType, data, lastEventID string) *goja.Object {
	obj := rt.NewObject()
	_ = obj.Set("type", eventType)
	if eventType != "open" && eventType != "error" {
		_ = obj.Set("data", data)
		_ = obj.Set("lastEventId", lastEventID)
		_ = obj.Set("origin", "")
	}
	return obj
}

// sseLineBuffer accumulates bytes and yields complete newline-terminated lines.
type sseLineBuffer struct {
	buf []byte
}

func (b *sseLineBuffer) feed(chunk []byte) []string {
	b.buf = append(b.buf, chunk...)
	var lines []string
	for {
		idx := bytes.IndexByte(b.buf, '\n')
		if idx < 0 {
			break
		}
		line := string(b.buf[:idx])
		b.buf = b.buf[idx+1:]
		lines = append(lines, line)
	}
	return lines
}

func (b *sseLineBuffer) remaining() []byte {
	out := b.buf
	b.buf = nil
	return out
}
