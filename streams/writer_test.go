package streams_test

import (
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/streams"
)

type recordingWriter struct {
	mu         sync.Mutex
	builder    strings.Builder
	failWrites bool
	closeCount atomic.Int32
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failWrites {
		return 0, errors.New("write exploded")
	}
	return w.builder.Write(p)
}

func (w *recordingWriter) Close() error {
	w.closeCount.Add(1)
	return nil
}

func (w *recordingWriter) written() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.builder.String()
}

var _ io.WriteCloser = (*recordingWriter)(nil)

func TestWriterStreamWritesChunksAndCloses(t *testing.T) {
	loop := newTestLoop(t)
	writer := &recordingWriter{}
	setup := func(vm *goja.Runtime) error {
		stream, err := streams.NewWritableStreamToWriter(vm, loop, writer)
		if err != nil {
			return err
		}
		return vm.Set("__w", stream)
	}
	result := runLoopScript(t, loop, setup, `
		(async () => {
			const writer = __w.getWriter();
			await writer.write(new Uint8Array([65]));
			await writer.write(new Uint8Array([66]));
			await writer.close();
			done("closed");
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	if result != "closed" {
		t.Fatalf("write result = %q", result)
	}
	if got := writer.written(); got != "AB" {
		t.Fatalf("written = %q", got)
	}
	if got := writer.closeCount.Load(); got != 1 {
		t.Fatalf("writer Close calls = %d, want 1", got)
	}
}

func TestWriterStreamRejectsInvalidChunkAndCloses(t *testing.T) {
	loop := newTestLoop(t)
	writer := &recordingWriter{}
	setup := func(vm *goja.Runtime) error {
		stream, err := streams.NewWritableStreamToWriter(vm, loop, writer)
		if err != nil {
			return err
		}
		return vm.Set("__w", stream)
	}
	result := runLoopScript(t, loop, setup, `
		(async () => {
			try {
				await __w.getWriter().write(42);
				done("resolved");
			} catch (error) {
				done(String(error instanceof TypeError));
			}
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	if result != "true" {
		t.Fatalf("invalid chunk result = %q", result)
	}
	if got := writer.closeCount.Load(); got != 1 {
		t.Fatalf("writer Close calls = %d, want 1", got)
	}
}

func TestWriterStreamRejectsWriteFailureAndCloses(t *testing.T) {
	loop := newTestLoop(t)
	writer := &recordingWriter{failWrites: true}
	setup := func(vm *goja.Runtime) error {
		stream, err := streams.NewWritableStreamToWriter(vm, loop, writer)
		if err != nil {
			return err
		}
		return vm.Set("__w", stream)
	}
	result := runLoopScript(t, loop, setup, `
		(async () => {
			try {
				await __w.getWriter().write(new Uint8Array([65]));
				done("resolved");
			} catch (error) {
				done(String(error.message).includes("write exploded"));
			}
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	if result != "true" {
		t.Fatalf("write failure result = %q", result)
	}
	if got := writer.closeCount.Load(); got != 1 {
		t.Fatalf("writer Close calls = %d, want 1", got)
	}
}

func TestWriterStreamAbortCloses(t *testing.T) {
	loop := newTestLoop(t)
	writer := &recordingWriter{}
	setup := func(vm *goja.Runtime) error {
		stream, err := streams.NewWritableStreamToWriter(vm, loop, writer)
		if err != nil {
			return err
		}
		return vm.Set("__w", stream)
	}
	result := runLoopScript(t, loop, setup, `
		__w.getWriter().abort("stop").then(() => done("aborted"));
	`)
	if result != "aborted" {
		t.Fatalf("abort result = %q", result)
	}
	if got := writer.closeCount.Load(); got != 1 {
		t.Fatalf("writer Close calls = %d, want 1", got)
	}
}

func TestWriterStreamNilScheduler(t *testing.T) {
	rt := goja.New()
	writer := &recordingWriter{}
	stream, err := streams.NewWritableStreamToWriter(rt, nil, writer)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Set("__w", stream); err != nil {
		t.Fatal(err)
	}
	value, err := rt.RunString(`
		(function () {
			const writer = __w.getWriter();
			return writer.write(new Uint8Array([65]))
				.then(function () { return writer.write(new Uint8Array([66])); })
				.then(function () { return writer.close(); });
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	promise := value.Export().(*goja.Promise)
	if promise.State() != goja.PromiseStateFulfilled {
		t.Fatalf("state = %v, result = %v", promise.State(), promise.Result())
	}
	if got := writer.written(); got != "AB" {
		t.Fatalf("written = %q", got)
	}
}
