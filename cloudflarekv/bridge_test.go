package cloudflarekv_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/cloudflarekv"
	"github.com/dop251/goja_nodejs/cloudflarekv/store"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/streams"
)

func TestBindNamespaceObjectDoesNotCreateGlobalBinding(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()
	result := runScript(t, loop, func(vm *goja.Runtime) error {
		target := vm.NewObject()
		if err := cloudflarekv.BindNamespaceObject(vm, loop, target, mem); err != nil {
			return err
		}
		if err := cloudflarekv.BindSyncNamespaceObject(vm, target, mem); err != nil {
			return err
		}
		if vm.Get("KV") != nil && !goja.IsUndefined(vm.Get("KV")) {
			return errors.New("unexpected global KV binding")
		}
		if err := target.Set("marker", "ok"); err != nil {
			return err
		}
		if err := vm.Set("storageTarget", target); err != nil {
			return err
		}
		_, err := vm.RunString(`done(storageTarget.marker)`)
		return err
	})

	if result != "ok" {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestBindNamespaceSupportsJSONListAndMetadata(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()
	fixed := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	mem.now = func() time.Time { return fixed }

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}

		_, err := vm.RunString(`
			KV.put("user:1", JSON.stringify({name: "Ada"}), {metadata: {role: "admin"}})
				.then(function () {
					return KV.get("user:1", "json");
				})
				.then(function (value) {
					return KV.getWithMetadata("user:1", "json").then(function (withMetadata) {
						return {
							value: value,
							withMetadata: withMetadata
						};
					});
				})
				.then(function (payload) {
					return KV.list({prefix: "user:"}).then(function (page) {
						payload.page = page;
						done(JSON.stringify(payload));
					});
				})
				.catch(function (err) {
					fail(String(err));
				});
		`)
		return err
	})

	var payload struct {
		Value struct {
			Name string `json:"name"`
		} `json:"value"`
		WithMetadata struct {
			Metadata map[string]any `json:"metadata"`
		} `json:"withMetadata"`
		Page struct {
			Keys []struct {
				Name     string         `json:"name"`
				Metadata map[string]any `json:"metadata"`
			} `json:"keys"`
			ListComplete bool `json:"list_complete"`
		} `json:"page"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("unmarshal JS result: %v", err)
	}

	if payload.Value.Name != "Ada" {
		t.Fatalf("unexpected JSON value: %#v", payload.Value)
	}
	if payload.WithMetadata.Metadata["role"] != "admin" {
		t.Fatalf("unexpected metadata: %#v", payload.WithMetadata.Metadata)
	}
	if len(payload.Page.Keys) != 1 || payload.Page.Keys[0].Name != "user:1" {
		t.Fatalf("unexpected list payload: %#v", payload.Page)
	}
	if payload.Page.Keys[0].Metadata["role"] != "admin" {
		t.Fatalf("unexpected list metadata: %#v", payload.Page.Keys[0].Metadata)
	}
	if !payload.Page.ListComplete {
		t.Fatalf("expected single-page list result, got %#v", payload.Page)
	}
}

func TestBindNamespaceObjectsSupportNestedAsyncAndSyncAccessWithoutGlobals(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()
	result := runScript(t, loop, func(vm *goja.Runtime) error {
		storage := vm.NewObject()
		kv := vm.NewObject()
		if err := cloudflarekv.BindNamespaceObject(vm, loop, kv, mem); err != nil {
			return err
		}
		syncKV := vm.NewObject()
		if err := cloudflarekv.BindSyncNamespaceObject(vm, syncKV, mem); err != nil {
			return err
		}
		if err := storage.Set("kv", kv); err != nil {
			return err
		}
		if err := storage.Set("synckv", syncKV); err != nil {
			return err
		}
		if err := vm.Set("storage", storage); err != nil {
			return err
		}

		_, err := vm.RunString(`
			if (typeof KV !== "undefined" || typeof SyncKV !== "undefined") {
				throw new Error("namespace object binding leaked a global");
			}
			storage.synckv.put("sync", "sync-value");
			storage.kv.put("async", "async-value")
				.then(function () {
					return storage.kv.get("async");
				})
				.then(function (asyncValue) {
					done(JSON.stringify({
						asyncValue: asyncValue,
						syncValue: storage.synckv.get("sync")
					}));
				})
				.catch(function (err) {
					fail(String(err));
				});
		`)
		return err
	})

	if result != `{"asyncValue":"async-value","syncValue":"sync-value"}` {
		t.Fatalf("unexpected nested storage result: %s", result)
	}
}

func TestBindNamespaceSupportsTypedArrays(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()
	fixed := time.Date(2026, 7, 6, 13, 0, 0, 0, time.UTC)
	mem.now = func() time.Time { return fixed }

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}

		_, err := vm.RunString(`
			var bytes = new Uint8Array([1, 2, 3, 4]);

			KV.put("bin", bytes, {metadata: {kind: "binary"}})
				.then(function () {
					return KV.getWithMetadata("bin", "arrayBuffer");
				})
				.then(function (result) {
					var view = new Uint8Array(result.value);
					var parts = [];
					for (var i = 0; i < view.length; i++) {
						parts.push(String(view[i]));
					}
					done(JSON.stringify({
						bytes: parts.join(","),
						metadata: result.metadata
					}));
				})
				.catch(function (err) {
					fail(String(err));
				});
		`)
		return err
	})

	var payload struct {
		Bytes    string         `json:"bytes"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("unmarshal JS bytes result: %v", err)
	}
	if payload.Bytes != "1,2,3,4" {
		t.Fatalf("unexpected byte payload: %#v", payload)
	}
	if payload.Metadata["kind"] != "binary" {
		t.Fatalf("unexpected binary metadata: %#v", payload.Metadata)
	}
}

func TestInstallConstructorIsolatesNamespaces(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	alpha := newMemStore()
	beta := newMemStore()
	fixed := time.Date(2026, 7, 6, 14, 0, 0, 0, time.UTC)
	alpha.now = func() time.Time { return fixed }
	beta.now = func() time.Time { return fixed }

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		resolver := func(namespace string) (store.NamespaceStore, error) {
			switch namespace {
			case "alpha":
				return alpha, nil
			case "beta":
				return beta, nil
			default:
				return nil, errors.New("unknown namespace: " + namespace)
			}
		}

		if err := cloudflarekv.InstallConstructor(vm, loop, resolver); err != nil {
			return err
		}

		_, err := vm.RunString(`
			var left = new KVNamespace("alpha");
			var right = new KVNamespace("beta");

			left.put("shared", "left")
				.then(function () {
					return right.put("shared", "right");
				})
				.then(function () {
					return left.get("shared");
				})
				.then(function (leftValue) {
					return right.get("shared").then(function (rightValue) {
						done(leftValue + "|" + rightValue);
					});
				})
				.catch(function (err) {
					fail(String(err));
				});
		`)
		return err
	})

	if result != "left|right" {
		t.Fatalf("unexpected constructor isolation result: %q", result)
	}
}

func TestPutRejectsConflictingExpirationOptions(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}

		_, err := vm.RunString(`
			KV.put("bad", "value", {expiration: 1700000000, expirationTtl: 60})
				.then(function () {
					fail("expected put to reject");
				}, function (err) {
					done(String(err));
				});
		`)
		return err
	})

	if !strings.Contains(result, "expiration") {
		t.Fatalf("expected expiration validation error, got %q", result)
	}
}

func TestPutAcceptsExpirationTTLNumbers(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()
	fixed := time.Date(2026, 7, 6, 16, 0, 0, 0, time.UTC)
	mem.now = func() time.Time { return fixed }

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}

		_, err := vm.RunString(`
			KV.put("ttl", "value", {expirationTtl: 60})
				.then(function () {
					return KV.get("ttl");
				})
				.then(function (value) {
					done(String(value));
				})
				.catch(function (err) {
					fail(String(err));
				});
		`)
		return err
	})

	if result != "value" {
		t.Fatalf("unexpected TTL result: %q", result)
	}
}

func TestBindSyncNamespaceSupportsJSONListAndMetadata(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()
	fixed := time.Date(2026, 7, 6, 17, 0, 0, 0, time.UTC)
	mem.now = func() time.Time { return fixed }

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindSyncNamespace(vm, "SyncKV", mem); err != nil {
			return err
		}

		_, err := vm.RunString(`
			SyncKV.put("user:2", JSON.stringify({name: "Grace"}), {metadata: {role: "editor"}});
			var value = SyncKV.get("user:2", "json");
			var withMetadata = SyncKV.getWithMetadata("user:2", "json");
			var page = SyncKV.list({prefix: "user:"});

			done(JSON.stringify({
				value: value,
				withMetadata: withMetadata,
				page: page
			}));
		`)
		return err
	})

	var payload struct {
		Value struct {
			Name string `json:"name"`
		} `json:"value"`
		WithMetadata struct {
			Metadata map[string]any `json:"metadata"`
		} `json:"withMetadata"`
		Page struct {
			Keys []struct {
				Name     string         `json:"name"`
				Metadata map[string]any `json:"metadata"`
			} `json:"keys"`
			ListComplete bool `json:"list_complete"`
		} `json:"page"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("unmarshal sync JS result: %v", err)
	}

	if payload.Value.Name != "Grace" {
		t.Fatalf("unexpected sync JSON value: %#v", payload.Value)
	}
	if payload.WithMetadata.Metadata["role"] != "editor" {
		t.Fatalf("unexpected sync metadata: %#v", payload.WithMetadata.Metadata)
	}
	if len(payload.Page.Keys) != 1 || payload.Page.Keys[0].Name != "user:2" {
		t.Fatalf("unexpected sync list payload: %#v", payload.Page)
	}
	if payload.Page.Keys[0].Metadata["role"] != "editor" {
		t.Fatalf("unexpected sync list metadata: %#v", payload.Page.Keys[0].Metadata)
	}
	if !payload.Page.ListComplete {
		t.Fatalf("expected single-page sync list result, got %#v", payload.Page)
	}
}

func TestBindSyncNamespaceSupportsTypedArrays(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()
	fixed := time.Date(2026, 7, 6, 18, 0, 0, 0, time.UTC)
	mem.now = func() time.Time { return fixed }

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindSyncNamespace(vm, "SyncKV", mem); err != nil {
			return err
		}

		_, err := vm.RunString(`
			var bytes = new Uint8Array([5, 6, 7, 8]);
			SyncKV.put("bin-sync", bytes, {metadata: {kind: "binary"}});
			var result = SyncKV.getWithMetadata("bin-sync", "arrayBuffer");
			var view = new Uint8Array(result.value);
			var parts = [];
			for (var i = 0; i < view.length; i++) {
				parts.push(String(view[i]));
			}

			done(JSON.stringify({
				bytes: parts.join(","),
				metadata: result.metadata
			}));
		`)
		return err
	})

	var payload struct {
		Bytes    string         `json:"bytes"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("unmarshal sync JS bytes result: %v", err)
	}
	if payload.Bytes != "5,6,7,8" {
		t.Fatalf("unexpected sync byte payload: %#v", payload)
	}
	if payload.Metadata["kind"] != "binary" {
		t.Fatalf("unexpected sync binary metadata: %#v", payload.Metadata)
	}
}

func TestBindSyncNamespaceThrowsForConflictingExpirationOptions(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindSyncNamespace(vm, "SyncKV", mem); err != nil {
			return err
		}

		_, err := vm.RunString(`
			try {
				SyncKV.put("bad-sync", "value", {expiration: 1700000000, expirationTtl: 60});
				fail("expected SyncKV.put to throw");
			} catch (err) {
				done(String(err));
			}
		`)
		return err
	})

	if !strings.Contains(result, "expiration") {
		t.Fatalf("expected sync expiration validation error, got %q", result)
	}
}

func TestBindNamespaceMarksTypedArraysAsBinary(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()

	runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}

		_, err := vm.RunString(`
			var bytes = new Uint8Array(4096);
			for (var i = 0; i < bytes.length; i++) {
				bytes[i] = 65;
			}

			KV.put("typed-binary", bytes)
				.then(function () {
					done("ok");
				})
				.catch(function (err) {
					fail(String(err));
				});
		`)
		return err
	})

	if len(mem.puts) != 1 {
		t.Fatalf("expected a single put call, got %d", len(mem.puts))
	}
	if got := mem.puts[0].options.ValueKind; got != store.ValueKindBinary {
		t.Fatalf("expected typed array payload to be binary, got value kind %d", got)
	}
}

func TestBindNamespaceSupportsReadableStreamGetAndMetadata(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		streams.Enable(vm)
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}

		_, err := vm.RunString(`
			KV.put("stream-value", JSON.stringify({message: "hello-stream"}), {
				metadata: {kind: "stream"}
			})
				.then(function () {
					return KV.getWithMetadata("stream-value", "stream");
				})
				.then(function (result) {
					var decoder = new TextDecoder();
					var reader = result.value.getReader();
					var parts = [];
					function pump() {
						return reader.read().then(function (chunk) {
							if (chunk.done) {
								done(JSON.stringify({
									text: parts.join(""),
									metadata: result.metadata
								}));
								return;
							}
							parts.push(decoder.decode(chunk.value, { stream: true }));
							return pump();
						});
					}
					return pump();
				})
				.catch(function (err) {
					fail(String(err));
				});
		`)
		return err
	})

	var payload struct {
		Text     string         `json:"text"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("unmarshal stream payload: %v", err)
	}
	if payload.Text != `{"message":"hello-stream"}` {
		t.Fatalf("unexpected stream text: %q", payload.Text)
	}
	if payload.Metadata["kind"] != "stream" {
		t.Fatalf("unexpected stream metadata: %#v", payload.Metadata)
	}
}

func TestBindNamespaceSupportsReadableStreamGet(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		streams.Enable(vm)
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}

		_, err := vm.RunString(`
			KV.put("stream-get", "hello raw stream")
				.then(function () {
					return KV.get("stream-get", "stream");
				})
				.then(function (stream) {
					var decoder = new TextDecoder();
					var reader = stream.getReader();
					var parts = [];
					function pump() {
						return reader.read().then(function (chunk) {
							if (chunk.done) {
								done(parts.join(""));
								return;
							}
							parts.push(decoder.decode(chunk.value, { stream: true }));
							return pump();
						});
					}
					return pump();
				})
				.catch(function (err) {
					fail(String(err));
				});
		`)
		return err
	})

	if result != "hello raw stream" {
		t.Fatalf("unexpected readable stream get result: %q", result)
	}
}

func TestBindNamespaceSupportsReadableStreamPut(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		streams.Enable(vm)
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}

		_, err := vm.RunString(`
			var source = new ReadableStream({
				start(controller) {
					controller.enqueue(new TextEncoder().encode("hello "));
					controller.enqueue(new TextEncoder().encode("stream put"));
					controller.close();
				}
			});

			KV.put("stream-put", source, {
				metadata: {source: "readable-stream"}
			})
				.then(function () {
					return KV.getWithMetadata("stream-put");
				})
				.then(function (result) {
					done(JSON.stringify(result));
				})
				.catch(function (err) {
					fail(String(err));
				});
		`)
		return err
	})

	var payload struct {
		Value    string         `json:"value"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("unmarshal stream put payload: %v", err)
	}
	if payload.Value != "hello stream put" {
		t.Fatalf("unexpected stream put value: %q", payload.Value)
	}
	if payload.Metadata["source"] != "readable-stream" {
		t.Fatalf("unexpected stream put metadata: %#v", payload.Metadata)
	}
}

func TestBindSyncNamespaceRejectsReadableStreamPut(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		streams.Enable(vm)
		if err := cloudflarekv.BindSyncNamespace(vm, "SyncKV", mem); err != nil {
			return err
		}

		_, err := vm.RunString(`
			try {
				var source = new ReadableStream({
					start(controller) {
						controller.enqueue(new TextEncoder().encode("hello"));
						controller.close();
					}
				});
				SyncKV.put("sync-stream-put", source);
				fail("expected SyncKV.put(stream) to throw");
			} catch (err) {
				done(String(err));
			}
		`)
		return err
	})

	if !strings.Contains(result, "ReadableStream") {
		t.Fatalf("expected sync stream rejection, got %q", result)
	}
}

// textCodecScript provides minimal TextEncoder/TextDecoder globals so the
// ReadableStream tests do not depend on other modules registering them.
const textCodecScript = `
	function TextEncoder() {}
	TextEncoder.prototype.encode = function (str) {
		var bytes = [];
		for (var i = 0; i < str.length; i++) {
			var code = str.charCodeAt(i);
			if (code < 0x80) {
				bytes.push(code);
			} else if (code < 0x800) {
				bytes.push(0xc0 | (code >> 6), 0x80 | (code & 0x3f));
			} else {
				bytes.push(0xe0 | (code >> 12), 0x80 | ((code >> 6) & 0x3f), 0x80 | (code & 0x3f));
			}
		}
		return new Uint8Array(bytes);
	};
	function TextDecoder() {}
	TextDecoder.prototype.decode = function (bytes) {
		var out = [];
		var i = 0;
		while (i < bytes.length) {
			var b = bytes[i];
			if (b < 0x80) {
				out.push(String.fromCharCode(b));
				i++;
			} else if (b < 0xe0) {
				out.push(String.fromCharCode(((b & 0x1f) << 6) | (bytes[i + 1] & 0x3f)));
				i += 2;
			} else {
				out.push(String.fromCharCode(((b & 0x0f) << 12) | ((bytes[i + 1] & 0x3f) << 6) | (bytes[i + 2] & 0x3f)));
				i += 3;
			}
		}
		return out.join("");
	};
`

func runScript(t *testing.T, loop *eventloop.EventLoop, run func(vm *goja.Runtime) error) string {
	t.Helper()

	doneCh := make(chan string, 1)
	errCh := make(chan error, 1)

	loop.RunOnLoop(func(vm *goja.Runtime) {
		if _, err := vm.RunString(textCodecScript); err != nil {
			errCh <- err
			return
		}
		if err := vm.Set("done", func(value string) {
			doneCh <- value
		}); err != nil {
			errCh <- err
			return
		}
		if err := vm.Set("fail", func(value string) {
			errCh <- errors.New(value)
		}); err != nil {
			errCh <- err
			return
		}
		if err := run(vm); err != nil {
			errCh <- err
		}
	})

	select {
	case value := <-doneCh:
		return value
	case err := <-errCh:
		t.Fatalf("script failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for JS script to complete")
	}

	return ""
}
