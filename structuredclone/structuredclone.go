package structuredclone

import (
	"strconv"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
)

const ModuleName = "structuredclone"

// Enable registers structuredClone as a global function.
func Enable(rt *goja.Runtime) {
	_ = rt.Set("structuredClone", structuredCloneFn(rt))
}

// Require exports structuredClone as the "structuredclone" core module.
func Require(rt *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)
	_ = exports.Set("structuredClone", structuredCloneFn(rt))
}

func init() {
	require.RegisterCoreModule(ModuleName, Require)
}

func structuredCloneFn(rt *goja.Runtime) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(rt.NewTypeError("structuredClone: value argument is required"))
		}
		val := call.Argument(0)
		seen := make(map[*goja.Object]goja.Value)
		return cloneValue(rt, val, seen)
	}
}

func cloneValue(rt *goja.Runtime, v goja.Value, seen map[*goja.Object]goja.Value) goja.Value {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return v
	}

	switch v.ExportType().String() {
	case "int", "int64", "uint", "uint64", "float32", "float64", "bool", "string":
		return v
	}

	obj, ok := v.(*goja.Object)
	if !ok {
		return v
	}

	if cached, ok := seen[obj]; ok {
		return cached
	}

	// goja reports ClassName()=="Object" for Map and Set instances, so we
	// detect them by constructor identity before dispatching. The routing is
	// required for Map/Set cloning to work; without it they fall through to the
	// plain-Object case. (Pathological inputs like Object.create(Map.prototype)
	// may be misrouted to an empty Map; acceptable for a polyfill.)
	className := obj.ClassName()
	if className == "Object" {
		if ctor := obj.Get("constructor"); ctor != nil {
			switch {
			case ctor.SameAs(rt.Get("Map")):
				className = "Map"
			case ctor.SameAs(rt.Get("Set")):
				className = "Set"
			}
		}
	}

	switch className {
	case "Array":
		n := int(obj.Get("length").ToInteger())
		arr := rt.NewArray()
		seen[obj] = arr
		for i := 0; i < n; i++ {
			_ = arr.Set(strconv.Itoa(i), cloneValue(rt, obj.Get(strconv.Itoa(i)), seen))
		}
		return arr
	case "Object", "goja.Object":
		newObj := rt.NewObject()
		seen[obj] = newObj
		for _, k := range obj.Keys() {
			_ = newObj.Set(k, cloneValue(rt, obj.Get(k), seen))
		}
		return newObj
	case "Map":
		mapCtor, ok := goja.AssertConstructor(rt.Get("Map"))
		if !ok {
			return jsonClone(rt, v)
		}
		newMap, err := mapCtor(nil)
		if err != nil {
			return jsonClone(rt, v)
		}
		seen[obj] = newMap
		setFn, _ := goja.AssertFunction(newMap.Get("set"))
		if entriesFn, ok := goja.AssertFunction(obj.Get("entries")); ok {
			if iter, err := entriesFn(obj); err == nil {
				if iterObj, ok := iter.(*goja.Object); ok {
					if next, ok := goja.AssertFunction(iterObj.Get("next")); ok {
						for {
							res, err := next(iterObj)
							if err != nil {
								break
							}
							resObj := res.ToObject(rt)
							if resObj.Get("done").ToBoolean() {
								break
							}
							entryObj := resObj.Get("value").ToObject(rt)
							k := cloneValue(rt, entryObj.Get("0"), seen)
							clonedV := cloneValue(rt, entryObj.Get("1"), seen)
							if setFn != nil {
								_, _ = setFn(newMap, k, clonedV)
							}
						}
					}
				}
			}
		}
		return newMap
	case "Set":
		setCtor, ok := goja.AssertConstructor(rt.Get("Set"))
		if !ok {
			return jsonClone(rt, v)
		}
		newSet, err := setCtor(nil)
		if err != nil {
			return jsonClone(rt, v)
		}
		seen[obj] = newSet
		addFn, _ := goja.AssertFunction(newSet.Get("add"))
		if valuesFn, ok := goja.AssertFunction(obj.Get("values")); ok {
			if iter, err := valuesFn(obj); err == nil {
				if iterObj, ok := iter.(*goja.Object); ok {
					if next, ok := goja.AssertFunction(iterObj.Get("next")); ok {
						for {
							res, err := next(iterObj)
							if err != nil {
								break
							}
							resObj := res.ToObject(rt)
							if resObj.Get("done").ToBoolean() {
								break
							}
							if addFn != nil {
								_, _ = addFn(newSet, cloneValue(rt, resObj.Get("value"), seen))
							}
						}
					}
				}
			}
		}
		return newSet
	case "Date":
		dateCtor, ok := goja.AssertConstructor(rt.Get("Date"))
		if !ok {
			return v
		}
		if getTime, ok := goja.AssertFunction(obj.Get("getTime")); ok {
			if t, err := getTime(obj); err == nil && t != nil {
				if newObj, err := dateCtor(nil, t); err == nil {
					seen[obj] = newObj
					return newObj
				}
			}
		}
		return v
	}

	return jsonClone(rt, v)
}

func jsonClone(rt *goja.Runtime, v goja.Value) goja.Value {
	jsonObj := rt.Get("JSON").ToObject(rt)
	stringify, _ := goja.AssertFunction(jsonObj.Get("stringify"))
	parse, _ := goja.AssertFunction(jsonObj.Get("parse"))
	if stringify == nil || parse == nil {
		return v
	}
	raw, err := stringify(goja.Undefined(), v)
	if err != nil {
		return v
	}
	cloned, err := parse(goja.Undefined(), raw)
	if err != nil {
		return v
	}
	return cloned
}
