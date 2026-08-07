package fetch

import (
	"errors"
	"io"
	"sync"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/runtimehost"
	"github.com/sealdice/goja_ext/streams"
)

// streamingBody pumps an io.ReadCloser into chunks consumed by a canonical
// ReadableStream pull. Reads happen on a background goroutine; the pull
// callback (running on the loop thread) never blocks. The queue is bounded so
// long-lived streams (e.g. SSE) do not buffer unboundedly.
type streamingBody struct {
	scheduler runtimehost.Scheduler
	body      io.ReadCloser
	highWater int
	cleanup   func()
	cleanOnce sync.Once

	mu       sync.Mutex
	queue    [][]byte
	waiters  []func(interface{}) error
	done     bool
	err      error
	closed   bool
	started  bool
	more     chan struct{}
	closedCh chan struct{}
}

func newStreamingBody(scheduler runtimehost.Scheduler, body io.ReadCloser, cleanup func()) *streamingBody {
	return &streamingBody{
		scheduler: scheduler,
		body:      body,
		highWater: 16,
		cleanup:   cleanup,
		more:      make(chan struct{}, 1),
		closedCh:  make(chan struct{}),
	}
}

func (b *streamingBody) start() {
	b.mu.Lock()
	if b.started {
		b.mu.Unlock()
		return
	}
	b.started = true
	b.mu.Unlock()
	go b.pump()
}

func (b *streamingBody) pump() {
	for {
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			return
		}
		if len(b.queue) >= b.highWater {
			b.mu.Unlock()
			select {
			case <-b.more:
				continue
			case <-b.closedCh:
				return
			}
		}
		b.mu.Unlock()

		buf := make([]byte, 64*1024)
		n, err := b.body.Read(buf)

		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			b.mu.Lock()
			b.queue = append(b.queue, chunk)
			waiters := b.waiters
			b.waiters = nil
			b.mu.Unlock()
			b.wake(waiters, false)
		}

		if err != nil {
			b.mu.Lock()
			b.err = err
			b.done = true
			waiters := b.waiters
			b.waiters = nil
			b.mu.Unlock()
			b.wake(waiters, true)
			_ = b.body.Close()
			return
		}
	}
}

func (b *streamingBody) wake(waiters []func(interface{}) error, terminal bool) {
	if len(waiters) == 0 && !terminal {
		return
	}
	b.scheduler.RunOnLoop(func(*goja.Runtime) {
		for _, r := range waiters {
			_ = r(goja.Undefined())
		}
		if terminal {
			b.runCleanup()
		}
	})
}

func (b *streamingBody) runCleanup() {
	b.cleanOnce.Do(func() {
		if b.cleanup != nil {
			b.cleanup()
		}
	})
}

func (b *streamingBody) close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	body := b.body
	waiters := b.waiters
	b.waiters = nil
	b.mu.Unlock()
	close(b.closedCh)
	for _, r := range waiters {
		_ = r(goja.Undefined())
	}
	if body != nil {
		_ = body.Close()
	}
	b.runCleanup()
}

func (b *streamingBody) signalMore() {
	select {
	case b.more <- struct{}{}:
	default:
	}
}

// pull implements the ReadableStream pull callback: it never blocks the loop
// thread. It enqueues a queued chunk, closes/errors on EOF, or returns a
// promise that resolves once the background pump delivers data.
func (b *streamingBody) pull(rt *goja.Runtime, controller *goja.Object) goja.Value {
	b.mu.Lock()
	if len(b.queue) > 0 {
		chunk := b.queue[0]
		b.queue = b.queue[1:]
		b.mu.Unlock()
		b.signalMore()
		callFetchController(rt, controller, "enqueue", bytesValue(rt, chunk))
		return goja.Undefined()
	}
	if b.done {
		err := b.err
		b.mu.Unlock()
		b.runCleanup()
		if err != nil && !errors.Is(err, io.EOF) {
			callFetchController(rt, controller, "error", rt.NewGoError(err))
		} else {
			callFetchController(rt, controller, "close")
		}
		return goja.Undefined()
	}
	promise, resolve, _ := rt.NewPromise()
	b.waiters = append(b.waiters, resolve)
	b.mu.Unlock()
	return rt.ToValue(promise)
}

// fetchReadableStream builds a canonical ReadableStream that streams the given
// HTTP body. The pull path never blocks the loop thread.
func fetchReadableStream(rt *goja.Runtime, scheduler runtimehost.Scheduler, body io.ReadCloser, cleanup func()) goja.Value {
	b := newStreamingBody(scheduler, body, cleanup)
	b.start()
	stream, err := streams.NewReadableStream(rt, streams.ReadableStreamSource{
		Pull: func(controller *goja.Object) goja.Value {
			return b.pull(rt, controller)
		},
		Cancel: func(reason goja.Value) goja.Value {
			b.close()
			return goja.Undefined()
		},
	})
	if err != nil {
		panic(err)
	}
	return stream
}

// bufferedReadableStream builds a canonical ReadableStream over a fixed buffer
// (used by manually constructed Response objects).
func bufferedReadableStream(rt *goja.Runtime, data []byte) goja.Value {
	stream, err := streams.NewReadableStream(rt, streams.ReadableStreamSource{
		Pull: func(controller *goja.Object) goja.Value {
			callFetchController(rt, controller, "enqueue", bytesValue(rt, data))
			callFetchController(rt, controller, "close")
			return goja.Undefined()
		},
	})
	if err != nil {
		panic(err)
	}
	return stream
}

// consumeBody reads the entire canonical ReadableStream into bytes, then
// converts via convert. The returned value is a Promise.
func consumeBody(rt *goja.Runtime, stream goja.Value, convert func(*goja.Runtime, []byte) (goja.Value, error)) goja.Value {
	result, resolve, reject := rt.NewPromise()
	var out []byte
	consumed, err := streams.ConsumeReadableStream(rt, stream, func(chunk goja.Value) goja.Value {
		out = append(out, bytesFromValue(chunk)...)
		return goja.Undefined()
	})
	if err != nil {
		_ = reject(rt.NewTypeError(err.Error()))
		return rt.ToValue(result)
	}
	fetchThen(rt, rt.ToValue(consumed),
		func(goja.FunctionCall) goja.Value {
			value, convErr := convert(rt, out)
			if convErr != nil {
				_ = reject(rt.NewTypeError(convErr.Error()))
				return goja.Undefined()
			}
			_ = resolve(value)
			return goja.Undefined()
		},
		func(call goja.FunctionCall) goja.Value {
			_ = reject(call.Argument(0))
			return goja.Undefined()
		},
	)
	return rt.ToValue(result)
}

func callFetchController(rt *goja.Runtime, controller *goja.Object, name string, args ...goja.Value) {
	fn, ok := goja.AssertFunction(controller.Get(name))
	if !ok {
		panic(rt.NewTypeError("ReadableStream controller method %s is not callable", name))
	}
	if _, err := fn(controller, args...); err != nil {
		panic(err)
	}
}

func bytesValue(rt *goja.Runtime, data []byte) goja.Value {
	arrayBuffer := rt.NewArrayBuffer(append([]byte(nil), data...))
	typed, err := rt.New(rt.Get("Uint8Array"), rt.ToValue(arrayBuffer))
	if err != nil {
		panic(err)
	}
	return typed
}

func fetchThen(
	rt *goja.Runtime,
	value goja.Value,
	onFulfilled func(goja.FunctionCall) goja.Value,
	onRejected func(goja.FunctionCall) goja.Value,
) {
	object := value.ToObject(rt)
	method, ok := goja.AssertFunction(object.Get("then"))
	if !ok {
		panic(rt.NewTypeError("fetch: expected a Promise-compatible value"))
	}
	if _, err := method(object, rt.ToValue(onFulfilled), rt.ToValue(onRejected)); err != nil {
		panic(err)
	}
}
