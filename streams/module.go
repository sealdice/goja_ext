package streams

import (
	_ "embed"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	"github.com/dop251/goja_nodejs/runtimehost"
)

const (
	ModuleName      = "streams"
	StreamWebModule = "stream/web"
	PolyfillVersion = "4.3.0"
)

var exportNames = [...]string{
	"ByteLengthQueuingStrategy",
	"CountQueuingStrategy",
	"ReadableByteStreamController",
	"ReadableStream",
	"ReadableStreamBYOBReader",
	"ReadableStreamBYOBRequest",
	"ReadableStreamDefaultController",
	"ReadableStreamDefaultReader",
	"TransformStream",
	"TransformStreamDefaultController",
	"WritableStream",
	"WritableStreamDefaultController",
	"WritableStreamDefaultWriter",
}

//go:embed text_streams.js
var textStreamsSource string

var moduleKey = runtimehost.NewKey("streams.exports")

//go:embed internal/polyfill/ponyfill.js
var ponyfillSource string

var ponyfillProgram = mustCompilePonyfill()

func mustCompilePonyfill() *goja.Program {
	source := `(function () {
		var module = { exports: {} };
		var exports = module.exports;
` + ponyfillSource + `
		return module.exports;
	})()`
	program, err := goja.Compile(
		"web-streams-polyfill@"+PolyfillVersion+"/ponyfill.js",
		source,
		false,
	)
	if err != nil {
		panic(fmt.Errorf("streams: compile embedded polyfill: %w", err))
	}
	return program
}

// Enable installs all Web Streams constructors on the runtime global object.
func Enable(rt *goja.Runtime) {
	exports := getExports(rt)
	for _, name := range exportNames {
		if err := rt.Set(name, exports.Get(name)); err != nil {
			panic(err)
		}
	}
	for _, name := range textExportNames {
		if err := rt.Set(name, exports.Get(name)); err != nil {
			panic(err)
		}
	}
}

// Require exports the canonical Web Streams constructors for this runtime.
func Require(rt *goja.Runtime, module *goja.Object) {
	if err := module.Set("exports", getExports(rt)); err != nil {
		panic(err)
	}
}

// Exports returns the canonical module exports for rt.
func Exports(rt *goja.Runtime) *goja.Object {
	return getExports(rt)
}

// Initialized reports whether Web Streams have already been initialized for
// rt without initializing them as a side effect.
func Initialized(rt *goja.Runtime) bool {
	_, ok := runtimehost.Load(rt, moduleKey)
	return ok
}

func getExports(rt *goja.Runtime) *goja.Object {
	if value, ok := runtimehost.Load(rt, moduleKey); ok {
		exports := value.(*goja.Object)
		ensureTextExports(rt, exports)
		return exports
	}

	ensureAsyncIteratorSymbol(rt)

	value, err := rt.RunProgram(ponyfillProgram)
	if err != nil {
		panic(fmt.Errorf("streams: initialize embedded polyfill: %w", err))
	}
	exports, ok := value.(*goja.Object)
	if !ok {
		panic("streams: embedded polyfill did not return an exports object")
	}
	for _, name := range exportNames {
		if value := exports.Get(name); value == nil || goja.IsUndefined(value) {
			panic(fmt.Sprintf("streams: embedded polyfill is missing export %q", name))
		}
	}
	ensureTextExports(rt, exports)
	runtimehost.Store(rt, moduleKey, exports)
	return exports
}

func ensureTextExports(rt *goja.Runtime, exports *goja.Object) {
	if value := exports.Get(textExportNames[0]); value != nil && !goja.IsUndefined(value) {
		return
	}

	initializer, err := rt.RunProgram(textStreamsProgram)
	if err != nil {
		panic(fmt.Errorf("streams: load text stream initializer: %w", err))
	}
	initialize, ok := goja.AssertFunction(initializer)
	if !ok {
		panic("streams: text stream program did not return an initializer")
	}
	encode := rt.ToValue(func(call goja.FunctionCall) goja.Value {
		return encodeUTF8(rt, call)
	})
	decode := rt.ToValue(func(call goja.FunctionCall) goja.Value {
		return decodeUTF8(rt, call)
	})
	value, err := initialize(goja.Undefined(), exports.Get("TransformStream"), encode, decode)
	if err != nil {
		panic(fmt.Errorf("streams: initialize text streams: %w", err))
	}
	textExports, ok := value.(*goja.Object)
	if !ok {
		panic("streams: text streams did not return an exports object")
	}
	for _, name := range textExportNames {
		value := textExports.Get(name)
		if value == nil || goja.IsUndefined(value) {
			panic(fmt.Sprintf("streams: text streams are missing export %q", name))
		}
		if err := exports.Set(name, value); err != nil {
			panic(err)
		}
	}
}

// ensureAsyncIteratorSymbol defines Symbol.asyncIterator when goja omits it, so
// bundled code that falls back to Symbol.for("Symbol.asyncIterator") and code
// that reads Symbol.asyncIterator agree on the same well-known symbol.
func ensureAsyncIteratorSymbol(rt *goja.Runtime) {
	symbolObj := rt.Get("Symbol").ToObject(rt)
	if value := symbolObj.Get("asyncIterator"); value != nil && !goja.IsUndefined(value) {
		return
	}
	symbolFor, ok := goja.AssertFunction(symbolObj.Get("for"))
	if !ok {
		return
	}
	sym, err := symbolFor(symbolObj, rt.ToValue("Symbol.asyncIterator"))
	if err == nil {
		_ = symbolObj.Set("asyncIterator", sym)
	}
}

var textStreamsProgram = mustCompileTextStreams()

func mustCompileTextStreams() *goja.Program {
	program, err := goja.Compile("goja_ext/streams/text_streams.js", textStreamsSource, false)
	if err != nil {
		panic(fmt.Errorf("streams: compile text streams: %w", err))
	}
	return program
}

func init() { //nolint:gochecknoinits // auto-register core module via blank import
	require.RegisterNativeModule(ModuleName, Require)
	require.RegisterCoreModule(StreamWebModule, Require)
}
