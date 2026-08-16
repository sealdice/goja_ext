package node

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/events"
	"github.com/dop251/goja_nodejs/require"
	"github.com/dop251/goja_nodejs/runtimehost"
	"github.com/dop251/goja_nodejs/streams"
)

const (
	ModuleName = "stream"
	NodeModule = "node:stream"
)

var moduleKey = runtimehost.NewKey("streams.node.exports")

// Require exports the canonical node classic stream object for this runtime.
func Require(rt *goja.Runtime, module *goja.Object) {
	if err := module.Set("exports", Exports(rt)); err != nil {
		panic(err)
	}
}

// Exports returns the canonical Node classic stream exports for rt. The same
// object is shared by require("stream"), require("node:stream") and, through
// the injected canonical events, the events module.
func Exports(rt *goja.Runtime) *goja.Object {
	value := runtimehost.GetOrCreate(rt, moduleKey, func() any {
		return initializePolyfill(rt)
	})
	return value.(*goja.Object)
}

func initializePolyfill(rt *goja.Runtime) *goja.Object {
	initializer, err := rt.RunProgram(polyfillProgram)
	if err != nil {
		panic(fmt.Errorf("node streams: load polyfill initializer: %w", err))
	}
	initialize, ok := goja.AssertFunction(initializer)
	if !ok {
		panic("node streams: polyfill did not return an initializer")
	}
	requireDependency := rt.ToValue(func(call goja.FunctionCall) goja.Value {
		switch call.Argument(0).String() {
		case "events":
			return events.Exports(rt)
		case "goja:stream/web":
			return streams.Exports(rt)
		default:
			panic(rt.NewTypeError("node streams: unresolved require: " + call.Argument(0).String()))
		}
	})
	value, err := initialize(goja.Undefined(), requireDependency, privateQueueMicrotask(rt))
	if err != nil {
		panic(fmt.Errorf("node streams: initialize polyfill: %w", err))
	}
	exports, ok := value.(*goja.Object)
	if !ok {
		panic("node streams: polyfill did not return an exports object")
	}
	return exports
}

func privateQueueMicrotask(rt *goja.Runtime) goja.Value {
	if value := rt.Get("queueMicrotask"); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
		return value
	}
	return rt.ToValue(func(call goja.FunctionCall) goja.Value {
		fn := call.Argument(0)
		if _, ok := goja.AssertFunction(fn); !ok {
			panic(rt.NewTypeError("queueMicrotask expects a function"))
		}
		resolveFn, ok := goja.AssertFunction(rt.Get("Promise").ToObject(rt).Get("resolve"))
		if !ok {
			return goja.Undefined()
		}
		promise, err := resolveFn(rt.Get("Promise").ToObject(rt))
		if err != nil {
			panic(err)
		}
		thenFn, ok := goja.AssertFunction(promise.ToObject(rt).Get("then"))
		if !ok {
			return goja.Undefined()
		}
		if _, err := thenFn(promise.ToObject(rt), fn); err != nil {
			panic(err)
		}
		return goja.Undefined()
	})
}

func init() { //nolint:gochecknoinits // auto-register core module via blank import
	require.RegisterCoreModule(ModuleName, Require)
}
