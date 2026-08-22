package events

import (
	_ "embed"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	"github.com/dop251/goja_nodejs/runtimehost"
)

const ModuleName = "events"

//go:embed events.js
var eventsSource string

var (
	moduleKey     = runtimehost.NewKey("events.exports")
	eventsProgram = mustCompile()
)

func mustCompile() *goja.Program {
	source := `(function () {
		var module = { exports: {} };
		var exports = module.exports;
` + eventsSource + `
		return module.exports;
	})()`
	program, err := goja.Compile("goja_ext/events/events.js", source, false)
	if err != nil {
		panic(fmt.Errorf("events: compile: %w", err))
	}
	return program
}

func Require(rt *goja.Runtime, module *goja.Object) {
	if err := module.Set("exports", Exports(rt)); err != nil {
		panic(err)
	}
}

// Exports returns the canonical events module exports for rt. The same object
// is shared by require("events"), require("node:events") and host integrations.
func Exports(rt *goja.Runtime) *goja.Object {
	if value, ok := runtimehost.Load(rt, moduleKey); ok {
		return value.(*goja.Object)
	}
	ensureAsyncIteratorSymbol(rt)
	value, err := rt.RunProgram(eventsProgram)
	if err != nil {
		panic(fmt.Errorf("events: initialize: %w", err))
	}
	exports, ok := value.(*goja.Object)
	if !ok {
		panic("events: did not return an exports object")
	}
	runtimehost.Store(rt, moduleKey, exports)
	return exports
}

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

// RegisterWithRegistry registers events and node:events on a specific registry.
func RegisterWithRegistry(registry *require.Registry) {
	registry.RegisterNativeModule(ModuleName, Require)
	registry.RegisterNativeModule(require.NodePrefix+ModuleName, Require)
}

func init() { //nolint:gochecknoinits // auto-register core module via blank import
	require.RegisterCoreModule(ModuleName, Require)
}
