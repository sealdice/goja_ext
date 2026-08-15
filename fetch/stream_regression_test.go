package fetch //nolint:testpackage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/abort"
	"github.com/dop251/goja_nodejs/streams"
	"go.uber.org/goleak"
)

var errControlledBodyClosed = errors.New("controlled body closed")

type controlledRead struct {
	data []byte
	err  error
}

type controlledReadCloser struct {
	reads       chan controlledRead
	readStarted chan struct{}
	closed      chan struct{}
	closeOnce   sync.Once
	closeCount  atomic.Int32
}

func newControlledReadCloser(buffer int) *controlledReadCloser {
	return &controlledReadCloser{
		reads:       make(chan controlledRead, buffer),
		readStarted: make(chan struct{}, buffer+4),
		closed:      make(chan struct{}),
	}
}

func (b *controlledReadCloser) Read(p []byte) (int, error) {
	b.readStarted <- struct{}{}
	select {
	case step := <-b.reads:
		return copy(p, step.data), step.err
	case <-b.closed:
		return 0, errControlledBodyClosed
	}
}

func (b *controlledReadCloser) Close() error {
	b.closeCount.Add(1)
	b.closeOnce.Do(func() { close(b.closed) })
	return nil
}

type contextReadCloser struct {
	ctx         context.Context
	readStarted chan struct{}
	startedOnce sync.Once
	closeCount  atomic.Int32
}

func (b *contextReadCloser) Read([]byte) (int, error) {
	b.startedOnce.Do(func() { close(b.readStarted) })
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func (b *contextReadCloser) Close() error {
	b.closeCount.Add(1)
	return nil
}

type immediateScheduler struct {
	rt *goja.Runtime
}

func TestStreamingBodySchedulerRejectionStopsPump(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	rt := goja.New()
	body := newControlledReadCloser(1)
	body.reads <- controlledRead{data: []byte("chunk")}
	var loopCleanups atomic.Int32
	var offLoopCleanups atomic.Int32
	streamBody := newStreamingBody(
		rejectingScheduler{rt: rt},
		body,
		func() { loopCleanups.Add(1) },
		func() { offLoopCleanups.Add(1) },
	)
	_ = streamBody.pull(rt, rt.NewObject())
	exited := make(chan struct{})
	go func() {
		streamBody.pump()
		close(exited)
	}()

	select {
	case <-body.closed:
	case <-time.After(100 * time.Millisecond):
		streamBody.close()
		awaitSignal(t, exited, "pump exit after test cleanup")
		t.Fatal("response body remained open after scheduler rejected waiter wake")
	}
	awaitSignal(t, exited, "pump exit after scheduler rejection")
	if got := loopCleanups.Load(); got != 0 {
		t.Fatalf("loop cleanup called %d times off-loop", got)
	}
	if got := offLoopCleanups.Load(); got != 1 {
		t.Fatalf("off-loop cleanup called %d times", got)
	}
}

func TestStreamingBodyTerminalSchedulerRejectionRunsOffLoopCleanup(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	rt := goja.New()
	body := newControlledReadCloser(1)
	body.reads <- controlledRead{err: io.EOF}
	var loopCleanups atomic.Int32
	var offLoopCleanups atomic.Int32
	streamBody := newStreamingBody(
		rejectingScheduler{rt: rt},
		body,
		func() { loopCleanups.Add(1) },
		func() { offLoopCleanups.Add(1) },
	)
	exited := make(chan struct{})
	go func() {
		streamBody.pump()
		close(exited)
	}()
	awaitSignal(t, exited, "terminal pump exit after scheduler rejection")
	if got := body.closeCount.Load(); got != 1 {
		t.Fatalf("response body Close called %d times", got)
	}
	if got := loopCleanups.Load(); got != 0 {
		t.Fatalf("loop cleanup called %d times off-loop", got)
	}
	if got := offLoopCleanups.Load(); got != 1 {
		t.Fatalf("off-loop cleanup called %d times", got)
	}
}

func (s immediateScheduler) Runtime() *goja.Runtime {
	return s.rt
}

func (s immediateScheduler) RunOnLoop(fn func(*goja.Runtime)) bool {
	fn(s.rt)
	return true
}

func responseClient(body io.ReadCloser) *http.Client {
	return &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       body,
			Request:    request,
		}, nil
	})}
}

func TestFetchStreamDeliversBytesReturnedWithEOFThenStaysDone(t *testing.T) {
	body := newControlledReadCloser(1)

	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		Enable(rt)
		if err := EnableFetch(rt, loop, WithHTTPClient(responseClient(body))); err != nil {
			t.Fatal(err)
		}
		_, err := rt.RunString(`
			globalThis.__done = false
			;(async () => {
				const response = await fetch("http://stream.test/bytes-eof")
				const reader = response.body.getReader()
				const firstPending = reader.read()
				globalThis.__readPending = true
				const first = await firstPending
				const second = await reader.read()
				const third = await reader.read()
				globalThis.__result = JSON.stringify([
					String.fromCharCode(...first.value), first.done,
					second.done, third.done
				])
			})().catch((error) => {
				globalThis.__err = String(error && error.stack || error)
			}).finally(() => { globalThis.__done = true })
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__readPending")
	body.reads <- controlledRead{data: []byte("last"), err: io.EOF}
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__err"); got != "" {
		t.Fatalf("stream rejected: %s", got)
	}
	if got, want := gstr(t, loop, "__result"), `["last",false,true,true]`; got != want {
		t.Fatalf("read sequence = %s, want %s", got, want)
	}
	if got := body.closeCount.Load(); got != 1 {
		t.Fatalf("body Close called %d times", got)
	}
}

func TestFetchStreamDeliversBytesBeforeSeparateEOF(t *testing.T) {
	body := newControlledReadCloser(3)
	body.reads <- controlledRead{data: []byte("one")}
	body.reads <- controlledRead{data: []byte("two")}
	body.reads <- controlledRead{err: io.EOF}

	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		Enable(rt)
		if err := EnableFetch(rt, loop, WithHTTPClient(responseClient(body))); err != nil {
			t.Fatal(err)
		}
		_, err := rt.RunString(`
			globalThis.__done = false
			;(async () => {
				const reader = (await fetch("http://stream.test/separate-eof")).body.getReader()
				const values = []
				while (true) {
					const item = await reader.read()
					if (item.done) break
					values.push(String.fromCharCode(...item.value))
				}
				globalThis.__result = values.join("|")
			})().catch((error) => {
				globalThis.__err = String(error && error.stack || error)
			}).finally(() => { globalThis.__done = true })
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__err"); got != "" {
		t.Fatalf("stream rejected: %s", got)
	}
	if got := gstr(t, loop, "__result"); got != "one|two" {
		t.Fatalf("stream bytes = %q", got)
	}
	if got := body.closeCount.Load(); got != 1 {
		t.Fatalf("body Close called %d times", got)
	}
}

func TestFetchStreamWrapsBodyReadFailureAsNetworkError(t *testing.T) {
	body := newControlledReadCloser(1)
	body.reads <- controlledRead{err: errors.New("read exploded")}

	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		Enable(rt)
		if err := EnableFetch(rt, loop, WithHTTPClient(responseClient(body))); err != nil {
			t.Fatal(err)
		}
		_, err := rt.RunString(`
			globalThis.__done = false
			;(async () => {
				try {
					await (await fetch("http://stream.test/read-error")).text()
					globalThis.__result = "resolved"
				} catch (error) {
					globalThis.__result = JSON.stringify([
						error.name, error.code,
						String(error.message).includes("Network error"),
						String(error.cause).includes("read exploded")
					])
				}
			})().finally(() => { globalThis.__done = true })
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got, want := gstr(t, loop, "__result"), `["FetchError","NETWORK_ERROR",true,true]`; got != want {
		t.Fatalf("body error = %s, want %s", got, want)
	}
	if got := body.closeCount.Load(); got != 1 {
		t.Fatalf("body Close called %d times", got)
	}
}

func TestFetchStreamAbortPreservesExactReason(t *testing.T) {
	var body *contextReadCloser
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		body = &contextReadCloser{ctx: request.Context(), readStarted: make(chan struct{})}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       body,
			Request:    request,
		}, nil
	})}

	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		abort.Enable(rt)
		Enable(rt)
		if err := EnableFetch(rt, loop, WithHTTPClient(client)); err != nil {
			t.Fatal(err)
		}
		_, err := rt.RunString(`
			globalThis.__done = false
			;(async () => {
				const controller = new AbortController()
				const reason = { marker: "exact-abort-reason" }
				const response = await fetch("http://stream.test/abort", { signal: controller.signal })
				const pending = response.body.getReader().read()
				controller.abort(reason)
				try {
					await pending
					globalThis.__result = "resolved"
				} catch (error) {
					globalThis.__result = String(error === reason)
				}
			})().catch((error) => {
				globalThis.__err = String(error && error.stack || error)
			}).finally(() => { globalThis.__done = true })
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__err"); got != "" {
		t.Fatalf("fetch rejected early: %s", got)
	}
	if got := gstr(t, loop, "__result"); got != "true" {
		t.Fatalf("abort identity preserved = %q", got)
	}
	if body == nil {
		t.Fatal("transport did not provide response body")
	}
	if got := body.closeCount.Load(); got != 1 {
		t.Fatalf("body Close called %d times", got)
	}
}

func TestStreamingBodyBackpressureAndCancellation(t *testing.T) {
	t.Run("read error exits", func(t *testing.T) {
		body := newControlledReadCloser(1)
		body.reads <- controlledRead{err: errors.New("terminal read error")}
		var cleanups atomic.Int32
		streamBody := newStreamingBody(immediateScheduler{rt: goja.New()}, body, func() {
			cleanups.Add(1)
		}, nil)
		exited := make(chan struct{})
		go func() {
			streamBody.pump()
			close(exited)
		}()

		awaitSignal(t, exited, "pump exit")
		if got := body.closeCount.Load(); got != 1 {
			t.Fatalf("body Close called %d times", got)
		}
		if got := cleanups.Load(); got != 1 {
			t.Fatalf("cleanup called %d times", got)
		}
	})

	t.Run("blocked read", func(t *testing.T) {
		body := newControlledReadCloser(0)
		var cleanups atomic.Int32
		streamBody := newStreamingBody(immediateScheduler{rt: goja.New()}, body, func() {
			cleanups.Add(1)
		}, nil)
		exited := make(chan struct{})
		go func() {
			streamBody.pump()
			close(exited)
		}()
		awaitSignal(t, body.readStarted, "body Read start")

		streamBody.close()
		awaitSignal(t, exited, "pump exit")
		streamBody.close()

		if got := body.closeCount.Load(); got != 1 {
			t.Fatalf("body Close called %d times", got)
		}
		if got := cleanups.Load(); got != 1 {
			t.Fatalf("cleanup called %d times", got)
		}
	})

	t.Run("high water blocks and resumes", func(t *testing.T) {
		body := newControlledReadCloser(17)
		want := make([]byte, 17)
		for i := range want {
			want[i] = byte(i)
			body.reads <- controlledRead{data: []byte{byte(i)}}
		}
		var cleanups atomic.Int32
		streamBody := newStreamingBody(immediateScheduler{rt: goja.New()}, body, func() {
			cleanups.Add(1)
		}, nil)
		exited := make(chan struct{})
		go func() {
			streamBody.pump()
			close(exited)
		}()

		for i := range 16 {
			awaitSignal(t, body.readStarted, fmt.Sprintf("Read %d", i+1))
		}
		waitQueueLength(t, streamBody, 16)
		select {
		case <-body.readStarted:
			t.Fatal("pump read beyond highWater=16")
		case <-time.After(30 * time.Millisecond):
		}

		got := make([]byte, 0, len(want))
		for len(got) < len(want) {
			chunk := takeQueuedChunk(t, streamBody)
			got = append(got, chunk...)
			streamBody.signalMore()
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("resumed bytes = %v, want %v", got, want)
		}

		streamBody.close()
		awaitSignal(t, exited, "pump exit")
		if got := body.closeCount.Load(); got != 1 {
			t.Fatalf("body Close called %d times", got)
		}
		if got := cleanups.Load(); got != 1 {
			t.Fatalf("cleanup called %d times", got)
		}
	})

	t.Run("cancel while high water blocked", func(t *testing.T) {
		body := newControlledReadCloser(16)
		for i := range 16 {
			body.reads <- controlledRead{data: []byte{byte(i)}}
		}
		var cleanups atomic.Int32
		streamBody := newStreamingBody(immediateScheduler{rt: goja.New()}, body, func() {
			cleanups.Add(1)
		}, nil)
		exited := make(chan struct{})
		go func() {
			streamBody.pump()
			close(exited)
		}()
		for i := range 16 {
			awaitSignal(t, body.readStarted, fmt.Sprintf("Read %d", i+1))
		}
		waitQueueLength(t, streamBody, 16)

		streamBody.close()
		awaitSignal(t, exited, "pump exit")
		if got := body.closeCount.Load(); got != 1 {
			t.Fatalf("body Close called %d times", got)
		}
		if got := cleanups.Load(); got != 1 {
			t.Fatalf("cleanup called %d times", got)
		}
	})
}

func TestFetchStreamPullDoesNotBlockLoop(t *testing.T) {
	body := newControlledReadCloser(0)
	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		Enable(rt)
		if err := EnableFetch(rt, loop, WithHTTPClient(responseClient(body))); err != nil {
			t.Fatal(err)
		}
		_, err := rt.RunString(`
			globalThis.__done = false
			;(async () => {
				const reader = (await fetch("http://stream.test/nonblocking")).body.getReader()
				reader.read()
				globalThis.__loopResponsive = true
				await reader.cancel("test complete")
			})().catch((error) => {
				globalThis.__err = String(error && error.stack || error)
			}).finally(() => { globalThis.__done = true })
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__err"); got != "" {
		t.Fatalf("stream rejected: %s", got)
	}
	if got := gstr(t, loop, "__loopResponsive"); got != "true" {
		t.Fatalf("loop responsive = %q", got)
	}
	if got := body.closeCount.Load(); got != 1 {
		t.Fatalf("body Close called %d times", got)
	}
}

func TestFetchRejectsMalformedStructuralSignalsBeforeNetwork(t *testing.T) {
	tests := []struct {
		name   string
		signal string
	}{
		{name: "aborted is not boolean", signal: `{ aborted: 0, addEventListener() {} }`},
		{name: "addEventListener is missing", signal: `{ aborted: false }`},
		{name: "addEventListener is not callable", signal: `{ aborted: false, addEventListener: 1 }`},
		{name: "signal is primitive", signal: `1`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
				requests.Add(1)
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("unexpected")),
					Request:    request,
				}, nil
			})}
			loop := startFetchLoop(t)
			runSync(t, loop, func(rt *goja.Runtime) {
				Enable(rt)
				if err := EnableFetch(rt, loop, WithHTTPClient(client)); err != nil {
					t.Fatal(err)
				}
				_, err := rt.RunString(`
					globalThis.__done = false
					fetch("http://signal.test/", { signal: ` + test.signal + ` }).then(
						() => { globalThis.__result = "resolved" },
						(error) => { globalThis.__result = error && error.name }
					).finally(() => { globalThis.__done = true })
				`)
				if err != nil {
					t.Fatal(err)
				}
			})
			waitBool(t, loop, "__done")
			if got := gstr(t, loop, "__result"); got != "TypeError" {
				t.Fatalf("fetch result = %q", got)
			}
			if got := requests.Load(); got != 0 {
				t.Fatalf("malformed signal reached network %d times", got)
			}
		})
	}
}

func TestFetchAcceptsStructuralSignalWithoutRemoveEventListener(t *testing.T) {
	var requests atomic.Int32
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    request,
		}, nil
	})}
	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		Enable(rt)
		if err := EnableFetch(rt, loop, WithHTTPClient(client)); err != nil {
			t.Fatal(err)
		}
		_, err := rt.RunString(`
			globalThis.__done = false
			let additions = 0
			const signal = {
				aborted: false,
				addEventListener() { additions++ }
			}
			fetch("http://signal.test/valid", { signal }).then(
				(response) => response.text(),
				(error) => "ERR:" + String(error)
			).then((text) => {
				globalThis.__result = text + "|" + additions
			}).finally(() => { globalThis.__done = true })
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__result"); got != "ok|1" {
		t.Fatalf("fetch result = %q", got)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("valid structural signal reached network %d times", got)
	}
}

func TestFetchDispatchFailurePreservesNetworkErrorCause(t *testing.T) {
	wantErr := errors.New("dial exploded")
	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, wantErr
	})}
	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		Enable(rt)
		if err := EnableFetch(rt, loop, WithHTTPClient(client)); err != nil {
			t.Fatal(err)
		}
		_, err := rt.RunString(`
			globalThis.__done = false
			fetch("http://network.test/failure").then(
				() => { globalThis.__result = "resolved" },
				(error) => {
					globalThis.__result = JSON.stringify([
						error.name, error.code,
						String(error.message).includes("Network error"),
						String(error.cause).includes("dial exploded")
					])
				}
			).finally(() => { globalThis.__done = true })
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got, want := gstr(t, loop, "__result"), `["FetchError","NETWORK_ERROR",true,true]`; got != want {
		t.Fatalf("dispatch error = %s, want %s", got, want)
	}
}

func TestFetchStreamConstructionFailureRejectsAndClosesBody(t *testing.T) {
	body := newControlledReadCloser(0)
	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		Enable(rt)
		if err := EnableFetch(rt, loop, WithHTTPClient(responseClient(body))); err != nil {
			t.Fatal(err)
		}
		if err := streams.Exports(rt).Set("ReadableStream", 1); err != nil {
			t.Fatal(err)
		}
		_, err := rt.RunString(`
			globalThis.__done = false
			let additions = 0
			let removals = 0
			let listener
			const signal = {
				aborted: false,
				addEventListener(type, fn) { additions++; listener = fn },
				removeEventListener(type, fn) {
					if (type === "abort" && fn === listener) removals++
				}
			}
			fetch("http://stream.test/init-error", { signal }).then(
				() => { globalThis.__result = "resolved" },
				(error) => {
					globalThis.__result = JSON.stringify([
						error.name, error.code,
						String(error.cause).length > 0,
						additions, removals
					])
				}
			).finally(() => { globalThis.__done = true })
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got, want := gstr(t, loop, "__result"), `["FetchError","NETWORK_ERROR",true,1,1]`; got != want {
		t.Fatalf("stream init result = %s, want %s", got, want)
	}
	if got := body.closeCount.Load(); got != 1 {
		t.Fatalf("body Close called %d times", got)
	}
	if got := loop.GetPanicCount(); got != 0 {
		t.Fatalf("event loop panic count = %d", got)
	}
}

func awaitSignal(t *testing.T, ch <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for %s", label)
	}
}

func waitQueueLength(t *testing.T, body *streamingBody, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		body.mu.Lock()
		got := len(body.queue)
		body.mu.Unlock()
		if got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timeout waiting for queue length %d", want)
}

func takeQueuedChunk(t *testing.T, body *streamingBody) []byte {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		body.mu.Lock()
		if len(body.queue) > 0 {
			chunk := body.queue[0]
			body.queue = body.queue[1:]
			body.mu.Unlock()
			return chunk
		}
		body.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timeout waiting for queued chunk")
	return nil
}
