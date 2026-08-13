package url

import (
	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
	"github.com/sealdice/goja_ext/runtimehost"
)

const ModuleName = "url"

var exportsKey = runtimehost.NewKey("url.exports")

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
	must(module.Set("exports", Exports(runtime)))
}

// Exports returns the canonical URL module exports for runtime.
func Exports(runtime *goja.Runtime) *goja.Object {
	value := runtimehost.GetOrCreate(runtime, exportsKey, func() any {
		exports := runtime.NewObject()
		m := &urlModule{r: runtime}
		must(exports.Set("URL", m.createURLConstructor()))
		must(exports.Set("URLSearchParams", m.createURLSearchParamsConstructor()))
		must(exports.Set("domainToASCII", m.domainToASCII))
		must(exports.Set("domainToUnicode", m.domainToUnicode))
		return exports
	})
	return value.(*goja.Object)
}

func Enable(runtime *goja.Runtime) {
	m := Exports(runtime)
	must(runtime.Set("URL", m.Get("URL")))
	must(runtime.Set("URLSearchParams", m.Get("URLSearchParams")))
}

func init() { //nolint:gochecknoinits // auto-register core module via blank import
	require.RegisterCoreModule(ModuleName, Require)
}
