package fetch

import (
	_ "embed"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/buffer"
	"github.com/dop251/goja_nodejs/streams"
	weburl "github.com/dop251/goja_nodejs/url"
)

//go:embed internal/bare/bundle.js
var bareFetchSource string

var bareFetchProgram = mustCompileBareFetch()

func mustCompileBareFetch() *goja.Program {
	source := `(function (Buffer, URL, URLSearchParams, require) {
		var module = { exports: {} };
		var exports = module.exports;
` + bareFetchSource + `
		return module.exports;
	})`
	program, err := goja.Compile("bare-fetch@3.2.0/facade.js", source, false)
	if err != nil {
		panic(fmt.Errorf("fetch: compile embedded bare-fetch facade: %w", err))
	}
	return program
}

func initializeBareFetch(rt *goja.Runtime) *goja.Object {
	initializerValue, err := rt.RunProgram(bareFetchProgram)
	if err != nil {
		panic(fmt.Errorf("fetch: load embedded bare-fetch facade: %w", err))
	}
	initializer, ok := goja.AssertFunction(initializerValue)
	if !ok {
		panic("fetch: embedded bare-fetch facade did not return an initializer")
	}

	bufferExports := buffer.Exports(rt)
	urlExports := weburl.Exports(rt)
	streamExports := streams.Exports(rt)
	requireFn := func(call goja.FunctionCall) goja.Value {
		switch call.Argument(0).String() {
		case "goja:stream/web":
			return newStreamWebShim(rt, streamExports)
		case "goja:url":
			return newURLShim(rt, urlExports)
		case "goja:buffer":
			return newBufferShim(rt, bufferExports)
		default:
			panic(rt.NewTypeError("fetch: unknown embedded module %q", call.Argument(0).String()))
		}
	}
	value, err := initializer(
		goja.Undefined(),
		bufferExports.Get("Buffer"),
		urlExports.Get("URL"),
		urlExports.Get("URLSearchParams"),
		rt.ToValue(requireFn),
	)
	if err != nil {
		panic(fmt.Errorf("fetch: initialize embedded bare-fetch facade: %w", err))
	}
	exports, ok := value.(*goja.Object)
	if !ok {
		panic("fetch: embedded bare-fetch facade did not return exports")
	}
	return exports
}

func newStreamWebShim(rt *goja.Runtime, canonical *goja.Object) *goja.Object {
	shim := rt.NewObject()
	for _, name := range canonical.Keys() {
		if err := shim.Set(name, canonical.Get(name)); err != nil {
			panic(err)
		}
	}
	if err := shim.Set("isReadableStream", func(call goja.FunctionCall) goja.Value {
		return rt.ToValue(streams.IsReadableStream(rt, call.Argument(0)))
	}); err != nil {
		panic(err)
	}
	if err := shim.Set("isReadableStreamDisturbed", func(call goja.FunctionCall) goja.Value {
		value := call.Argument(0)
		if !streams.IsReadableStream(rt, value) {
			return rt.ToValue(false)
		}
		return rt.ToValue(value.ToObject(rt).Get("_disturbed").ToBoolean())
	}); err != nil {
		panic(err)
	}
	return shim
}

func newURLShim(rt *goja.Runtime, canonical *goja.Object) *goja.Object {
	shim := rt.NewObject()
	urlCtor := canonical.Get("URL")
	paramsCtor := canonical.Get("URLSearchParams")
	_ = shim.Set("URL", urlCtor)
	_ = shim.Set("URLSearchParams", paramsCtor)
	_ = shim.Set("isURL", func(call goja.FunctionCall) goja.Value {
		return rt.ToValue(instanceOf(rt, call.Argument(0), urlCtor))
	})
	_ = shim.Set("isURLSearchParams", func(call goja.FunctionCall) goja.Value {
		return rt.ToValue(instanceOf(rt, call.Argument(0), paramsCtor))
	})
	return shim
}

func newBufferShim(rt *goja.Runtime, canonical *goja.Object) *goja.Object {
	shim := rt.NewObject()
	bufferCtor := canonical.Get("Buffer")
	uint8ArrayCtor := rt.Get("Uint8Array")
	_ = shim.Set("isBuffer", func(call goja.FunctionCall) goja.Value {
		value := call.Argument(0)
		return rt.ToValue(instanceOf(rt, value, bufferCtor) || instanceOf(rt, value, uint8ArrayCtor))
	})
	return shim
}

func instanceOf(rt *goja.Runtime, value, constructor goja.Value) bool {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return false
	}
	var matches bool
	if exception := rt.Try(func() {
		matches = rt.InstanceOf(value, constructor.ToObject(rt))
	}); exception != nil {
		return false
	}
	return matches
}
