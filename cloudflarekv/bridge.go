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

func InstallConstructor(vm *goja.Runtime, loop *eventloop.EventLoop, resolver func(namespace string) (store.NamespaceStore, error), options ...BindOption) error {
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

		if err := bindObject(vm, loop, call.This, newBindingState(store, options)); err != nil {
			panic(vm.ToValue(err.Error()))
		}
		return nil
	}

	ctor := vm.ToValue(constructor).(*goja.Object)
	return vm.Set("KVNamespace", ctor)
}

func BindNamespace(vm *goja.Runtime, loop *eventloop.EventLoop, bindingName string, ns store.NamespaceStore, options ...BindOption) error {
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
	if err := BindNamespaceObject(vm, loop, object, ns, options...); err != nil {
		return err
	}
	return vm.Set(bindingName, object)
}

// BindNamespaceObject binds the asynchronous KV methods onto an existing JS
// object without creating a global binding. This is useful when a namespace
// needs to be composed into another object such as storage.kv.
func BindNamespaceObject(vm *goja.Runtime, loop *eventloop.EventLoop, target *goja.Object, ns store.NamespaceStore, options ...BindOption) error {
	if vm == nil {
		return errors.New("runtime is required")
	}
	if loop == nil {
		return errors.New("event loop is required")
	}
	if target == nil {
		return errors.New("target is required")
	}
	if ns == nil {
		return errors.New("store is required")
	}

	return bindObject(vm, loop, target, newBindingState(ns, options))
}

func BindSyncNamespace(vm *goja.Runtime, bindingName string, ns store.NamespaceStore, options ...BindOption) error {
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
	if err := BindSyncNamespaceObject(vm, object, ns, options...); err != nil {
		return err
	}
	return vm.Set(bindingName, object)
}

// BindSyncNamespaceObject binds the synchronous KV methods onto an existing
// JS object without creating a global binding.
func BindSyncNamespaceObject(vm *goja.Runtime, target *goja.Object, ns store.NamespaceStore, options ...BindOption) error {
	if vm == nil {
		return errors.New("runtime is required")
	}
	if target == nil {
		return errors.New("target is required")
	}
	if ns == nil {
		return errors.New("store is required")
	}

	return bindSyncObject(vm, target, newBindingState(ns, options))
}

func bindObject(vm *goja.Runtime, loop *eventloop.EventLoop, object *goja.Object, state *bindingState) error {
	if err := object.Set("get", func(call goja.FunctionCall) goja.Value {
		keys, bulk, err := parseGetKeys(vm, call.Argument(0))
		if err != nil {
			return rejectedPromise(vm, err)
		}
		if validationErr := state.validateKeys(keys, bulk); validationErr != nil {
			return rejectedPromise(vm, validationErr)
		}
		getOptions, err := parseGetOptions(vm, call.Argument(1))
		if err != nil {
			return rejectedPromise(vm, err)
		}
		valueType := getOptions.valueType
		cacheTTL, err := state.cacheTTL(getOptions.cacheTTL, getOptions.cacheTTLSupplied)
		if err != nil {
			return rejectedPromise(vm, err)
		}
		if bulk {
			if valueType != "text" && valueType != "json" {
				return rejectedPromise(vm, errors.New("bulk get supports only text and json types"))
			}
			return getManyPromise(vm, loop, state, keys, valueType, false, cacheTTL)
		}
		key := keys[0]

		return newPromise(vm, loop, func() (any, error) {
			return getStorageValueCached(context.Background(), state, key, valueType == "stream", cacheTTL)
		}, func(vm *goja.Runtime, raw any) (goja.Value, error) {
			result := raw.(storageGetResult)
			if !result.found {
				return goja.Null(), nil
			}
			if result.streamRecord == nil {
				if err := state.validateValueSize(int64(len(result.record.Value))); err != nil {
					return nil, err
				}
			}
			if valueType == "stream" {
				if result.streamRecord != nil {
					return streamRecordToReadableStream(vm, loop, *result.streamRecord, state.config.limits.MaxValueBytes)
				}
				return bytesToReadableStreamValue(vm, result.record.Value)
			}
			return toJSValue(vm, result.record.Value, valueType)
		})
	}); err != nil {
		return err
	}

	if err := object.Set("getWithMetadata", func(call goja.FunctionCall) goja.Value {
		keys, bulk, err := parseGetKeys(vm, call.Argument(0))
		if err != nil {
			return rejectedPromise(vm, err)
		}
		if validationErr := state.validateKeys(keys, bulk); validationErr != nil {
			return rejectedPromise(vm, validationErr)
		}
		getOptions, err := parseGetOptions(vm, call.Argument(1))
		if err != nil {
			return rejectedPromise(vm, err)
		}
		valueType := getOptions.valueType
		cacheTTL, err := state.cacheTTL(getOptions.cacheTTL, getOptions.cacheTTLSupplied)
		if err != nil {
			return rejectedPromise(vm, err)
		}
		if bulk {
			if valueType != "text" && valueType != "json" {
				return rejectedPromise(vm, errors.New("bulk getWithMetadata supports only text and json types"))
			}
			return getManyPromise(vm, loop, state, keys, valueType, true, cacheTTL)
		}
		key := keys[0]

		return newPromise(vm, loop, func() (any, error) {
			return getStorageValueCached(context.Background(), state, key, valueType == "stream", cacheTTL)
		}, func(vm *goja.Runtime, raw any) (goja.Value, error) {
			payload := raw.(storageGetResult)
			result := vm.NewObject()
			if !payload.found {
				_ = result.Set("value", goja.Null())
				_ = result.Set("metadata", goja.Null())
				return result, nil
			}
			if payload.streamRecord == nil {
				if validationErr := state.validateValueSize(int64(len(payload.record.Value))); validationErr != nil {
					return nil, validationErr
				}
			}

			var value goja.Value
			if valueType == "stream" {
				if payload.streamRecord != nil {
					value, err = streamRecordToReadableStream(vm, loop, *payload.streamRecord, state.config.limits.MaxValueBytes)
				} else {
					value, err = bytesToReadableStreamValue(vm, payload.record.Value)
				}
			} else {
				value, err = toJSValue(vm, payload.record.Value, valueType)
			}
			if err != nil {
				return nil, err
			}
			metadata, err := metadataToValue(vm, payload.metadata())
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
		if err := state.validateKey(key); err != nil {
			return rejectedPromise(vm, err)
		}
		if streams.IsReadableStream(vm, call.Argument(1)) {
			options, err := parsePutOptions(vm, call.Argument(2))
			if err != nil {
				return rejectedPromise(vm, err)
			}
			options.ValueKind = store.ValueKindBinary
			if err := state.validatePutOptions(options); err != nil {
				return rejectedPromise(vm, err)
			}
			if err := state.beginWrite(key); err != nil {
				return rejectedPromise(vm, err)
			}
			if putter, ok := state.ns.(store.StreamPutter); ok {
				return putReadableStreamWithCapability(vm, loop, putter, key, call.Argument(1), options, state.config.limits.MaxValueBytes, func() { state.cache.delete(key) })
			}
			return putReadableStreamPromise(vm, loop, state.ns, key, call.Argument(1), options, state.config.limits.MaxValueBytes, func() { state.cache.delete(key) })
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
		if err := state.validateValueSize(int64(len(value))); err != nil {
			return rejectedPromise(vm, err)
		}
		if err := state.validatePutOptions(options); err != nil {
			return rejectedPromise(vm, err)
		}
		if err := state.beginWrite(key); err != nil {
			return rejectedPromise(vm, err)
		}

		return newPromise(vm, loop, func() (any, error) {
			if err := state.ns.Put(context.Background(), key, value, options); err != nil {
				return nil, err
			}
			state.cache.delete(key)
			return nil, nil
		}, func(vm *goja.Runtime, raw any) (goja.Value, error) {
			return goja.Undefined(), nil
		})
	}); err != nil {
		return err
	}

	if err := object.Set("delete", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		if err := state.validateKey(key); err != nil {
			return rejectedPromise(vm, err)
		}
		return newPromise(vm, loop, func() (any, error) {
			if err := state.ns.Delete(context.Background(), key); err != nil {
				return nil, err
			}
			state.cache.delete(key)
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
		if err := state.validateListOptions(&options); err != nil {
			return rejectedPromise(vm, err)
		}

		return newPromise(vm, loop, func() (any, error) {
			result, err := state.ns.List(context.Background(), options)
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

func bindSyncObject(vm *goja.Runtime, object *goja.Object, state *bindingState) error {
	if err := object.Set("get", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		if err := state.validateKey(key); err != nil {
			panic(jsErrorValue(vm, err))
		}
		getOptions, err := parseGetOptions(vm, call.Argument(1))
		if err != nil {
			panic(jsErrorValue(vm, err))
		}
		valueType := getOptions.valueType
		cacheTTL, err := state.cacheTTL(getOptions.cacheTTL, getOptions.cacheTTLSupplied)
		if err != nil {
			panic(jsErrorValue(vm, err))
		}

		record, found, err := state.getRecordCached(context.Background(), key, cacheTTL)
		if err != nil {
			panic(jsErrorValue(vm, err))
		}
		if !found {
			return goja.Null()
		}
		if validationErr := state.validateValueSize(int64(len(record.Value))); validationErr != nil {
			panic(jsErrorValue(vm, validationErr))
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
		if err := state.validateKey(key); err != nil {
			panic(jsErrorValue(vm, err))
		}
		getOptions, err := parseGetOptions(vm, call.Argument(1))
		if err != nil {
			panic(jsErrorValue(vm, err))
		}
		valueType := getOptions.valueType
		cacheTTL, err := state.cacheTTL(getOptions.cacheTTL, getOptions.cacheTTLSupplied)
		if err != nil {
			panic(jsErrorValue(vm, err))
		}

		record, found, err := state.getRecordCached(context.Background(), key, cacheTTL)
		if err != nil {
			panic(jsErrorValue(vm, err))
		}

		result := vm.NewObject()
		if !found {
			_ = result.Set("value", goja.Null())
			_ = result.Set("metadata", goja.Null())
			return result
		}
		if validationErr := state.validateValueSize(int64(len(record.Value))); validationErr != nil {
			panic(jsErrorValue(vm, validationErr))
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
		if err := state.validateKey(key); err != nil {
			panic(jsErrorValue(vm, err))
		}
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
		if err := state.validateValueSize(int64(len(value))); err != nil {
			panic(jsErrorValue(vm, err))
		}
		if err := state.validatePutOptions(options); err != nil {
			panic(jsErrorValue(vm, err))
		}
		if err := state.beginWrite(key); err != nil {
			panic(jsErrorValue(vm, err))
		}

		if err := state.ns.Put(context.Background(), key, value, options); err != nil {
			panic(jsErrorValue(vm, err))
		}
		state.cache.delete(key)

		return goja.Undefined()
	}); err != nil {
		return err
	}

	if err := object.Set("delete", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		if err := state.validateKey(key); err != nil {
			panic(jsErrorValue(vm, err))
		}
		if err := state.ns.Delete(context.Background(), key); err != nil {
			panic(jsErrorValue(vm, err))
		}
		state.cache.delete(key)
		return goja.Undefined()
	}); err != nil {
		return err
	}

	if err := object.Set("list", func(call goja.FunctionCall) goja.Value {
		options, err := parseListOptions(vm, call.Argument(0))
		if err != nil {
			panic(jsErrorValue(vm, err))
		}
		if validationErr := state.validateListOptions(&options); validationErr != nil {
			panic(jsErrorValue(vm, validationErr))
		}

		result, err := state.ns.List(context.Background(), options)
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

type getOptions struct {
	valueType        string
	cacheTTL         time.Duration
	cacheTTLSupplied bool
}

func parseGetOptions(vm *goja.Runtime, argument goja.Value) (getOptions, error) {
	if isNilish(argument) {
		return getOptions{valueType: "text"}, nil
	}

	if exported, ok := argument.Export().(string); ok {
		valueType, err := normalizeValueType(exported)
		return getOptions{valueType: valueType}, err
	}

	object := argument.ToObject(vm)
	typeValue := object.Get("type")
	valueType := "text"
	var err error
	if !isNilish(typeValue) {
		exported, ok := typeValue.Export().(string)
		if !ok {
			return getOptions{}, errors.New("type option must be a string")
		}
		valueType, err = normalizeValueType(exported)
		if err != nil {
			return getOptions{}, err
		}
	}

	options := getOptions{valueType: valueType}
	cacheTTLValue := object.Get("cacheTtl")
	if !isNilish(cacheTTLValue) {
		seconds, err := numberToInt64(cacheTTLValue)
		if err != nil {
			return getOptions{}, fmt.Errorf("cacheTtl: %w", err)
		}
		if seconds < 0 {
			return getOptions{}, errors.New("cacheTtl must be non-negative")
		}
		options.cacheTTL = time.Duration(seconds) * time.Second
		options.cacheTTLSupplied = true
	}
	return options, nil
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
		if seconds <= 0 {
			return store.PutOptions{}, errors.New("expirationTtl must be positive")
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
	stream, err := streams.NewReadableStreamFromBytes(
		vm,
		value,
		readableStreamChunkSize,
		streams.WithChunkValue(streams.ArrayBufferChunk),
	)
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
	maximumBytes int64,
	onSuccess func(),
) goja.Value {
	promise, resolve, reject := vm.NewPromise()
	var buffer bytes.Buffer

	consumed, err := streams.ConsumeReadableStream(vm, streamValue, func(chunk goja.Value) goja.Value {
		bytes, chunkErr := streamChunkBytes(vm, chunk)
		if chunkErr != nil {
			panic(vm.ToValue(chunkErr.Error()))
		}
		if maximumBytes > 0 && int64(buffer.Len()+len(bytes)) > maximumBytes {
			panic(vm.ToValue(fmt.Sprintf("KV value exceeds the maximum size of %d bytes", maximumBytes)))
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
				if err == nil && onSuccess != nil {
					onSuccess()
				}
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
