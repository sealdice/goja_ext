package process

import (
	"os"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	"github.com/dop251/goja_nodejs/runtimehost"
)

const ModuleName = "process"

type Process struct {
	env map[string]string
}

var exportsKey = runtimehost.NewKey("process.exports")

func Require(runtime *goja.Runtime, module *goja.Object) {
	if err := module.Set("exports", Exports(runtime)); err != nil {
		panic(err)
	}
}

// Exports returns the canonical process object for runtime.
func Exports(runtime *goja.Runtime) *goja.Object {
	value := runtimehost.GetOrCreate(runtime, exportsKey, func() any {
		p := &Process{
			env: make(map[string]string),
		}

		for _, e := range os.Environ() {
			envKeyValue := strings.SplitN(e, "=", 2)
			p.env[envKeyValue[0]] = envKeyValue[1]
		}

		o := runtime.NewObject()
		if err := o.Set("env", p.env); err != nil {
			panic(err)
		}
		if err := o.Set("cwd", func(goja.FunctionCall) goja.Value {
			return runtime.ToValue(runtimehost.Cwd(runtime))
		}); err != nil {
			panic(err)
		}
		if err := o.Set("chdir", func(call goja.FunctionCall) goja.Value {
			directory := call.Argument(0)
			if directory == nil || goja.IsUndefined(directory) || goja.IsNull(directory) {
				panic(runtime.NewTypeError("process.chdir requires a directory"))
			}
			if err := runtimehost.Chdir(runtime, directory.String()); err != nil {
				panic(runtime.NewGoError(err))
			}
			return goja.Undefined()
		}); err != nil {
			panic(err)
		}
		return o
	})
	return value.(*goja.Object)
}

func Enable(runtime *goja.Runtime) {
	if err := runtime.Set("process", Exports(runtime)); err != nil {
		panic(err)
	}
}

func init() { //nolint:gochecknoinits // auto-register core module via blank import
	require.RegisterCoreModule(ModuleName, Require)
}
