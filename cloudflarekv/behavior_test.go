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

func TestGetMissingKeyReturnsNull(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}
		_, err := vm.RunString(`
			KV.get("missing", "json")
				.then(function (value) {
					return KV.getWithMetadata("missing").then(function (withMetadata) {
						done(value + "|" + JSON.stringify(withMetadata));
					});
				})
				.catch(function (err) {
					fail(String(err));
				});
		`)
		return err
	})

	if result != "null|{\"value\":null,\"metadata\":null}" {
		t.Fatalf("unexpected missing-key result: %q", result)
	}
}

func TestGetArrayBuffer(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}
		_, err := vm.RunString(`
			KV.put("ab", "hello")
				.then(function () {
					return KV.get("ab", "arrayBuffer");
				})
				.then(function (buffer) {
					var view = new Uint8Array(buffer);
					var parts = [];
					for (var i = 0; i < view.length; i++) {
						parts.push(String.fromCharCode(view[i]));
					}
					done(parts.join(""));
				})
				.catch(function (err) {
					fail(String(err));
				});
		`)
		return err
	})

	if result != "hello" {
		t.Fatalf("unexpected arrayBuffer result: %q", result)
	}
}

func TestGetInvalidJSONRejects(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}
		_, err := vm.RunString(`
			KV.put("bad-json", "not json at all")
				.then(function () {
					return KV.get("bad-json", "json");
				})
				.then(function () {
					fail("expected JSON parse to reject");
				}, function (err) {
					done(String(err));
				});
		`)
		return err
	})

	if !strings.Contains(result, "JSON") {
		t.Fatalf("expected JSON parse error, got %q", result)
	}
}

func TestGetUnsupportedTypeRejects(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}
		_, err := vm.RunString(`
			KV.put("x", "v")
				.then(function () {
					return KV.get("x", "unknown-type");
				})
				.then(function () {
					fail("expected unsupported type to reject");
				}, function (err) {
					done(String(err));
				});
		`)
		return err
	})

	if !strings.Contains(result, "unsupported type") {
		t.Fatalf("expected unsupported type error, got %q", result)
	}
}

func TestPutRejectsUnsupportedValue(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}
		_, err := vm.RunString(`
			KV.put("num", 42)
				.then(function () {
					fail("expected put to reject");
				}, function (err) {
					done(String(err));
				});
		`)
		return err
	})

	if !strings.Contains(result, "value must be") {
		t.Fatalf("expected value validation error, got %q", result)
	}
}

func TestListPagination(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}
		_, err := vm.RunString(`
			var puts = ["a", "b", "c", "d", "e"].map(function (suffix) {
				return KV.put("page:" + suffix, suffix);
			});
			Promise.all(puts)
				.then(function () {
					return KV.list({prefix: "page:", limit: 2});
				})
				.then(function (first) {
					if (first.keys.length !== 2 || first.list_complete !== false || first.cursor === undefined) {
						throw new Error("unexpected first page: " + JSON.stringify(first));
					}
					return KV.list({prefix: "page:", limit: 2, cursor: first.cursor});
				})
				.then(function (second) {
					if (second.keys.length !== 2 || second.list_complete !== false) {
						throw new Error("unexpected second page: " + JSON.stringify(second));
					}
					return KV.list({prefix: "page:", limit: 2, cursor: second.cursor});
				})
				.then(function (third) {
					if (third.keys.length !== 1 || third.list_complete !== true) {
						throw new Error("unexpected third page: " + JSON.stringify(third));
					}
					done(third.keys[0].name);
				})
				.catch(function (err) {
					fail(String(err));
				});
		`)
		return err
	})

	if result != "page:e" {
		t.Fatalf("unexpected pagination result: %q", result)
	}
}

func TestDeleteRemovesKey(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}
		_, err := vm.RunString(`
			KV.put("del-me", "value")
				.then(function () {
					return KV.delete("del-me");
				})
				.then(function () {
					return KV.get("del-me");
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

	if result != "null" {
		t.Fatalf("expected deleted key to read as null, got %q", result)
	}
}

func TestPutExpirationTimestampIsRecorded(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()

	runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}
		_, err := vm.RunString(`
			KV.put("exp", "v", {expiration: 1700000000})
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
	expiration := mem.puts[0].options.Expiration
	if expiration == nil {
		t.Fatal("expected expiration to be recorded")
	}
	want := time.Unix(1700000000, 0).UTC()
	if !expiration.Equal(want) {
		t.Fatalf("expiration = %v, want %v", expiration, want)
	}
}

func TestExpiredKeyReadsAsMissing(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	mem.now = func() time.Time { return now }

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}
		_, err := vm.RunString(`
			KV.put("short-lived", "v", {expirationTtl: 60})
				.then(function () {
					return KV.get("short-lived");
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

	if result != "v" {
		t.Fatalf("expected unexpired key to read as value, got %q", result)
	}

	// Advance the store clock past the TTL and verify the key reads as missing.
	expired := now.Add(2 * time.Hour)
	mem.now = func() time.Time { return expired }

	result = runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}
		_, err := vm.RunString(`
			KV.get("short-lived")
				.then(function (value) {
					done(String(value));
				})
				.catch(function (err) {
					fail(String(err));
				});
		`)
		return err
	})

	if result != "null" {
		t.Fatalf("expected expired key to read as null, got %q", result)
	}
}

func TestSyncGetStreamRejects(t *testing.T) {
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
			SyncKV.put("s", "value");
			try {
				SyncKV.get("s", "stream");
				fail("expected SyncKV.get(stream) to throw");
			} catch (err) {
				done(String(err));
			}
		`)
		return err
	})

	if !strings.Contains(result, "stream") {
		t.Fatalf("expected sync stream rejection, got %q", result)
	}
}

func TestInstallConstructorUnknownNamespaceRejects(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		resolver := func(namespace string) (store.NamespaceStore, error) {
			if namespace == "known" {
				return mem, nil
			}
			return nil, errors.New("unknown namespace: " + namespace)
		}
		if err := cloudflarekv.InstallConstructor(vm, loop, resolver); err != nil {
			return err
		}
		_, err := vm.RunString(`
			try {
				new KVNamespace("nope");
				fail("expected constructor to throw");
			} catch (err) {
				done(String(err));
			}
		`)
		return err
	})

	if !strings.Contains(result, "unknown") {
		t.Fatalf("expected unknown namespace error, got %q", result)
	}
}

func TestBindNamespaceValidation(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()
	vm := goja.New()

	cases := []struct {
		name        string
		bindingName string
		ns          store.NamespaceStore
		want        string
	}{
		{"empty binding name", "  ", mem, "binding name is required"},
		{"nil store", "KV", nil, "store is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := cloudflarekv.BindNamespace(vm, loop, tc.bindingName, tc.ns)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("BindNamespace error = %v, want containing %q", err, tc.want)
			}
		})
	}

	if err := cloudflarekv.BindNamespace(nil, loop, "KV", mem); err == nil || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("nil runtime error = %v", err)
	}
	if err := cloudflarekv.BindNamespace(vm, nil, "KV", mem); err == nil || !strings.Contains(err.Error(), "event loop") {
		t.Fatalf("nil loop error = %v", err)
	}
	if err := cloudflarekv.BindSyncNamespace(vm, "SyncKV", mem); err != nil {
		t.Fatalf("BindSyncNamespace = %v", err)
	}
}

func TestBindNamespaceObjectValidation(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()
	vm := goja.New()
	target := vm.NewObject()

	tests := []struct {
		name string
		bind func() error
		want string
	}{
		{"async nil runtime", func() error { return cloudflarekv.BindNamespaceObject(nil, loop, target, mem) }, "runtime is required"},
		{"async nil loop", func() error { return cloudflarekv.BindNamespaceObject(vm, nil, target, mem) }, "event loop is required"},
		{"async nil target", func() error { return cloudflarekv.BindNamespaceObject(vm, loop, nil, mem) }, "target is required"},
		{"async nil store", func() error { return cloudflarekv.BindNamespaceObject(vm, loop, target, nil) }, "store is required"},
		{"sync nil runtime", func() error { return cloudflarekv.BindSyncNamespaceObject(nil, target, mem) }, "runtime is required"},
		{"sync nil target", func() error { return cloudflarekv.BindSyncNamespaceObject(vm, nil, mem) }, "target is required"},
		{"sync nil store", func() error { return cloudflarekv.BindSyncNamespaceObject(vm, target, nil) }, "store is required"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.bind()
			if err == nil || err.Error() != test.want {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestPutClassifiesJSONStrings(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()

	runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}
		_, err := vm.RunString(`
			Promise.all([
				KV.put("json-str", JSON.stringify({a: 1})),
				KV.put("plain-str", "hello"),
				KV.put("array-str", "[1, 2, 3]")
			]).then(function () {
				done("ok");
			}).catch(function (err) {
				fail(String(err));
			});
		`)
		return err
	})

	if len(mem.puts) != 3 {
		t.Fatalf("expected 3 put calls, got %d", len(mem.puts))
	}
	byKey := map[string]store.ValueKind{}
	for _, put := range mem.puts {
		byKey[put.key] = put.options.ValueKind
	}
	if byKey["json-str"] != store.ValueKindJSON {
		t.Fatalf("json-str kind = %d, want JSON", byKey["json-str"])
	}
	if byKey["array-str"] != store.ValueKindJSON {
		t.Fatalf("array-str kind = %d, want JSON", byKey["array-str"])
	}
	if byKey["plain-str"] != store.ValueKindText {
		t.Fatalf("plain-str kind = %d, want Text", byKey["plain-str"])
	}
}

func TestJSONRoundTripThroughList(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	mem := newMemStore()

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			return err
		}
		_, err := vm.RunString(`
			KV.put("doc", JSON.stringify({nested: {deep: [1, 2, 3]}}), {metadata: {tag: "x"}})
				.then(function () {
					return KV.list({prefix: "doc"});
				})
				.then(function (page) {
					var key = page.keys[0];
					done(JSON.stringify({name: key.name, metadata: key.metadata}));
				})
				.catch(function (err) {
					fail(String(err));
				});
		`)
		return err
	})

	var payload struct {
		Name     string         `json:"name"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("unmarshal list round trip: %v", err)
	}
	if payload.Name != "doc" {
		t.Fatalf("unexpected key name: %q", payload.Name)
	}
	if payload.Metadata["tag"] != "x" {
		t.Fatalf("unexpected list metadata: %#v", payload.Metadata)
	}
}
