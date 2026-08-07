package timers

import (
	_ "embed"
	"fmt"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
)

const (
	ModuleName        = "timers"
	PromisesModuleName = "timers/promises"
)

//go:embed promises.js
var promisesSource string

var promisesProgram = mustCompile()

func mustCompile() *goja.Program {
	source := `(function () {
		var module = { exports: {} };
` + promisesSource + `
		return module.exports;
	})()`
	program, err := goja.Compile("goja_ext/timers/promises.js", source, false)
	if err != nil {
		panic(fmt.Errorf("timers: compile promises: %w", err))
	}
	return program
}

func Require(rt *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)
	for _, name := range []string{
		"setTimeout", "setInterval", "setImmediate",
		"clearTimeout", "clearInterval", "clearImmediate",
	} {
		bindTimer(rt, exports, name)
	}
}

// bindTimer re-exports the eventloop-installed global timer. When the runtime
// has no event loop (no global timers), the exported function throws a clear
// error when invoked.
func bindTimer(rt *goja.Runtime, exports *goja.Object, name string) {
	if fn := rt.Get(name); fn != nil && !goja.IsUndefined(fn) {
		_ = exports.Set(name, fn)
		return
	}
	_ = exports.Set(name, func(call goja.FunctionCall) goja.Value {
		panic(rt.NewTypeError("timers require an event loop"))
	})
}

func RequirePromises(rt *goja.Runtime, module *goja.Object) {
	value, err := rt.RunProgram(promisesProgram)
	if err != nil {
		panic(fmt.Errorf("timers: initialize promises: %w", err))
	}
	if err := module.Set("exports", value); err != nil {
		panic(err)
	}
}

func init() {
	require.RegisterCoreModule(ModuleName, Require)
	require.RegisterCoreModule(PromisesModuleName, RequirePromises)
}
