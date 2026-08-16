package cloudflarekv_test

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/cloudflarekv"
	"github.com/dop251/goja_nodejs/cloudflarekv/store"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/streams"
)

type trackingReadCloser struct {
	reader io.Reader
	mu     sync.Mutex
	closed bool
}

func (r *trackingReadCloser) Read(p []byte) (int, error) { return r.reader.Read(p) }

func (r *trackingReadCloser) Close() error {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	return nil
}

func (r *trackingReadCloser) isClosed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}

type streamingStore struct {
	*memStore
	getBody        *trackingReadCloser
	streamGetCalls int
	streamPutCalls int
	streamPutValue []byte
}

func (s *streamingStore) GetStream(context.Context, string) (store.StreamRecord, bool, error) {
	s.streamGetCalls++
	return store.StreamRecord{Body: s.getBody, Size: -1}, true, nil
}

func (s *streamingStore) PutStream(
	_ context.Context,
	_ string,
	body io.Reader,
	_ store.PutOptions,
) error {
	s.streamPutCalls++
	value, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	s.streamPutValue = value
	return nil
}

var (
	_ store.StreamGetter = (*streamingStore)(nil)
	_ store.StreamPutter = (*streamingStore)(nil)
)

func TestReadableStreamGetUsesStreamGetterAndClosesOnCancel(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	body := &trackingReadCloser{reader: bytes.NewReader([]byte("streamed-value"))}
	backend := &streamingStore{memStore: newMemStore(), getBody: body}
	result := runScript(t, loop, func(vm *goja.Runtime) error {
		streams.Enable(vm)
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", backend); err != nil {
			return err
		}
		_, err := vm.RunString(`
			KV.get("key", "stream").then(async function (stream) {
				const reader = stream.getReader();
				const first = await reader.read();
				await reader.cancel("stop");
				done(new TextDecoder().decode(first.value));
			}).catch(function (err) { fail(String(err)); });
		`)
		return err
	})

	if result != "streamed-value" {
		t.Fatalf("stream result = %q", result)
	}
	if backend.streamGetCalls != 1 {
		t.Fatalf("GetStream calls = %d, want 1", backend.streamGetCalls)
	}
	if !body.isClosed() {
		t.Fatal("cancel did not close the storage reader")
	}
}

func TestReadableStreamPutUsesStreamPutter(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	backend := &streamingStore{memStore: newMemStore()}
	result := runScript(t, loop, func(vm *goja.Runtime) error {
		streams.Enable(vm)
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", backend); err != nil {
			return err
		}
		_, err := vm.RunString(`
			const body = new ReadableStream({
				start(controller) {
					controller.enqueue(new TextEncoder().encode("one-"));
					controller.enqueue(new TextEncoder().encode("two"));
					controller.close();
				}
			});
			KV.put("key", body).then(function () { done("ok"); })
				.catch(function (err) { fail(String(err)); });
		`)
		return err
	})

	if result != "ok" {
		t.Fatalf("put result = %q", result)
	}
	if backend.streamPutCalls != 1 {
		t.Fatalf("PutStream calls = %d, want 1", backend.streamPutCalls)
	}
	if string(backend.streamPutValue) != "one-two" {
		t.Fatalf("PutStream value = %q", backend.streamPutValue)
	}
}
