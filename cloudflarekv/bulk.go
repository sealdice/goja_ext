package cloudflarekv

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/cloudflarekv/store"
	"github.com/dop251/goja_nodejs/eventloop"
)

const bulkFallbackConcurrency = 8

func parseGetKeys(vm *goja.Runtime, value goja.Value) ([]string, bool, error) {
	if goja.IsString(value) {
		return []string{value.String()}, false, nil
	}
	object, ok := value.(*goja.Object)
	if !ok || object.ClassName() != "Array" {
		return nil, false, errors.New("key must be a string or an array of strings")
	}
	var keys []string
	if err := vm.ExportTo(value, &keys); err != nil {
		return nil, false, errors.New("keys must be an array of strings")
	}
	return keys, true, nil
}

func getManyFromStore(
	ctx context.Context,
	ns store.NamespaceStore,
	keys []string,
) (map[string]store.Record, error) {
	if getter, ok := ns.(store.BulkGetter); ok {
		return getter.GetMany(ctx, keys)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(map[string]store.Record, len(keys))
	jobs := make(chan string)
	var resultMu sync.Mutex
	var firstErr error
	var errOnce sync.Once
	var workers sync.WaitGroup
	workerCount := min(len(keys), bulkFallbackConcurrency)
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for key := range jobs {
				record, found, err := ns.Get(ctx, key)
				if err != nil {
					errOnce.Do(func() {
						firstErr = err
						cancel()
					})
					return
				}
				if found {
					resultMu.Lock()
					result[key] = record
					resultMu.Unlock()
				}
			}
		}()
	}
producer:
	for _, key := range keys {
		select {
		case jobs <- key:
		case <-ctx.Done():
			break producer
		}
	}
	close(jobs)
	workers.Wait()
	return result, firstErr
}

func getManyPromise(
	vm *goja.Runtime,
	loop *eventloop.EventLoop,
	state *bindingState,
	keys []string,
	valueType string,
	withMetadata bool,
	cacheTTL time.Duration,
) goja.Value {
	return newPromise(vm, loop, func() (any, error) {
		records := make(map[string]store.Record, len(keys))
		misses := make([]string, 0, len(keys))
		seenMiss := make(map[string]struct{}, len(keys))
		for _, key := range keys {
			if record, found, cached := state.cache.get(key); cached {
				if found {
					records[key] = record
				}
				continue
			}
			if _, seen := seenMiss[key]; !seen {
				seenMiss[key] = struct{}{}
				misses = append(misses, key)
			}
		}
		fetched := make(map[string]store.Record, len(misses))
		if len(misses) > 0 {
			var err error
			fetched, err = getManyFromStore(context.Background(), state.ns, misses)
			if err != nil {
				return nil, err
			}
		}
		for _, key := range misses {
			record, found := fetched[key]
			state.cache.put(key, record, found, cacheTTL)
			if found {
				records[key] = record
			}
		}
		if maximumResponseBytes := state.config.limits.MaxBulkResponseBytes; maximumResponseBytes > 0 {
			var size int64
			for _, record := range records {
				size += int64(len(record.Value) + len(record.Metadata))
				if size > maximumResponseBytes {
					return nil, fmt.Errorf("bulk get response exceeds the maximum size of %d bytes", maximumResponseBytes)
				}
			}
		}
		return records, nil
	}, func(vm *goja.Runtime, raw any) (goja.Value, error) {
		records := raw.(map[string]store.Record)
		result, err := vm.New(vm.Get("Map"))
		if err != nil {
			return nil, err
		}
		set, ok := goja.AssertFunction(result.Get("set"))
		if !ok {
			return nil, errors.New("map.set is not callable")
		}
		for _, key := range keys {
			record, found := records[key]
			var value goja.Value
			if found {
				value, err = toJSValue(vm, record.Value, valueType)
				if err != nil {
					return nil, err
				}
			} else {
				value = goja.Null()
			}
			if withMetadata {
				entry := vm.NewObject()
				_ = entry.Set("value", value)
				metadata := goja.Null()
				if found {
					metadata, err = metadataToValue(vm, record.Metadata)
					if err != nil {
						return nil, err
					}
				}
				_ = entry.Set("metadata", metadata)
				value = entry
			}
			if _, err := set(result, vm.ToValue(key), value); err != nil {
				return nil, err
			}
		}
		return result, nil
	})
}
