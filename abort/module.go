package abort

import (
	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
	"github.com/sealdice/goja_ext/runtimehost"
)

const ModuleName = "abort"

var exportsKey = runtimehost.NewKey("abort.exports")

// Exports returns the canonical Abort constructors for rt.
func Exports(rt *goja.Runtime) *goja.Object {
	value := runtimehost.GetOrCreate(rt, exportsKey, func() any {
		exports := rt.NewObject()
		if err := exports.Set("AbortController", newAbortControllerCtor(rt)); err != nil {
			panic(err)
		}
		if err := exports.Set("AbortSignal", newAbortSignalStatic(rt)); err != nil {
			panic(err)
		}
		return exports
	})
	return value.(*goja.Object)
}

// Enable registers AbortController and AbortSignal as globals.
func Enable(rt *goja.Runtime) {
	exports := Exports(rt)
	_ = rt.Set("AbortController", exports.Get("AbortController"))
	_ = rt.Set("AbortSignal", exports.Get("AbortSignal"))
}

// Require exports AbortController and AbortSignal as the "abort" module.
func Require(rt *goja.Runtime, module *goja.Object) {
	if err := module.Set("exports", Exports(rt)); err != nil {
		panic(err)
	}
}

func init() { //nolint:gochecknoinits // auto-register core module via blank import
	require.RegisterCoreModule(ModuleName, Require)
}
