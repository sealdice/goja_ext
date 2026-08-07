package node

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/abort"
	"github.com/sealdice/goja_ext/events"
	"github.com/sealdice/goja_ext/require"
	"github.com/sealdice/goja_ext/streams"
)

const (
	ModuleName = "stream"
	NodeModule = "node:stream"
)

var moduleSymbol = goja.NewSymbol("goja_ext.node_stream.module")

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
	global := rt.GlobalObject()
	if value := global.GetSymbol(moduleSymbol); value != nil &&
		!goja.IsUndefined(value) && !goja.IsNull(value) {
		if exports, ok := value.(*goja.Object); ok {
			return exports
		}
	}

	previousSelf := global.Get("self")
	self := rt.NewObject()
	abortModule := rt.NewObject()
	if err := abortModule.Set("exports", rt.NewObject()); err != nil {
		panic(err)
	}
	abort.Require(rt, abortModule)
	abortExports := abortModule.Get("exports").ToObject(rt)
	if err := self.Set("AbortController", abortExports.Get("AbortController")); err != nil {
		panic(err)
	}
	if err := self.Set("AbortSignal", abortExports.Get("AbortSignal")); err != nil {
		panic(err)
	}
	if err := global.Set("self", self); err != nil {
		panic(err)
	}
	if err := global.Set("__goja_ext_canonical_events", events.Exports(rt)); err != nil {
		panic(err)
	}
	if err := global.Set("__goja_ext_streams_canonical", canonicalStreams(rt)); err != nil {
		panic(err)
	}
	installQueueMicrotask(rt)

	value, err := rt.RunProgram(polyfillProgram)
	if previousSelf == nil || goja.IsUndefined(previousSelf) {
		_ = global.Delete("self")
	} else {
		_ = global.Set("self", previousSelf)
	}
	if err != nil {
		panic(fmt.Errorf("node streams: initialize polyfill: %w", err))
	}
	exports, ok := value.(*goja.Object)
	if !ok {
		panic("node streams: polyfill did not return an exports object")
	}
	if err := global.SetSymbol(moduleSymbol, exports); err != nil {
		panic(err)
	}
	return exports
}

func canonicalStreams(rt *goja.Runtime) *goja.Object {
	streamsExports := streams.Exports(rt)
	canonical := rt.NewObject()
	for _, name := range []string{
		"ReadableStream", "WritableStream", "TransformStream",
		"ReadableStreamDefaultReader", "WritableStreamDefaultWriter",
		"ReadableStreamBYOBReader", "ReadableStreamBYOBRequest",
		"ReadableByteStreamController", "ReadableStreamDefaultController",
		"WritableStreamDefaultController", "TransformStreamDefaultController",
		"ByteLengthQueuingStrategy", "CountQueuingStrategy",
	} {
		_ = canonical.Set(name, streamsExports.Get(name))
	}
	return canonical
}

// installQueueMicrotask ensures a queueMicrotask global exists so the bundled
// streamx uses goja's native microtask queue instead of process.nextTick.
func installQueueMicrotask(rt *goja.Runtime) {
	if value := rt.Get("queueMicrotask"); value != nil && !goja.IsUndefined(value) {
		return
	}
	_ = rt.Set("queueMicrotask", func(call goja.FunctionCall) goja.Value {
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

func init() {
	require.RegisterCoreModule(ModuleName, Require)
}
