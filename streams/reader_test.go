package streams_test

import (
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/streams"
)

func newTestLoop(t *testing.T) *eventloop.EventLoop {
	t.Helper()
	loop := eventloop.NewEventLoop()
	loop.Start()
	t.Cleanup(func() { loop.Stop() })
	return loop
}

// runStreamsScript registers done/fail helpers, runs setup, then executes
// script on the loop and returns the value passed to done.
func runLoopScript(
	t *testing.T,
	loop *eventloop.EventLoop,
	setup func(vm *goja.Runtime) error,
	script string,
) string {
	t.Helper()
	result := make(chan string, 1)
	setupErr := make(chan error, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		_ = vm.Set("done", func(call goja.FunctionCall) goja.Value {
			select {
			case result <- call.Argument(0).String():
			default:
			}
			return goja.Undefined()
		})
		_ = vm.Set("fail", func(call goja.FunctionCall) goja.Value {
			select {
			case result <- "FAIL:" + call.Argument(0).String():
			default:
			}
			return goja.Undefined()
		})
		setupErr <- setup(vm)
	})
	if err := <-setupErr; err != nil {
		t.Fatal(err)
	}
	runErr := make(chan error, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		_, err := vm.RunString(script)
		runErr <- err
	})
	if err := <-runErr; err != nil {
		t.Fatal(err)
	}
	select {
	case out := <-result:
		return out
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for script result")
		return ""
	}
}

type countingReadCloser struct {
	reader     io.Reader
	readCalls  atomic.Int32
	closeCount atomic.Int32
	closeOnce  sync.Once
}

func (c *countingReadCloser) Read(p []byte) (int, error) {
	c.readCalls.Add(1)
	return c.reader.Read(p)
}

func (c *countingReadCloser) Close() error {
	c.closeCount.Add(1)
	c.closeOnce.Do(func() {})
	return nil
}

type stepRead struct {
	data []byte
	err  error
}

type failureReader struct{ err error }

func (r failureReader) Read([]byte) (int, error) { return 0, r.err }

// gatedReadCloser reports each read on readStarted, then waits for a step.
type gatedReadCloser struct {
	readStarted chan struct{}
	steps       chan stepRead
	closed      chan struct{}
	closeOnce   sync.Once
}

func newGatedReadCloser() *gatedReadCloser {
	return &gatedReadCloser{
		readStarted: make(chan struct{}, 8),
		steps:       make(chan stepRead),
		closed:      make(chan struct{}),
	}
}

func (g *gatedReadCloser) Read(p []byte) (int, error) {
	select {
	case g.readStarted <- struct{}{}:
	case <-g.closed:
		return 0, io.ErrClosedPipe
	}
	select {
	case step := <-g.steps:
		return copy(p, step.data), step.err
	case <-g.closed:
		return 0, io.ErrClosedPipe
	}
}

func (g *gatedReadCloser) Close() error {
	g.closeOnce.Do(func() { close(g.closed) })
	return nil
}

// eofWithDataReadCloser returns data and io.EOF from a single Read.
type eofWithDataReadCloser struct {
	data []byte
	done bool
}

func (r *eofWithDataReadCloser) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	return copy(p, r.data), io.EOF
}

func (r *eofWithDataReadCloser) Close() error { return nil }

func TestReaderStreamDeliversChunksAndCloses(t *testing.T) {
	loop := newTestLoop(t)
	body := &countingReadCloser{reader: strings.NewReader("hello")}
	settleErrs := make(chan error, 1)
	var settleCount atomic.Int32
	setup := func(vm *goja.Runtime) error {
		source, err := streams.NewReadableStreamFromReader(vm, loop, body,
			streams.WithOnSettled(func(_ *goja.Runtime, err error) {
				settleCount.Add(1)
				select {
				case settleErrs <- err:
				default:
				}
			}),
		)
		if err != nil {
			return err
		}
		return vm.Set("__s", source.Stream())
	}
	result := runLoopScript(t, loop, setup, `
		(async () => {
			const reader = __s.getReader();
			let text = "";
			while (true) {
				const item = await reader.read();
				if (item.done) break;
				text += String.fromCharCode(...item.value);
			}
			done(text);
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	if result != "hello" {
		t.Fatalf("streamed text = %q", result)
	}
	if got := body.closeCount.Load(); got != 1 {
		t.Fatalf("reader Close calls = %d, want 1", got)
	}
	select {
	case err := <-settleErrs:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("settle err = %v, want io.EOF", err)
		}
	case <-time.After(time.Second):
		t.Fatal("OnSettled was not called")
	}
	if got := settleCount.Load(); got != 1 {
		t.Fatalf("OnSettled calls = %d, want 1", got)
	}
}

func TestReaderStreamDeliversDataReturnedWithEOF(t *testing.T) {
	loop := newTestLoop(t)
	body := &eofWithDataReadCloser{data: []byte("last")}
	setup := func(vm *goja.Runtime) error {
		source, err := streams.NewReadableStreamFromReader(vm, loop, body)
		if err != nil {
			return err
		}
		return vm.Set("__s", source.Stream())
	}
	result := runLoopScript(t, loop, setup, `
		(async () => {
			const reader = __s.getReader();
			const first = await reader.read();
			const second = await reader.read();
			done(JSON.stringify([
				String.fromCharCode(...first.value), first.done, second.done,
			]));
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	if want := `["last",false,true]`; result != want {
		t.Fatalf("read sequence = %s, want %s", result, want)
	}
}

func TestReaderStreamMapsReadFailure(t *testing.T) {
	loop := newTestLoop(t)
	body := &countingReadCloser{reader: io.MultiReader(
		strings.NewReader("x"),
		failureReader{err: errors.New("boom")},
	)}
	setup := func(vm *goja.Runtime) error {
		source, err := streams.NewReadableStreamFromReader(vm, loop, body)
		if err != nil {
			return err
		}
		return vm.Set("__s", source.Stream())
	}
	result := runLoopScript(t, loop, setup, `
		(async () => {
			const reader = __s.getReader();
			const first = await reader.read();
			try {
				await reader.read();
				done("resolved");
			} catch (error) {
				done(JSON.stringify([
					error instanceof Error,
					String(error.message).includes("boom"),
					String.fromCharCode(...first.value),
				]));
			}
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	if want := `[true,true,"x"]`; result != want {
		t.Fatalf("error result = %s, want %s", result, want)
	}
	if got := body.closeCount.Load(); got != 1 {
		t.Fatalf("reader Close calls = %d, want 1", got)
	}
}

func TestReaderStreamDoesNotReadAheadOfConsumer(t *testing.T) {
	loop := newTestLoop(t)
	body := newGatedReadCloser()
	runErr := make(chan error, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		source, err := streams.NewReadableStreamFromReader(vm, loop, body)
		if err != nil {
			runErr <- err
			return
		}
		_ = vm.Set("__s", source.Stream())
		_, err = vm.RunString(`
			globalThis.__result = null
			globalThis.__readPending = false
			;(async () => {
				const reader = __s.getReader();
				const pending = reader.read();
				globalThis.__readPending = true;
				const item = await pending;
				const done = await reader.read();
				globalThis.__result = String.fromCharCode(...item.value) + "|" + done.done;
			})().catch((error) => {
				globalThis.__result = "ERR:" + String(error && error.stack || error)
			});
		`)
		runErr <- err
	})
	if err := <-runErr; err != nil {
		t.Fatal(err)
	}

	select {
	case <-body.readStarted:
	case <-time.After(time.Second):
		t.Fatal("first read did not start")
	}
	select {
	case <-body.readStarted:
		t.Fatal("bridge read ahead of consumer demand")
	case <-time.After(30 * time.Millisecond):
	}

	body.steps <- stepRead{data: []byte("a"), err: nil}
	body.steps <- stepRead{data: nil, err: io.EOF}
	waitGlobalTruthy(t, loop, "__result")
	if got := readGlobalString(t, loop, "__result"); got != "a|true" {
		t.Fatalf("result = %q, want %q", got, "a|true")
	}
}

func waitGlobalTruthy(t *testing.T, loop *eventloop.EventLoop, name string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		value := make(chan goja.Value, 1)
		loop.RunOnLoop(func(vm *goja.Runtime) { value <- vm.Get(name) })
		if v := <-value; v != nil && !goja.IsNull(v) && !goja.IsUndefined(v) && v.ToBoolean() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", name)
}

func readGlobalString(t *testing.T, loop *eventloop.EventLoop, name string) string {
	t.Helper()
	value := make(chan goja.Value, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) { value <- vm.Get(name) })
	return (<-value).String()
}

func TestReaderStreamCancelClosesReader(t *testing.T) {
	loop := newTestLoop(t)
	body := newGatedReadCloser()
	setup := func(vm *goja.Runtime) error {
		source, err := streams.NewReadableStreamFromReader(vm, loop, body)
		if err != nil {
			return err
		}
		return vm.Set("__s", source.Stream())
	}
	result := runLoopScript(t, loop, setup, `
		(async () => {
			const reader = __s.getReader();
			const pending = reader.read();
			await reader.cancel("stop");
			done("cancelled");
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	if result != "cancelled" {
		t.Fatalf("cancel result = %q", result)
	}
	select {
	case <-body.closed:
	case <-time.After(time.Second):
		t.Fatal("cancel did not close the reader")
	}
}

func TestReaderStreamErrorRejectsPendingReadWithExactReason(t *testing.T) {
	loop := newTestLoop(t)
	body := newGatedReadCloser()
	settledCount := atomic.Int32{}
	setup := func(vm *goja.Runtime) error {
		source, err := streams.NewReadableStreamFromReader(vm, loop, body,
			streams.WithOnSettled(func(*goja.Runtime, error) { settledCount.Add(1) }),
		)
		if err != nil {
			return err
		}
		if err := vm.Set("__error", func(call goja.FunctionCall) goja.Value {
			source.Error(vm, call.Argument(0))
			return goja.Undefined()
		}); err != nil {
			return err
		}
		return vm.Set("__s", source.Stream())
	}
	result := runLoopScript(t, loop, setup, `
		(async () => {
			const reason = { marker: "exact" };
			const pending = __s.getReader().read();
			globalThis.__readPending = true;
			__error(reason);
			try {
				await pending;
				done("resolved");
			} catch (error) {
				done(String(error === reason));
			}
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	if result != "true" {
		t.Fatalf("abort identity = %q", result)
	}
	select {
	case <-body.closed:
	case <-time.After(time.Second):
		t.Fatal("Error did not close the reader")
	}
	deadline := time.Now().Add(time.Second)
	for settledCount.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := settledCount.Load(); got != 1 {
		t.Fatalf("OnSettled calls = %d, want 1", got)
	}
}

func TestReaderStreamChunkSizeAndValueOptions(t *testing.T) {
	loop := newTestLoop(t)
	setup := func(vm *goja.Runtime) error {
		source, err := streams.NewReadableStreamFromReader(
			vm, loop, io.NopCloser(strings.NewReader("hello")),
			streams.WithChunkSize(2),
			streams.WithChunkValue(streams.ArrayBufferChunk),
		)
		if err != nil {
			return err
		}
		return vm.Set("__s", source.Stream())
	}
	result := runLoopScript(t, loop, setup, `
		(async () => {
			const reader = __s.getReader();
			const lengths = [];
			let isBuffer = true;
			while (true) {
				const item = await reader.read();
				if (item.done) break;
				isBuffer = isBuffer && item.value instanceof ArrayBuffer;
				lengths.push(item.value.byteLength);
			}
			done(lengths.join(",") + "|" + isBuffer);
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	if result != "2,2,1|true" {
		t.Fatalf("chunk result = %q", result)
	}
}

func TestReaderStreamNilSchedulerDeliversSynchronously(t *testing.T) {
	rt := goja.New()
	body := &countingReadCloser{reader: strings.NewReader("sync")}
	source, err := streams.NewReadableStreamFromReader(rt, nil, body)
	if err != nil {
		t.Fatal(err)
	}
	var consumed strings.Builder
	promise, err := streams.ConsumeReadableStream(rt, source.Stream(), func(chunk goja.Value) goja.Value {
		consumed.WriteString(string(chunk.Export().([]byte)))
		return goja.Undefined()
	})
	if err != nil {
		t.Fatal(err)
	}
	if promise.State() != goja.PromiseStateFulfilled {
		t.Fatalf("consume state = %v, result = %v", promise.State(), promise.Result())
	}
	if consumed.String() != "sync" {
		t.Fatalf("consumed = %q", consumed.String())
	}
	if got := body.closeCount.Load(); got != 1 {
		t.Fatalf("reader Close calls = %d, want 1", got)
	}
}

func TestReaderStreamSettlesOffLoopWhenLoopIsGone(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	body := newGatedReadCloser()
	offLoopErrs := make(chan error, 1)
	setupDone := make(chan error, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		_, err := streams.NewReadableStreamFromReader(vm, loop, body,
			streams.WithOnSettledOffLoop(func(err error) {
				select {
				case offLoopErrs <- err:
				default:
				}
			}),
		)
		_, _ = vm.RunString(`__s.getReader().read()`)
		setupDone <- err
	})
	if err := <-setupDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-body.readStarted:
	case <-time.After(time.Second):
		t.Fatal("read did not start")
	}

	loop.Terminate()
	body.steps <- stepRead{err: io.EOF}

	select {
	case err := <-offLoopErrs:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("off-loop settle err = %v, want io.EOF", err)
		}
	case <-time.After(time.Second):
		t.Fatal("off-loop settle hook was not called")
	}
	select {
	case <-body.closed:
	case <-time.After(time.Second):
		t.Fatal("loop-dead path did not close the reader")
	}
}

func TestReaderStreamConstructionFailureClosesReader(t *testing.T) {
	rt := goja.New()
	if err := streams.Exports(rt).Set("ReadableStream", 1); err != nil {
		t.Fatal(err)
	}
	body := &countingReadCloser{reader: strings.NewReader("x")}
	settled := make(chan struct{}, 1)
	_, err := streams.NewReadableStreamFromReader(rt, nil, body,
		streams.WithOnSettled(func(*goja.Runtime, error) {
			select {
			case settled <- struct{}{}:
			default:
			}
		}),
	)
	if err == nil {
		t.Fatal("construction unexpectedly succeeded")
	}
	if got := body.closeCount.Load(); got != 1 {
		t.Fatalf("reader Close calls = %d, want 1", got)
	}
	select {
	case <-settled:
	default:
		t.Fatal("OnSettled was not called on construction failure")
	}
}

func TestReaderFromBytesChunksCloseAndCancel(t *testing.T) {
	rt := goja.New()
	stream, err := streams.NewReadableStreamFromBytes(rt, []byte("hello"), 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Set("__s", stream); err != nil {
		t.Fatal(err)
	}
	value, err := rt.RunString(`
		(async () => {
			const reader = __s.getReader();
			const first = await reader.read();
			await reader.cancel("stop");
			return JSON.stringify([
				String.fromCharCode(...first.value),
				first.value instanceof Uint8Array,
				first.value.byteLength,
			]);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	promise := value.Export().(*goja.Promise)
	if promise.State() != goja.PromiseStateFulfilled {
		t.Fatalf("state = %v, result = %v", promise.State(), promise.Result())
	}
	if got, want := promise.Result().String(), `["he",true,2]`; got != want {
		t.Fatalf("result = %s, want %s", got, want)
	}
}
