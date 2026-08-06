package abort

import (
	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
)

const ModuleName = "abort"

// Enable registers AbortController and AbortSignal as globals.
func Enable(rt *goja.Runtime) {
	_ = rt.Set("AbortController", newAbortControllerCtor(rt))
	_ = rt.Set("AbortSignal", newAbortSignalStatic(rt))
}

// Require exports AbortController and AbortSignal as the "abort" module.
func Require(rt *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)
	_ = exports.Set("AbortController", newAbortControllerCtor(rt))
	_ = exports.Set("AbortSignal", newAbortSignalStatic(rt))
}

func init() {
	require.RegisterCoreModule(ModuleName, Require)
}
