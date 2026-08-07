package events

import (
	_ "embed"
	"fmt"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
)

const ModuleName = "events"

//go:embed events.js
var eventsSource string

var (
	moduleSymbol  = goja.NewSymbol("goja_ext.events.module")
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
// is shared by require("events"), require("node:events") and the streamx
// facade, guaranteeing constructor identity.
func Exports(rt *goja.Runtime) *goja.Object {
	global := rt.GlobalObject()
	if value := global.GetSymbol(moduleSymbol); value != nil &&
		!goja.IsUndefined(value) && !goja.IsNull(value) {
		if exports, ok := value.(*goja.Object); ok {
			return exports
		}
	}
	value, err := rt.RunProgram(eventsProgram)
	if err != nil {
		panic(fmt.Errorf("events: initialize: %w", err))
	}
	exports, ok := value.(*goja.Object)
	if !ok {
		panic("events: did not return an exports object")
	}
	if err := global.SetSymbol(moduleSymbol, exports); err != nil {
		panic(err)
	}
	return exports
}

// RegisterWithRegistry registers events and node:events on a specific registry.
func RegisterWithRegistry(registry *require.Registry) {
	registry.RegisterNativeModule(ModuleName, Require)
	registry.RegisterNativeModule(require.NodePrefix+ModuleName, Require)
}

func init() {
	require.RegisterCoreModule(ModuleName, Require)
}
