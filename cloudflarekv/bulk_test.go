package cloudflarekv_test

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/cloudflarekv"
	"github.com/dop251/goja_nodejs/cloudflarekv/store"
	"github.com/dop251/goja_nodejs/eventloop"
)

type bulkStore struct {
	*memStore
	calls int
}

func (s *bulkStore) GetMany(
	ctx context.Context,
	keys []string,
) (map[string]store.Record, error) {
	s.calls++
	result := make(map[string]store.Record, len(keys))
	for _, key := range keys {
		record, found, err := s.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if found {
			result[key] = record
		}
	}
	return result, nil
}

var _ store.BulkGetter = (*bulkStore)(nil)

func TestBulkGetUsesCapabilityAndReturnsMap(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	backend := &bulkStore{memStore: newMemStore()}
	if err := backend.Put(t.Context(), "a", []byte("A"), store.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := backend.Put(t.Context(), "b", []byte("B"), store.PutOptions{}); err != nil {
		t.Fatal(err)
	}

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", backend); err != nil {
			return err
		}
		_, err := vm.RunString(`
			KV.get(["b", "missing", "a"], "text")
			.then(function () { return KV.get(["b", "missing", "a"], "text"); })
			.then(function (values) {
				if (!(values instanceof Map)) throw new Error("result is not a Map");
				done(JSON.stringify([
					Array.from(values.keys()),
					values.get("b"),
					values.get("missing"),
					values.get("a")
				]));
			}).catch(function (err) { fail(String(err)); });
		`)
		return err
	})

	if result != `[["b","missing","a"],"B",null,"A"]` {
		t.Fatalf("bulk get result = %s", result)
	}
	if backend.calls != 1 {
		t.Fatalf("GetMany calls = %d, want 1", backend.calls)
	}
}

func TestBulkGetWithMetadataReturnsMapEntries(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	backend := &bulkStore{memStore: newMemStore()}
	if err := backend.Put(t.Context(), "a", []byte(`{"ok":true}`), store.PutOptions{
		Metadata: map[string]any{"source": "bulk"},
	}); err != nil {
		t.Fatal(err)
	}

	result := runScript(t, loop, func(vm *goja.Runtime) error {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", backend); err != nil {
			return err
		}
		_, err := vm.RunString(`
			KV.getWithMetadata(["a", "missing"], "json").then(function (values) {
				done(JSON.stringify([
					values.get("a"),
					values.get("missing")
				]));
			}).catch(function (err) { fail(String(err)); });
		`)
		return err
	})

	if result != `[{"value":{"ok":true},"metadata":{"source":"bulk"}},{"value":null,"metadata":null}]` {
		t.Fatalf("bulk metadata result = %s", result)
	}
	if backend.calls != 1 {
		t.Fatalf("GetMany calls = %d, want 1", backend.calls)
	}
}
