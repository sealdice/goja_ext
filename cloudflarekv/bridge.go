package cloudflarekv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/cloudflarekv/store"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/streams"
)

const readableStreamChunkSize = 64 * 1024

func InstallConstructor(vm *goja.Runtime, loop *eventloop.EventLoop, resolver func(namespace string) (store.NamespaceStore, error)) error {
	if vm == nil {
		return errors.New("runtime is required")
	}
	if loop == nil {
		return errors.New("event loop is required")
	}
	if resolver == nil {
		return errors.New("resolver is required")
	}

	constructor := func(call goja.ConstructorCall) *goja.Object {
		namespace := strings.TrimSpace(call.Argument(0).String())
		store, err := resolver(namespace)
		if err != nil {
			panic(vm.ToValue(err.Error()))
		}

		if err := bindObject(vm, loop, call.This, store); err != nil {
			panic(vm.ToValue(err.Error()))
		}
		return nil
	}

	ctor := vm.ToValue(constructor).(*goja.Object)
	return vm.Set("KVNamespace", ctor)
}

func BindNamespace(vm *goja.Runtime, loop *eventloop.EventLoop, bindingName string, ns store.NamespaceStore) error {
	if vm == nil {
		return errors.New("runtime is required")
	}
	if loop == nil {
		return errors.New("event loop is required")
	}
	if strings.TrimSpace(bindingName) == "" {
		return errors.New("binding name is required")
	}
	if ns == nil {
		return errors.New("store is required")
	}

	object := vm.NewObject()
	if err := bindObject(vm, loop, object, ns); err != nil {
		return err
	}
	return vm.Set(bindingName, object)
}

func BindSyncNamespace(vm *goja.Runtime, bindingName string, ns store.NamespaceStore) error {
	if vm == nil {
		return errors.New("runtime is required")
	}
	if strings.TrimSpace(bindingName) == "" {
		return errors.New("binding name is required")
	}
	if ns == nil {
		return errors.New("store is required")
	}

	object := vm.NewObject()
	if err := bindSyncObject(vm, object, ns); err != nil {
		return err
	}
	return vm.Set(bindingName, object)
}

func bindObject(vm *goja.Runtime, loop *eventloop.EventLoop, object *goja.Object, ns store.NamespaceStore) error {
	if err := object.Set("get", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		valueType, err := parseGetType(vm, call.Argument(1))
		if err != nil {
			return rejectedPromise(vm, err)
		}

		type getResult struct {
			record store.Record
			found  bool
		}

		return newPromise(vm, loop, func() (any, error) {
			record, found, err := ns.Get(context.Background(), key)
			if err != nil {
				return nil, err
			}
			return getResult{record: record, found: found}, nil
		}, func(vm *goja.Runtime, raw any) (goja.Value, error) {
			result := raw.(getResult)
			if !result.found {
				return goja.Null(), nil
			}
			if valueType == "stream" {
				return bytesToReadableStreamValue(vm, result.record.Value)
			}
			return toJSValue(vm, result.record.Value, valueType)
		})
	}); err != nil {
		return err
	}

	if err := object.Set("getWithMetadata", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		valueType, err := parseGetType(vm, call.Argument(1))
		if err != nil {
			return rejectedPromise(vm, err)
		}

		type getResult struct {
			record store.Record
			found  bool
		}

		return newPromise(vm, loop, func() (any, error) {
			record, found, getErr := ns.Get(context.Background(), key)
			if getErr != nil {
				return nil, getErr
			}
			return getResult{record: record, found: found}, nil
		}, func(vm *goja.Runtime, raw any) (goja.Value, error) {
			payload := raw.(getResult)
			result := vm.NewObject()
			if !payload.found {
				_ = result.Set("value", goja.Null())
				_ = result.Set("metadata", goja.Null())
				return result, nil
			}

			var value goja.Value
			if valueType == "stream" {
				value, err = bytesToReadableStreamValue(vm, payload.record.Value)
			} else {
				value, err = toJSValue(vm, payload.record.Value, valueType)
			}
			if err != nil {
				return nil, err
			}
			metadata, err := metadataToValue(vm, payload.record.Metadata)
			if err != nil {
				return nil, err
			}

			_ = result.Set("value", value)
			_ = result.Set("metadata", metadata)
			return result, nil
		})
	}); err != nil {
		return err
	}

	if err := object.Set("put", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		if streams.IsReadableStream(vm, call.Argument(1)) {
			options, err := parsePutOptions(vm, call.Argument(2))
			if err != nil {
				return rejectedPromise(vm, err)
			}
			options.ValueKind = store.ValueKindBinary
			return putReadableStreamPromise(vm, loop, ns, key, call.Argument(1), options)
		}

		value, valueKind, err := exportBytes(vm, call.Argument(1))
		if err != nil {
			return rejectedPromise(vm, err)
		}
		options, err := parsePutOptions(vm, call.Argument(2))
		if err != nil {
			return rejectedPromise(vm, err)
		}
		options.ValueKind = valueKind

		return newPromise(vm, loop, func() (any, error) {
			if err := ns.Put(context.Background(), key, value, options); err != nil {
				return nil, err
			}
			return nil, nil
		}, func(vm *goja.Runtime, raw any) (goja.Value, error) {
			return goja.Undefined(), nil
		})
	}); err != nil {
		return err
	}

	if err := object.Set("delete", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		return newPromise(vm, loop, func() (any, error) {
			if err := ns.Delete(context.Background(), key); err != nil {
				return nil, err
			}
			return nil, nil
		}, func(vm *goja.Runtime, raw any) (goja.Value, error) {
			return goja.Undefined(), nil
		})
	}); err != nil {
		return err
	}

	if err := object.Set("list", func(call goja.FunctionCall) goja.Value {
		options, err := parseListOptions(vm, call.Argument(0))
		if err != nil {
			return rejectedPromise(vm, err)
		}

		return newPromise(vm, loop, func() (any, error) {
			result, err := ns.List(context.Background(), options)
			if err != nil {
				return nil, err
			}
			return result, nil
		}, func(vm *goja.Runtime, raw any) (goja.Value, error) {
			return listToValue(vm, raw.(store.ListResult))
		})
	}); err != nil {
		return err
	}

	return nil
}

func bindSyncObject(vm *goja.Runtime, object *goja.Object, ns store.NamespaceStore) error {
	if err := object.Set("get", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		valueType, err := parseGetType(vm, call.Argument(1))
		if err != nil {
			panic(jsErrorValue(vm, err))
		}

		record, found, err := ns.Get(context.Background(), key)
		if err != nil {
			panic(jsErrorValue(vm, err))
		}
		if !found {
			return goja.Null()
		}
		if valueType == "stream" {
			panic(jsErrorValue(vm, errors.New(`SyncKV.get(..., "stream") is not supported; use async KV.get(..., "stream") instead`)))
		}

		value, err := toJSValue(vm, record.Value, valueType)
		if err != nil {
			panic(jsErrorValue(vm, err))
		}
		return value
	}); err != nil {
		return err
	}

	if err := object.Set("getWithMetadata", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		valueType, err := parseGetType(vm, call.Argument(1))
		if err != nil {
			panic(jsErrorValue(vm, err))
		}

		record, found, err := ns.Get(context.Background(), key)
		if err != nil {
			panic(jsErrorValue(vm, err))
		}

		result := vm.NewObject()
		if !found {
			_ = result.Set("value", goja.Null())
			_ = result.Set("metadata", goja.Null())
			return result
		}
		if valueType == "stream" {
			panic(jsErrorValue(vm, errors.New(`SyncKV.getWithMetadata(..., "stream") is not supported; use async KV.getWithMetadata(..., "stream") instead`)))
		}

		value, err := toJSValue(vm, record.Value, valueType)
		if err != nil {
			panic(jsErrorValue(vm, err))
		}
		metadata, err := metadataToValue(vm, record.Metadata)
		if err != nil {
			panic(jsErrorValue(vm, err))
		}

		_ = result.Set("value", value)
		_ = result.Set("metadata", metadata)
		return result
	}); err != nil {
		return err
	}

	if err := object.Set("put", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		if streams.IsReadableStream(vm, call.Argument(1)) {
			panic(jsErrorValue(vm, errors.New("ReadableStream input is only supported by async KV.put")))
		}
		value, valueKind, err := exportBytes(vm, call.Argument(1))
		if err != nil {
			panic(jsErrorValue(vm, err))
		}
		options, err := parsePutOptions(vm, call.Argument(2))
		if err != nil {
			panic(jsErrorValue(vm, err))
		}
		options.ValueKind = valueKind

		if err := ns.Put(context.Background(), key, value, options); err != nil {
			panic(jsErrorValue(vm, err))
		}

		return goja.Undefined()
	}); err != nil {
		return err
	}

	if err := object.Set("delete", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		if err := ns.Delete(context.Background(), key); err != nil {
			panic(jsErrorValue(vm, err))
		}
		return goja.Undefined()
	}); err != nil {
		return err
	}

	if err := object.Set("list", func(call goja.FunctionCall) goja.Value {
		options, err := parseListOptions(vm, call.Argument(0))
		if err != nil {
			panic(jsErrorValue(vm, err))
		}

		result, err := ns.List(context.Background(), options)
		if err != nil {
			panic(jsErrorValue(vm, err))
		}

		value, err := listToValue(vm, result)
		if err != nil {
			panic(jsErrorValue(vm, err))
		}
		return value
	}); err != nil {
		return err
	}

	return nil
}

func newPromise(
	vm *goja.Runtime,
	loop *eventloop.EventLoop,
	work func() (any, error),
	convert func(*goja.Runtime, any) (goja.Value, error),
) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	go func() {
		payload, err := work()
		loop.RunOnLoop(func(loopVM *goja.Runtime) {
			if err != nil {
				_ = reject(loopVM.ToValue(err.Error()))
				return
			}

			value, convertErr := convert(loopVM, payload)
			if convertErr != nil {
				_ = reject(loopVM.ToValue(convertErr.Error()))
				return
			}
			_ = resolve(value)
		})
	}()

	return vm.ToValue(promise)
}

func rejectedPromise(vm *goja.Runtime, err error) goja.Value {
	promise, _, reject := vm.NewPromise()
	_ = reject(jsErrorValue(vm, err))
	return vm.ToValue(promise)
}

func parseGetType(vm *goja.Runtime, argument goja.Value) (string, error) {
	if isNilish(argument) {
		return "text", nil
	}

	if exported, ok := argument.Export().(string); ok {
		return normalizeValueType(exported)
	}

	object := argument.ToObject(vm)
	typeValue := object.Get("type")
	if isNilish(typeValue) {
		return "text", nil
	}
	exported, ok := typeValue.Export().(string)
	if !ok {
		return "", errors.New("type option must be a string")
	}
	return normalizeValueType(exported)
}

func parsePutOptions(vm *goja.Runtime, argument goja.Value) (store.PutOptions, error) {
	if isNilish(argument) {
		return store.PutOptions{}, nil
	}

	object := argument.ToObject(vm)
	expirationValue := object.Get("expiration")
	expirationTTLValue := object.Get("expirationTtl")
	if !isNilish(expirationValue) && !isNilish(expirationTTLValue) {
		return store.PutOptions{}, errors.New("expiration and expirationTtl cannot both be set")
	}

	options := store.PutOptions{}
	if !isNilish(expirationValue) {
		seconds, err := numberToInt64(expirationValue)
		if err != nil {
			return store.PutOptions{}, fmt.Errorf("expiration: %w", err)
		}
		expiration := time.Unix(seconds, 0).UTC()
		options.Expiration = &expiration
	}
	if !isNilish(expirationTTLValue) {
		seconds, err := numberToInt64(expirationTTLValue)
		if err != nil {
			return store.PutOptions{}, fmt.Errorf("expirationTtl: %w", err)
		}
		options.ExpirationTTL = time.Duration(seconds) * time.Second
	}

	metadataValue := object.Get("metadata")
	if !isNilish(metadataValue) {
		options.Metadata = metadataValue.Export()
	}

	return options, nil
}

func parseListOptions(vm *goja.Runtime, argument goja.Value) (store.ListOptions, error) {
	if isNilish(argument) {
		return store.ListOptions{}, nil
	}

	object := argument.ToObject(vm)
	options := store.ListOptions{}

	prefixValue := object.Get("prefix")
	if !isNilish(prefixValue) {
		prefix, ok := prefixValue.Export().(string)
		if !ok {
			return store.ListOptions{}, errors.New("prefix must be a string")
		}
		options.Prefix = prefix
	}

	limitValue := object.Get("limit")
	if !isNilish(limitValue) {
		limit, err := numberToInt64(limitValue)
		if err != nil {
			return store.ListOptions{}, fmt.Errorf("limit: %w", err)
		}
		options.Limit = int(limit)
	}

	cursorValue := object.Get("cursor")
	if !isNilish(cursorValue) {
		cursor, ok := cursorValue.Export().(string)
		if !ok {
			return store.ListOptions{}, errors.New("cursor must be a string")
		}
		options.Cursor = cursor
	}

	return options, nil
}

func normalizeValueType(value string) (string, error) {
	switch value {
	case "", "text":
		return "text", nil
	case "json":
		return "json", nil
	case "arrayBuffer":
		return "arrayBuffer", nil
	case "stream":
		return "stream", nil
	default:
		return "", fmt.Errorf("unsupported type %q", value)
	}
}

func exportBytes(vm *goja.Runtime, value goja.Value) ([]byte, store.ValueKind, error) {
	if exported, ok := value.Export().(string); ok {
		return []byte(exported), classifyStringValue(exported), nil
	}

	var bytes []byte
	if err := vm.ExportTo(value, &bytes); err == nil {
		return append([]byte(nil), bytes...), store.ValueKindBinary, nil
	}

	return nil, store.ValueKindUnknown, errors.New("value must be a string, ArrayBuffer, TypedArray, or DataView")
}

func toJSValue(vm *goja.Runtime, value []byte, valueType string) (goja.Value, error) {
	switch valueType {
	case "text":
		return vm.ToValue(string(value)), nil
	case "json":
		var payload any
		if err := json.Unmarshal(value, &payload); err != nil {
			return nil, fmt.Errorf("parse JSON value: %w", err)
		}
		return vm.ToValue(payload), nil
	case "arrayBuffer":
		return vm.ToValue(vm.NewArrayBuffer(append([]byte(nil), value...))), nil
	default:
		return nil, fmt.Errorf("unsupported type %q", valueType)
	}
}

func metadataToValue(vm *goja.Runtime, metadata json.RawMessage) (goja.Value, error) {
	if len(metadata) == 0 {
		return goja.Null(), nil
	}

	var payload any
	if err := json.Unmarshal(metadata, &payload); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}
	return vm.ToValue(payload), nil
}

func listToValue(vm *goja.Runtime, result store.ListResult) (goja.Value, error) {
	object := vm.NewObject()

	keys := make([]any, 0, len(result.Keys))
	for _, key := range result.Keys {
		item := vm.NewObject()
		_ = item.Set("name", key.Name)
		if key.Expiration != nil {
			_ = item.Set("expiration", key.Expiration.Unix())
		}
		metadata, err := metadataToValue(vm, key.Metadata)
		if err != nil {
			return nil, err
		}
		if !goja.IsNull(metadata) {
			_ = item.Set("metadata", metadata)
		}
		keys = append(keys, item)
	}

	_ = object.Set("keys", keys)
	_ = object.Set("list_complete", result.ListComplete)
	if result.Cursor != "" {
		_ = object.Set("cursor", result.Cursor)
	}

	return object, nil
}

func numberToInt64(value goja.Value) (int64, error) {
	switch number := value.Export().(type) {
	case int:
		return int64(number), nil
	case int8:
		return int64(number), nil
	case int16:
		return int64(number), nil
	case int32:
		return int64(number), nil
	case int64:
		return number, nil
	case uint:
		return int64(number), nil
	case uint8:
		return int64(number), nil
	case uint16:
		return int64(number), nil
	case uint32:
		return int64(number), nil
	case uint64:
		if number > math.MaxInt64 {
			return 0, errors.New("must fit in int64")
		}
		return int64(number), nil
	case float32:
		return floatNumberToInt64(float64(number))
	case float64:
		return floatNumberToInt64(number)
	default:
		return 0, errors.New("must be a number")
	}
}

func isNilish(value goja.Value) bool {
	return value == nil || goja.IsUndefined(value) || goja.IsNull(value)
}

func floatNumberToInt64(number float64) (int64, error) {
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, errors.New("must be a finite number")
	}
	if number != math.Trunc(number) {
		return 0, errors.New("must be an integer")
	}
	if number < math.MinInt64 || number > math.MaxInt64 {
		return 0, errors.New("must fit in int64")
	}
	return int64(number), nil
}

func jsErrorValue(vm *goja.Runtime, err error) goja.Value {
	return vm.ToValue(err.Error())
}

func classifyStringValue(value string) store.ValueKind {
	trimmed := bytes.TrimSpace([]byte(value))
	if len(trimmed) > 0 {
		switch trimmed[0] {
		case '{', '[':
			if json.Valid(trimmed) {
				return store.ValueKindJSON
			}
		}
	}
	return store.ValueKindText
}

func bytesToReadableStreamValue(vm *goja.Runtime, value []byte) (goja.Value, error) {
	data := append([]byte(nil), value...)
	offset := 0

	stream, err := streams.NewReadableStream(vm, streams.ReadableStreamSource{
		Pull: func(controller *goja.Object) goja.Value {
			if offset >= len(data) {
				callObjectMethodOrPanic(vm, controller, "close")
				return goja.Undefined()
			}

			end := offset + readableStreamChunkSize
			if end > len(data) {
				end = len(data)
			}
			chunk := append([]byte(nil), data[offset:end]...)
			offset = end
			callObjectMethodOrPanic(vm, controller, "enqueue", vm.ToValue(vm.NewArrayBuffer(chunk)))
			return goja.Undefined()
		},
		Cancel: func(reason goja.Value) goja.Value {
			offset = len(data)
			return goja.Undefined()
		},
	})
	if err != nil {
		return nil, err
	}
	return stream, nil
}

func putReadableStreamPromise(
	vm *goja.Runtime,
	loop *eventloop.EventLoop,
	ns store.NamespaceStore,
	key string,
	streamValue goja.Value,
	options store.PutOptions,
) goja.Value {
	promise, resolve, reject := vm.NewPromise()
	var buffer bytes.Buffer

	consumed, err := streams.ConsumeReadableStream(vm, streamValue, func(chunk goja.Value) goja.Value {
		bytes, chunkErr := streamChunkBytes(vm, chunk)
		if chunkErr != nil {
			panic(vm.ToValue(chunkErr.Error()))
		}
		_, _ = buffer.Write(bytes)
		return goja.Undefined()
	})
	if err != nil {
		_ = reject(vm.ToValue(err.Error()))
		return vm.ToValue(promise)
	}

	thenValue(vm, vm.ToValue(consumed),
		func(call goja.FunctionCall) goja.Value {
			value := append([]byte(nil), buffer.Bytes()...)
			go func() {
				err := ns.Put(context.Background(), key, value, options)
				loop.RunOnLoop(func(loopVM *goja.Runtime) {
					if err != nil {
						_ = reject(loopVM.ToValue(err.Error()))
						return
					}
					_ = resolve(goja.Undefined())
				})
			}()
			return goja.Undefined()
		},
		func(call goja.FunctionCall) goja.Value {
			_ = reject(valueOrUndefined(call.Argument(0)))
			return goja.Undefined()
		},
	)

	return vm.ToValue(promise)
}

func streamChunkBytes(vm *goja.Runtime, value goja.Value) ([]byte, error) {
	if arrayBuffer, ok := value.Export().(goja.ArrayBuffer); ok {
		return append([]byte(nil), arrayBuffer.Bytes()...), nil
	}

	var bytes []byte
	if err := vm.ExportTo(value, &bytes); err == nil {
		return append([]byte(nil), bytes...), nil
	}

	return nil, errors.New("ReadableStream chunk must be an ArrayBuffer or ArrayBufferView")
}

func thenValue(
	vm *goja.Runtime,
	promise goja.Value,
	onFulfilled func(goja.FunctionCall) goja.Value,
	onRejected func(goja.FunctionCall) goja.Value,
) {
	object := promise.ToObject(vm)
	method, ok := goja.AssertFunction(object.Get("then"))
	if !ok {
		panic(vm.NewTypeError("expected a Promise-compatible value"))
	}
	if _, err := method(object, vm.ToValue(onFulfilled), vm.ToValue(onRejected)); err != nil {
		panic(err)
	}
}

func valueOrUndefined(value goja.Value) goja.Value {
	if value == nil {
		return goja.Undefined()
	}
	return value
}

func callObjectMethodOrPanic(vm *goja.Runtime, object *goja.Object, name string, args ...goja.Value) {
	method, ok := goja.AssertFunction(object.Get(name))
	if !ok {
		panic(vm.NewTypeError("%s is not callable", name))
	}
	if _, err := method(object, args...); err != nil {
		panic(err)
	}
}
