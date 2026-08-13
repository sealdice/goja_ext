package structuredclone

import (
	"fmt"
	"strconv"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
)

const ModuleName = "structuredclone"

var typedArrayClasses = map[string]struct{}{
	"BigInt64Array":     {},
	"BigUint64Array":    {},
	"Float32Array":      {},
	"Float64Array":      {},
	"Int8Array":         {},
	"Int16Array":        {},
	"Int32Array":        {},
	"Uint8Array":        {},
	"Uint8ClampedArray": {},
	"Uint16Array":       {},
	"Uint32Array":       {},
}

// Enable registers structuredClone as a global function.
func Enable(rt *goja.Runtime) {
	_ = rt.Set("structuredClone", structuredCloneFn(rt))
}

// Require exports structuredClone as the "structuredclone" core module.
func Require(rt *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)
	_ = exports.Set("structuredClone", structuredCloneFn(rt))
}

func init() { //nolint:gochecknoinits // auto-register core module via blank import
	require.RegisterCoreModule(ModuleName, Require)
}

func structuredCloneFn(rt *goja.Runtime) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(rt.NewTypeError("structuredClone: value argument is required"))
		}
		return cloneValue(rt, call.Argument(0), make(map[*goja.Object]goja.Value))
	}
}

func cloneValue(rt *goja.Runtime, value goja.Value, seen map[*goja.Object]goja.Value) goja.Value {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return value
	}
	if _, isSymbol := value.(*goja.Symbol); isSymbol {
		throwDataCloneErrorf(rt, "%s could not be cloned", value.String())
	}

	obj, isObject := value.(*goja.Object)
	if !isObject {
		// Number, String, Boolean, and BigInt primitives are immutable values.
		return value
	}
	if _, callable := goja.AssertFunction(value); callable {
		throwDataCloneErrorf(rt, "%s could not be cloned", value.String())
	}
	if cached, ok := seen[obj]; ok {
		return cached
	}

	kind := objectKind(rt, obj)
	switch kind {
	case "Array":
		return cloneArray(rt, obj, seen)
	case "Object", "goja.Object":
		return clonePlainObject(rt, obj, seen)
	case "Map":
		return cloneMap(rt, obj, seen)
	case "Set":
		return cloneSet(rt, obj, seen)
	case "Date":
		return cloneDate(rt, obj, seen)
	case "RegExp":
		return cloneRegExp(rt, obj, seen)
	case "ArrayBuffer":
		return cloneArrayBuffer(rt, obj, seen)
	case "DataView":
		return cloneView(rt, obj, kind, seen)
	case "WeakMap", "WeakSet", "Promise":
		throwDataCloneErrorf(rt, "%s could not be cloned", kind)
	default:
		if _, typedArray := typedArrayClasses[kind]; typedArray {
			return cloneView(rt, obj, kind, seen)
		}
		throwDataCloneErrorf(rt, "%s objects are not supported", kind)
	}
	return nil
}

func objectKind(rt *goja.Runtime, obj *goja.Object) string {
	className := obj.ClassName()
	ctor := obj.Get("constructor")
	if ctor == nil {
		return className
	}
	for _, name := range []string{
		"Map", "Set", "WeakMap", "WeakSet", "Promise", "Date", "RegExp",
		"ArrayBuffer", "DataView", "BigInt64Array", "BigUint64Array",
		"Float32Array", "Float64Array", "Int8Array", "Int16Array", "Int32Array",
		"Uint8Array", "Uint8ClampedArray", "Uint16Array", "Uint32Array",
	} {
		candidate := rt.Get(name)
		if candidate != nil && !goja.IsUndefined(candidate) && ctor.SameAs(candidate) {
			return name
		}
	}
	return className
}

func cloneArray(rt *goja.Runtime, source *goja.Object, seen map[*goja.Object]goja.Value) goja.Value {
	length := int(source.Get("length").ToInteger())
	cloned := rt.NewArray()
	seen[source] = cloned
	for index := range length {
		key := strconv.Itoa(index)
		mustSet(rt, cloned, key, cloneValue(rt, source.Get(key), seen))
	}
	return cloned
}

func clonePlainObject(rt *goja.Runtime, source *goja.Object, seen map[*goja.Object]goja.Value) goja.Value {
	cloned := rt.NewObject()
	seen[source] = cloned
	for _, key := range source.Keys() {
		mustSet(rt, cloned, key, cloneValue(rt, source.Get(key), seen))
	}
	return cloned
}

func cloneMap(rt *goja.Runtime, source *goja.Object, seen map[*goja.Object]goja.Value) goja.Value {
	cloned := mustConstruct(rt, "Map")
	seen[source] = cloned
	set := mustFunction(rt, cloned.Get("set"), "Map.set")
	entries := mustFunction(rt, source.Get("entries"), "Map.entries")
	iterator, err := entries(source)
	if err != nil {
		panic(err)
	}
	next := mustFunction(rt, iterator.ToObject(rt).Get("next"), "Map iterator.next")
	for {
		result, err := next(iterator)
		if err != nil {
			panic(err)
		}
		resultObj := result.ToObject(rt)
		if resultObj.Get("done").ToBoolean() {
			break
		}
		entry := resultObj.Get("value").ToObject(rt)
		_, err = set(
			cloned,
			cloneValue(rt, entry.Get("0"), seen),
			cloneValue(rt, entry.Get("1"), seen),
		)
		if err != nil {
			panic(err)
		}
	}
	return cloned
}

func cloneSet(rt *goja.Runtime, source *goja.Object, seen map[*goja.Object]goja.Value) goja.Value {
	cloned := mustConstruct(rt, "Set")
	seen[source] = cloned
	add := mustFunction(rt, cloned.Get("add"), "Set.add")
	values := mustFunction(rt, source.Get("values"), "Set.values")
	iterator, err := values(source)
	if err != nil {
		panic(err)
	}
	next := mustFunction(rt, iterator.ToObject(rt).Get("next"), "Set iterator.next")
	for {
		result, err := next(iterator)
		if err != nil {
			panic(err)
		}
		resultObj := result.ToObject(rt)
		if resultObj.Get("done").ToBoolean() {
			break
		}
		_, err = add(cloned, cloneValue(rt, resultObj.Get("value"), seen))
		if err != nil {
			panic(err)
		}
	}
	return cloned
}

func cloneDate(rt *goja.Runtime, source *goja.Object, seen map[*goja.Object]goja.Value) goja.Value {
	getTime := mustFunction(rt, source.Get("getTime"), "Date.getTime")
	timestamp, err := getTime(source)
	if err != nil {
		panic(err)
	}
	cloned := mustConstruct(rt, "Date", timestamp)
	seen[source] = cloned
	return cloned
}

func cloneRegExp(rt *goja.Runtime, source *goja.Object, seen map[*goja.Object]goja.Value) goja.Value {
	cloned := mustConstruct(rt, "RegExp", source.Get("source"), source.Get("flags"))
	seen[source] = cloned
	return cloned
}

func cloneArrayBuffer(rt *goja.Runtime, source *goja.Object, seen map[*goja.Object]goja.Value) goja.Value {
	arrayBuffer, ok := source.Export().(goja.ArrayBuffer)
	if !ok || arrayBuffer.Detached() {
		throwDataCloneErrorf(rt, "ArrayBuffer could not be cloned")
	}
	data := append([]byte(nil), arrayBuffer.Bytes()...)
	cloned := rt.ToValue(rt.NewArrayBuffer(data)).ToObject(rt)
	seen[source] = cloned
	return cloned
}

func cloneView(
	rt *goja.Runtime,
	source *goja.Object,
	kind string,
	seen map[*goja.Object]goja.Value,
) goja.Value {
	buffer := source.Get("buffer")
	clonedBuffer := cloneValue(rt, buffer, seen)
	byteOffset := source.Get("byteOffset")
	var cloned *goja.Object
	if kind == "DataView" {
		cloned = mustConstruct(rt, kind, clonedBuffer, byteOffset, source.Get("byteLength"))
	} else {
		cloned = mustConstruct(rt, kind, clonedBuffer, byteOffset, source.Get("length"))
	}
	seen[source] = cloned
	return cloned
}

func mustConstruct(rt *goja.Runtime, name string, arguments ...goja.Value) *goja.Object {
	constructor, ok := goja.AssertConstructor(rt.Get(name))
	if !ok {
		throwDataCloneErrorf(rt, "%s constructor is unavailable", name)
	}
	object, err := constructor(nil, arguments...)
	if err != nil {
		panic(err)
	}
	return object
}

func mustFunction(rt *goja.Runtime, value goja.Value, name string) goja.Callable {
	function, ok := goja.AssertFunction(value)
	if !ok {
		throwDataCloneErrorf(rt, "%s is unavailable", name)
	}
	return function
}

func mustSet(rt *goja.Runtime, object *goja.Object, key string, value goja.Value) {
	if err := object.Set(key, value); err != nil {
		throwDataCloneErrorf(rt, "property %q could not be cloned: %v", key, err)
	}
}

func throwDataCloneErrorf(rt *goja.Runtime, format string, arguments ...interface{}) {
	err := rt.NewTypeError(fmt.Sprintf(format, arguments...))
	_ = err.Set("name", "DataCloneError")
	_ = err.Set("code", 25)
	panic(err)
}
