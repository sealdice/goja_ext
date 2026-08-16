package cloudflarekv_test

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/cloudflarekv"
	"github.com/dop251/goja_nodejs/cloudflarekv/store"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/streams"
)

type countingStore struct {
	*memStore
	mu       sync.Mutex
	getCalls int
}

func (backend *countingStore) Get(ctx context.Context, key string) (store.Record, bool, error) {
	backend.mu.Lock()
	backend.getCalls++
	backend.mu.Unlock()
	return backend.memStore.Get(ctx, key)
}

func (backend *countingStore) calls() int {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.getCalls
}

func TestBindingCachesPresentAndMissingValues(t *testing.T) {
	for _, found := range []bool{true, false} {
		t.Run(map[bool]string{true: "present", false: "missing"}[found], func(t *testing.T) {
			loop := eventloop.NewEventLoop()
			loop.Start()
			defer loop.Stop()
			backend := &countingStore{memStore: newMemStore()}
			if found {
				_ = backend.Put(context.Background(), "key", []byte("value"), store.PutOptions{})
			}

			runScript(t, loop, func(vm *goja.Runtime) error {
				if err := cloudflarekv.BindNamespace(vm, loop, "KV", backend); err != nil {
					return err
				}
				_, err := vm.RunString(`KV.get("key").then(() => KV.get("key"))
					.then(value => done(String(value)))
					.catch(error => fail(String(error)));`)
				return err
			})
			if backend.calls() != 1 {
				t.Fatalf("Get calls = %d, want 1", backend.calls())
			}
		})
	}
}

func TestBindingCacheCanBeDisabledAndRejectsShortCacheTTL(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()
	backend := &countingStore{memStore: newMemStore()}
	_ = backend.Put(context.Background(), "key", []byte("value"), store.PutOptions{})

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", backend, cloudflarekv.WithCacheCapacity(0)); err != nil {
			return err
		}
		_, err := vm.RunString(`KV.get("key").then(() => KV.get("key", {cacheTtl: 29})).then(
			() => fail("short cacheTtl accepted"), error => done(String(error)));`)
		return err
	})
	if backend.calls() != 1 {
		t.Fatalf("Get calls before validation = %d, want 1", backend.calls())
	}
	if !strings.Contains(result, "30") {
		t.Fatalf("cacheTtl error = %q", result)
	}

	runScript(t, loop, func(vm *goja.Runtime) error {
		_, err := vm.RunString(`KV.get("key").then(() => done("ok")).catch(error => fail(String(error)));`)
		return err
	})
	if backend.calls() != 2 {
		t.Fatalf("Get calls with disabled cache = %d, want 2", backend.calls())
	}
}

func TestPutInvalidatesBindingCache(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()
	backend := &countingStore{memStore: newMemStore()}
	_ = backend.Put(context.Background(), "key", []byte("old"), store.PutOptions{})

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", backend); err != nil {
			return err
		}
		_, err := vm.RunString(`KV.get("key").then(() => KV.put("key", "new"))
			.then(() => KV.get("key")).then(done).catch(error => fail(String(error)));`)
		return err
	})
	if result != "new" || backend.calls() != 2 {
		t.Fatalf("result = %q, Get calls = %d", result, backend.calls())
	}
}

func TestCompletedStreamReadPopulatesBindingCache(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()
	body := &trackingReadCloser{reader: bytes.NewReader([]byte("streamed"))}
	backend := &streamingStore{memStore: newMemStore(), getBody: body}

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		streams.Enable(vm)
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", backend); err != nil {
			return err
		}
		_, err := vm.RunString(`KV.get("key", "stream").then(async stream => {
			const reader = stream.getReader();
			while (!(await reader.read()).done) {}
		}).then(() => KV.get("key")).then(done).catch(error => fail(String(error)));`)
		return err
	})
	if result != "streamed" || backend.streamGetCalls != 1 {
		t.Fatalf("result = %q, stream Get calls = %d", result, backend.streamGetCalls)
	}
}
