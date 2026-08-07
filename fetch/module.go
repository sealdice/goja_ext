package fetch

import (
	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
	"github.com/sealdice/goja_ext/runtimehost"
)

const ModuleName = "fetch"

var exportsKey = runtimehost.NewKey("fetch.exports")

// Exports returns the canonical Fetch API constructors for rt.
func Exports(rt *goja.Runtime) *goja.Object {
	value := runtimehost.GetOrCreate(rt, exportsKey, func() any {
		exports := rt.NewObject()
		for name, constructor := range map[string]any{
			"Headers":  newHeadersCtor(rt),
			"Request":  newRequestCtor(rt),
			"Response": newResponseCtor(rt),
			"FormData": newFormDataCtor(rt),
		} {
			if err := exports.Set(name, constructor); err != nil {
				panic(err)
			}
		}
		return exports
	})
	return value.(*goja.Object)
}

// Enable registers Headers, Request, Response and FormData as globals.
// It does not register fetch itself — fetch needs an event loop and a resty
// client; use EnableFetch for that.
func Enable(rt *goja.Runtime) {
	exports := Exports(rt)
	for _, name := range []string{"Headers", "Request", "Response", "FormData"} {
		_ = rt.Set(name, exports.Get(name))
	}
}

// Require exports Headers, Request, Response and FormData as the "fetch" module.
func Require(rt *goja.Runtime, module *goja.Object) {
	if err := module.Set("exports", Exports(rt)); err != nil {
		panic(err)
	}
}

func init() {
	require.RegisterCoreModule(ModuleName, Require)
}
