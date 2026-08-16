package fetch

import (
	"errors"
	"io"
	"sync"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/runtimehost"
	"github.com/dop251/goja_nodejs/streams"
)

const streamReadBufferSize = 64 * 1024

type streamReadBuffer [streamReadBufferSize]byte

var streamReadBufferPool = sync.Pool{
	New: func() any { return new(streamReadBuffer) },
}

// streamingBody pumps an io.ReadCloser into chunks consumed by a canonical
// ReadableStream pull. Reads happen on a background goroutine; the pull
// callback (running on the loop thread) never blocks. The queue is bounded so
// long-lived streams (e.g. SSE) do not buffer unboundedly.
type streamingBody struct {
	scheduler      runtimehost.Scheduler
	body           io.ReadCloser
	highWater      int
	cleanup        func()
	cleanupOffLoop func()
	cleanOnce      sync.Once
	closeOnce      sync.Once
	stopOnce       sync.Once

	mu              sync.Mutex
	queue           [][]byte
	waiters         []func(interface{}) error
	terminalWaiters []func(interface{}) error
	done            bool
	err             error
	closed          bool
	finalized       bool
	started         bool
	controller      *goja.Object
	more            chan struct{}
	closedCh        chan struct{}
}

func newStreamingBody(
	scheduler runtimehost.Scheduler,
	body io.ReadCloser,
	cleanup func(),
	cleanupOffLoop func(),
) *streamingBody {
	streaming := &streamingBody{
		scheduler: scheduler,
		body:      body,
		highWater: 16,
		cleanup:   cleanup,
		more:      make(chan struct{}, 1),
		closedCh:  make(chan struct{}),
	}
	streaming.cleanupOffLoop = cleanupOffLoop
	return streaming
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
	readBuffer := streamReadBufferPool.Get().(*streamReadBuffer)
	defer streamReadBufferPool.Put(readBuffer)

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

		n, err := b.body.Read(readBuffer[:])

		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			return
		}
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, readBuffer[:n])
			b.queue = append(b.queue, chunk)
		}

		if err != nil {
			b.err = err
			b.done = true
			b.terminalWaiters = append(b.terminalWaiters, b.waiters...)
			b.waiters = nil
			b.mu.Unlock()
			b.scheduleTerminal()
			b.closeBody()
			return
		}

		waiters := b.waiters
		b.waiters = nil
		b.mu.Unlock()
		if n > 0 {
			b.wake(waiters)
		}
	}
}

func (b *streamingBody) wake(waiters []func(interface{}) error) {
	if len(waiters) == 0 {
		return
	}
	if b.scheduler.RunOnLoop(func(*goja.Runtime) {
		for _, r := range waiters {
			_ = r(goja.Undefined())
		}
	}) {
		return
	}
	b.terminateOffLoop()
}

func (b *streamingBody) scheduleTerminal() {
	if b.scheduler.RunOnLoop(func(rt *goja.Runtime) {
		b.finishOnLoop(rt)
	}) {
		return
	}
	b.terminateOffLoop()
}

func (b *streamingBody) runCleanup() {
	b.cleanOnce.Do(func() {
		if b.cleanup != nil {
			b.cleanup()
		}
	})
}

func (b *streamingBody) runCleanupOffLoop() {
	b.cleanOnce.Do(func() {
		if b.cleanupOffLoop != nil {
			b.cleanupOffLoop()
		}
	})
}

func (b *streamingBody) terminateOffLoop() {
	b.mu.Lock()
	if b.finalized {
		b.mu.Unlock()
		return
	}
	b.finalized = true
	b.closed = true
	b.queue = nil
	b.waiters = nil
	b.terminalWaiters = nil
	b.mu.Unlock()
	b.stop()
	b.closeBody()
	b.runCleanupOffLoop()
}

func (b *streamingBody) close() {
	b.mu.Lock()
	if b.finalized {
		b.mu.Unlock()
		return
	}
	b.finalized = true
	b.closed = true
	b.queue = nil
	waiters := make([]func(interface{}) error, 0, len(b.waiters)+len(b.terminalWaiters))
	waiters = append(waiters, b.waiters...)
	waiters = append(waiters, b.terminalWaiters...)
	b.waiters = nil
	b.terminalWaiters = nil
	b.mu.Unlock()
	b.stop()
	for _, r := range waiters {
		_ = r(goja.Undefined())
	}
	b.closeBody()
	b.runCleanup()
}

func (b *streamingBody) abort(rt *goja.Runtime, reason goja.Value) {
	b.mu.Lock()
	if b.finalized {
		b.mu.Unlock()
		return
	}
	b.finalized = true
	b.closed = true
	b.queue = nil
	waiters := make([]func(interface{}) error, 0, len(b.waiters)+len(b.terminalWaiters))
	waiters = append(waiters, b.waiters...)
	waiters = append(waiters, b.terminalWaiters...)
	b.waiters = nil
	b.terminalWaiters = nil
	controller := b.controller
	b.mu.Unlock()

	b.stop()
	if controller != nil {
		callFetchController(rt, controller, "error", valueOrUndefined(reason))
	}
	for _, resolve := range waiters {
		_ = resolve(goja.Undefined())
	}
	b.closeBody()
	b.runCleanup()
}

func (b *streamingBody) finishOnLoop(rt *goja.Runtime) {
	b.mu.Lock()
	if b.finalized {
		b.mu.Unlock()
		return
	}
	b.finalized = true
	b.closed = true
	chunks := b.queue
	b.queue = nil
	waiters := make([]func(interface{}) error, 0, len(b.waiters)+len(b.terminalWaiters))
	waiters = append(waiters, b.waiters...)
	waiters = append(waiters, b.terminalWaiters...)
	b.waiters = nil
	b.terminalWaiters = nil
	err := b.err
	controller := b.controller
	b.mu.Unlock()

	b.stop()
	b.closeBody()
	b.runCleanup()
	if controller != nil {
		if err != nil && !errors.Is(err, io.EOF) {
			cause := rt.NewGoError(err)
			callFetchController(rt, controller, "error", fetchNetworkError(rt, cause))
		} else {
			for _, chunk := range chunks {
				callFetchController(rt, controller, "enqueue", bytesValue(rt, chunk))
			}
			callFetchController(rt, controller, "close")
		}
	}
	for _, resolve := range waiters {
		_ = resolve(goja.Undefined())
	}
}

func fetchNetworkError(rt *goja.Runtime, cause goja.Value) goja.Value {
	fetchError := Exports(rt).Get("_FetchError").ToObject(rt)
	networkError, ok := goja.AssertFunction(fetchError.Get("NETWORK_ERROR"))
	if !ok {
		return cause
	}
	value, err := networkError(fetchError, rt.ToValue("Network error"), cause)
	if err != nil {
		return cause
	}
	return value
}

func (b *streamingBody) stop() {
	b.stopOnce.Do(func() { close(b.closedCh) })
}

func (b *streamingBody) closeBody() {
	b.closeOnce.Do(func() {
		if b.body != nil {
			_ = b.body.Close()
		}
	})
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
		b.mu.Unlock()
		b.finishOnLoop(rt)
		return goja.Undefined()
	}
	promise, resolve, _ := rt.NewPromise()
	b.waiters = append(b.waiters, resolve)
	b.mu.Unlock()
	return rt.ToValue(promise)
}

// fetchReadableStream builds a canonical ReadableStream that streams the given
// HTTP body. The pull path never blocks the loop thread.
func fetchReadableStream(
	rt *goja.Runtime,
	scheduler runtimehost.Scheduler,
	body io.ReadCloser,
	cleanup func(),
	cleanupOffLoop func(),
	abort *dispatchAbortState,
) (*goja.Object, error) {
	b := newStreamingBody(scheduler, body, cleanup, cleanupOffLoop)
	stream, err := streams.NewReadableStream(rt, streams.ReadableStreamSource{
		Start: func(controller *goja.Object) goja.Value {
			b.mu.Lock()
			b.controller = controller
			b.mu.Unlock()
			b.start()
			return goja.Undefined()
		},
		Pull: func(controller *goja.Object) goja.Value {
			return b.pull(rt, controller)
		},
		Cancel: func(reason goja.Value) goja.Value {
			b.close()
			return goja.Undefined()
		},
	})
	if err != nil {
		b.close()
		return nil, err
	}
	if abort != nil {
		abort.setHandler(func(reason goja.Value) {
			b.abort(rt, reason)
		})
	}
	return stream, nil
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
	// NewArrayBuffer keeps data as its backing store; callers transfer ownership.
	arrayBuffer := rt.NewArrayBuffer(data)
	typed, err := rt.New(rt.Get("Uint8Array"), rt.ToValue(arrayBuffer))
	if err != nil {
		panic(err)
	}
	return typed
}
