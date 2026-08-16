package cloudflarekv_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/cloudflarekv"
	"github.com/dop251/goja_nodejs/cloudflarekv/store"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/streams"
)

func TestBindingEnforcesCloudflareShapeLimits(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	backend := newMemStore()
	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", backend); err != nil {
			return err
		}
		_, err := vm.RunString(`
			(async function () {
				const errors = [];
				for (const operation of [
					() => KV.put("x".repeat(513), "v"),
					() => KV.put("metadata", "v", {metadata: {v: "x".repeat(1024)}}),
					() => KV.put("ttl", "v", {expirationTtl: 59}),
					() => KV.list({limit: 1001}),
					() => KV.get(new Array(101).fill(0).map((_, i) => "k" + i))
				]) {
					try { await operation(); errors.push("accepted"); }
					catch (error) { errors.push(String(error)); }
				}
				done(JSON.stringify(errors));
			})().catch(function (err) { fail(String(err)); });
		`)
		return err
	})

	for _, marker := range []string{"512", "metadata", "60", "1000", "100"} {
		if !strings.Contains(result, marker) {
			t.Fatalf("limit result %s does not mention %q", result, marker)
		}
	}
}

func TestStreamingGetStopsAtConfiguredValueLimit(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	body := &trackingReadCloser{reader: bytes.NewReader([]byte("12345"))}
	backend := &streamingStore{memStore: newMemStore(), getBody: body}
	result := runScript(t, loop, func(vm *goja.Runtime) error {
		streams.Enable(vm)
		limits := cloudflarekv.CloudflareLimits()
		limits.MaxValueBytes = 4
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", backend, cloudflarekv.WithLimits(limits)); err != nil {
			return err
		}
		_, err := vm.RunString(`KV.get("key", "stream").then(async stream => {
			const reader = stream.getReader();
			while (!(await reader.read()).done) {}
			done("accepted");
		}).catch(error => done(String(error)));`)
		return err
	})
	if !strings.Contains(result, "4") {
		t.Fatalf("stream get limit error = %q", result)
	}
	if !body.isClosed() {
		t.Fatal("oversized stream body was not closed")
	}
}

func TestStreamingPutStopsAtConfiguredValueLimit(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	backend := &streamingStore{memStore: newMemStore()}
	result := runScript(t, loop, func(vm *goja.Runtime) error {
		streams.Enable(vm)
		limits := cloudflarekv.CloudflareLimits()
		limits.MaxValueBytes = 4
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", backend, cloudflarekv.WithLimits(limits)); err != nil {
			return err
		}
		_, err := vm.RunString(`
			const body = new ReadableStream({
				start(controller) {
					controller.enqueue(new Uint8Array([1, 2, 3]));
					controller.enqueue(new Uint8Array([4, 5]));
					controller.close();
				}
			});
			KV.put("key", body).then(
				() => fail("oversized stream was accepted"),
				error => done(String(error))
			);
		`)
		return err
	})

	if !strings.Contains(result, "4") {
		t.Fatalf("stream limit error = %q", result)
	}
}

func TestWriteRateLimitCanBeDisabledByGoHost(t *testing.T) {
	for _, tc := range []struct {
		name        string
		options     []cloudflarekv.BindOption
		wantLimited bool
	}{
		{name: "default", wantLimited: true},
		{name: "disabled", options: []cloudflarekv.BindOption{cloudflarekv.WithWriteRateLimit(0)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loop := eventloop.NewEventLoop()
			loop.Start()
			defer loop.Stop()
			backend := newMemStore()
			result := runScript(t, loop, func(vm *goja.Runtime) error {
				if err := cloudflarekv.BindNamespace(vm, loop, "KV", backend, tc.options...); err != nil {
					return err
				}
				_, err := vm.RunString(`
					KV.put("same", "one").then(() => KV.put("same", "two")).then(
						() => done("accepted"),
						error => done(String(error))
					);
				`)
				return err
			})
			limited := strings.Contains(result, "1 second")
			if limited != tc.wantLimited {
				t.Fatalf("result = %q, wantLimited = %v", result, tc.wantLimited)
			}
		})
	}
}

func TestGoStoreCallsBypassBindingLimits(t *testing.T) {
	backend := newMemStore()
	value := make([]byte, 25*1024*1024+1)
	if err := backend.Put(context.Background(), strings.Repeat("k", 513), value, store.PutOptions{
		ExpirationTTL: time.Second,
	}); err != nil {
		t.Fatalf("direct store put was limited: %v", err)
	}
}
