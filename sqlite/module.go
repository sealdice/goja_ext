package sqlite

import (
	_ "embed"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/buffer"
	hostfs "github.com/dop251/goja_nodejs/fs"
	"github.com/dop251/goja_nodejs/require"
	"github.com/dop251/goja_nodejs/runtimehost"
)

const ModuleName = "sqlite"

//go:embed facade.js
var facadeSource string

var (
	moduleStateKey = runtimehost.NewKey("sqlite.exports")
	facadeProgram  = mustCompileFacade()
)

type moduleState struct {
	core    *hostfs.Core
	binding *runtimeBinding
	exports *goja.Object
}

func mustCompileFacade() *goja.Program {
	program, err := goja.Compile("goja_ext/sqlite/facade.js", facadeSource, false)
	if err != nil {
		panic(fmt.Errorf("sqlite: compile facade: %w", err))
	}
	return program
}

func RequireWithOptions(opts ...hostfs.Option) require.ModuleLoader {
	options := append([]hostfs.Option(nil), opts...)
	return func(rt *goja.Runtime, module *goja.Object) {
		exports, err := exportsForRuntime(rt, options...)
		if err != nil {
			panic(fmt.Errorf("sqlite: %w", err))
		}
		if err := module.Set("exports", exports); err != nil {
			panic(err)
		}
	}
}

func RegisterWithOptions(registry *require.Registry, opts ...hostfs.Option) {
	if registry == nil {
		panic("sqlite: registry is required")
	}
	loader := RequireWithOptions(opts...)
	registry.RegisterNativeModule(ModuleName, loader)
	registry.RegisterNativeModule(require.NodePrefix+ModuleName, loader)
}

func RegisterWithRegistry(registry *require.Registry) {
	RegisterWithOptions(registry)
}

func Require(rt *goja.Runtime, module *goja.Object) {
	RequireWithOptions()(rt, module)
}

func Enable(rt *goja.Runtime, opts ...hostfs.Option) error {
	exports, err := exportsForRuntime(rt, opts...)
	if err != nil {
		return err
	}
	return rt.Set(ModuleName, exports)
}

func exportsForRuntime(rt *goja.Runtime, opts ...hostfs.Option) (*goja.Object, error) {
	core, err := hostfs.EnsureCore(rt, opts...)
	if err != nil {
		return nil, err
	}
	if value, ok := runtimehost.Load(rt, moduleStateKey); ok {
		state, ok := value.(*moduleState)
		if !ok {
			return nil, errors.New("invalid module state")
		}
		if state.core != core {
			return nil, errors.New("runtime already has a different filesystem core")
		}
		return state.exports, nil
	}

	binding := newRuntimeBinding(rt, core)
	buffer.Enable(rt)
	initializerValue, err := rt.RunProgram(facadeProgram)
	if err != nil {
		return nil, fmt.Errorf("initialize facade: %w", err)
	}
	initializer, ok := goja.AssertFunction(initializerValue)
	if !ok {
		return nil, errors.New("facade did not return an initializer")
	}
	value, err := initializer(goja.Undefined(), binding.exports())
	if err != nil {
		return nil, fmt.Errorf("initialize exports: %w", err)
	}
	exports, ok := value.(*goja.Object)
	if !ok {
		return nil, errors.New("facade did not return an exports object")
	}
	runtimehost.Store(rt, moduleStateKey, &moduleState{core: core, binding: binding, exports: exports})
	return exports, nil
}

func init() { //nolint:gochecknoinits // auto-register core module via blank import
	require.RegisterCoreModule(ModuleName, Require)
}
