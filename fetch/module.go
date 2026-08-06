package fetch

import (
	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
)

const ModuleName = "fetch"

// Enable registers Headers, Request, Response and FormData as globals.
// It does not register fetch itself — fetch needs an event loop and a resty
// client; use EnableFetch for that.
func Enable(rt *goja.Runtime) {
	_ = rt.Set("Headers", newHeadersCtor(rt))
	_ = rt.Set("Request", newRequestCtor(rt))
	_ = rt.Set("Response", newResponseCtor(rt))
	_ = rt.Set("FormData", newFormDataCtor(rt))
}

// Require exports Headers, Request, Response and FormData as the "fetch" module.
func Require(rt *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)
	_ = exports.Set("Headers", newHeadersCtor(rt))
	_ = exports.Set("Request", newRequestCtor(rt))
	_ = exports.Set("Response", newResponseCtor(rt))
	_ = exports.Set("FormData", newFormDataCtor(rt))
}

func init() {
	require.RegisterCoreModule(ModuleName, Require)
}
