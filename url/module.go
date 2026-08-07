package url

import (
	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
)

const ModuleName = "url"

type urlModule struct {
	r *goja.Runtime

	URLSearchParamsPrototype         *goja.Object
	URLSearchParamsIteratorPrototype *goja.Object
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func Require(runtime *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)
	m := &urlModule{
		r: runtime,
	}
	must(exports.Set("URL", m.createURLConstructor()))
	must(exports.Set("URLSearchParams", m.createURLSearchParamsConstructor()))
	must(exports.Set("domainToASCII", m.domainToASCII))
	must(exports.Set("domainToUnicode", m.domainToUnicode))
}

func Enable(runtime *goja.Runtime) {
	m := require.Require(runtime, ModuleName).ToObject(runtime)
	must(runtime.Set("URL", m.Get("URL")))
	must(runtime.Set("URLSearchParams", m.Get("URLSearchParams")))
}

func init() {
	require.RegisterCoreModule(ModuleName, Require)
}
