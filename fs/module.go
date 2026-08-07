package fs

import (
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/eventloop"
	"github.com/sealdice/goja_ext/require"
	"github.com/sealdice/goja_ext/runtimehost"
)

const (
	ModuleName         = "fs"
	PromisesModuleName = "fs/promises"
)

type moduleInstance struct {
	rt      *goja.Runtime
	core    *Core
	loop    *eventloop.EventLoop
	streams bool
}

var (
	runtimeCoreKey            = runtimehost.NewKey("fs.core")
	runtimeFullExportsKey     = runtimehost.NewKey("fs.exports")
	runtimePromisesExportsKey = runtimehost.NewKey("fs.promises.exports")
	runtimeGlobalExportsKey   = runtimehost.NewKey("fs.global.exports")
)

type runtimeCoreState struct {
	core *Core
	cfg  config
	err  error
}

type runtimeExportsState struct {
	exports *goja.Object
	loop    *eventloop.EventLoop
	streams bool
}

func RequireWithOptions(opts ...Option) require.ModuleLoader {
	return requireWithOptions(nil, false, opts...)
}

func RequirePromisesWithOptions(opts ...Option) require.ModuleLoader {
	return requireWithOptions(nil, true, opts...)
}

func RequireWithLoop(loop *eventloop.EventLoop, opts ...Option) require.ModuleLoader {
	if loop == nil {
		panic("fs: event loop is required")
	}
	return requireWithOptions(loop, false, opts...)
}

func RequirePromisesWithLoop(loop *eventloop.EventLoop, opts ...Option) require.ModuleLoader {
	if loop == nil {
		panic("fs: event loop is required")
	}
	return requireWithOptions(loop, true, opts...)
}

func RegisterWithOptions(registry *require.Registry, opts ...Option) {
	registerLoaders(
		registry,
		RequireWithOptions(opts...),
		RequirePromisesWithOptions(opts...),
	)
}

func RegisterWithLoop(registry *require.Registry, loop *eventloop.EventLoop, opts ...Option) {
	registerLoaders(
		registry,
		RequireWithLoop(loop, opts...),
		RequirePromisesWithLoop(loop, opts...),
	)
}

func registerLoaders(
	registry *require.Registry,
	full require.ModuleLoader,
	promises require.ModuleLoader,
) {
	if registry == nil {
		panic("fs: registry is required")
	}
	registry.RegisterNativeModule(ModuleName, full)
	registry.RegisterNativeModule(require.NodePrefix+ModuleName, full)
	registry.RegisterNativeModule(PromisesModuleName, promises)
	registry.RegisterNativeModule(require.NodePrefix+PromisesModuleName, promises)
}

func requireWithOptions(loop *eventloop.EventLoop, promises bool, opts ...Option) require.ModuleLoader {
	options := append([]Option(nil), opts...)
	return func(rt *goja.Runtime, module *goja.Object) {
		core, err := coreForRuntime(rt, options...)
		if err != nil {
			panic(fmt.Errorf("fs: configure backend: %w", err))
		}
		cfg, err := configFromOptions(options)
		if err != nil {
			panic(fmt.Errorf("fs: configure exports: %w", err))
		}
		if cached, err := cachedExports(rt, promises, loop, cfg); err != nil {
			panic(err)
		} else if cached != nil {
			if err := module.Set("exports", cached); err != nil {
				panic(err)
			}
			return
		}
		instance := &moduleInstance{rt: rt, core: core, loop: loop, streams: cfg.withStream}
		exports := module.Get("exports").ToObject(rt)
		if promises {
			bindPromiseExports(instance, exports)
		} else {
			bindFullExports(instance, exports)
		}
		runtimehost.Store(rt, exportsKey(promises), &runtimeExportsState{
			exports: exports,
			loop:    loop,
			streams: cfg.withStream,
		})
	}
}

func cachedExports(rt *goja.Runtime, promises bool, loop *eventloop.EventLoop, cfg config) (*goja.Object, error) {
	value, ok := runtimehost.Load(rt, exportsKey(promises))
	if !ok {
		return nil, nil
	}
	state, ok := value.(*runtimeExportsState)
	if !ok {
		return nil, errors.New("fs: invalid cached exports state")
	}
	if state.loop != loop {
		return nil, errors.New("fs: exports already use a different scheduler")
	}
	if !promises && state.streams != cfg.withStream {
		return nil, errors.New("fs: exports already use a different stream mode")
	}
	return state.exports, nil
}

func exportsKey(promises bool) *runtimehost.Key {
	if promises {
		return runtimePromisesExportsKey
	}
	return runtimeFullExportsKey
}

func coreForRuntime(rt *goja.Runtime, opts ...Option) (*Core, error) {
	cfg, err := configFromOptions(opts)
	if err != nil {
		return nil, err
	}
	value := runtimehost.GetOrCreate(rt, runtimeCoreKey, func() any {
		core, createErr := newCoreFromConfig(cfg)
		return &runtimeCoreState{core: core, cfg: cfg, err: createErr}
	})
	state, ok := value.(*runtimeCoreState)
	if !ok {
		return nil, errors.New("fs: invalid runtime core state")
	}
	if state.err != nil {
		return nil, state.err
	}
	if conflict := coreConfigConflict(state.cfg, cfg); conflict != "" {
		return nil, fmt.Errorf("runtime already has a different %s", conflict)
	}
	if err := runtimehost.BindCwdProvider(rt, state.core); err != nil {
		return nil, fmt.Errorf("bind cwd provider: %w", err)
	}
	return state.core, nil
}

func Enable(rt *goja.Runtime, opts ...Option) error {
	return enable(rt, nil, opts...)
}

func EnableWithLoop(rt *goja.Runtime, loop *eventloop.EventLoop, opts ...Option) error {
	if loop == nil {
		return fmt.Errorf("fs: event loop is required")
	}
	return enable(rt, loop, opts...)
}

func enable(rt *goja.Runtime, loop *eventloop.EventLoop, opts ...Option) error {
	core, err := coreForRuntime(rt, opts...)
	if err != nil {
		return err
	}
	cfg, err := configFromOptions(opts)
	if err != nil {
		return err
	}
	if cached, err := cachedGlobalExports(rt, loop, cfg); err != nil {
		return err
	} else if cached != nil {
		return rt.Set("fs", cached)
	}
	instance := &moduleInstance{rt: rt, core: core, loop: loop, streams: cfg.withStream}
	exports := rt.NewObject()
	bindFullExports(instance, exports)
	runtimehost.Store(rt, runtimeGlobalExportsKey, &runtimeExportsState{
		exports: exports,
		loop:    loop,
		streams: cfg.withStream,
	})
	return rt.Set("fs", exports)
}

func cachedGlobalExports(rt *goja.Runtime, loop *eventloop.EventLoop, cfg config) (*goja.Object, error) {
	value, ok := runtimehost.Load(rt, runtimeGlobalExportsKey)
	if !ok {
		return nil, nil
	}
	state, ok := value.(*runtimeExportsState)
	if !ok {
		return nil, errors.New("fs: invalid cached global exports state")
	}
	if state.loop != loop {
		return nil, errors.New("fs: global exports already use a different scheduler")
	}
	if state.streams != cfg.withStream {
		return nil, errors.New("fs: global exports already use a different stream mode")
	}
	return state.exports, nil
}

func Require(rt *goja.Runtime, module *goja.Object) {
	loader := RequireWithOptions()
	loader(rt, module)
}

func RequirePromises(rt *goja.Runtime, module *goja.Object) {
	loader := RequirePromisesWithOptions()
	loader(rt, module)
}

func init() {
	require.RegisterCoreModule(ModuleName, Require)
	require.RegisterCoreModule(PromisesModuleName, RequirePromises)
}

func bindFullExports(instance *moduleInstance, exports *goja.Object) {
	setFunction(instance.rt, exports, "chdir", instance.chdir)
	setFunction(instance.rt, exports, "chmodSync", instance.chmodSync)
	setFunction(instance.rt, exports, "chmod", instance.chmod)
	setFunction(instance.rt, exports, "chownSync", instance.chownSync)
	setFunction(instance.rt, exports, "chown", instance.chown)
	setFunction(instance.rt, exports, "copyFileSync", instance.copyFileSync)
	setFunction(instance.rt, exports, "copyFile", instance.copyFile)
	setFunction(instance.rt, exports, "createSync", instance.createSync)
	setFunction(instance.rt, exports, "create", instance.create)
	setFunction(instance.rt, exports, "cwd", instance.cwd)
	setFunction(instance.rt, exports, "makeTempDirSync", instance.makeTempDirSync)
	setFunction(instance.rt, exports, "makeTempDir", instance.makeTempDir)
	setFunction(instance.rt, exports, "makeTempFileSync", instance.makeTempFileSync)
	setFunction(instance.rt, exports, "makeTempFile", instance.makeTempFile)
	setFunction(instance.rt, exports, "mkdirSync", instance.mkdirSync)
	setFunction(instance.rt, exports, "mkdir", instance.mkdir)
	setFunction(instance.rt, exports, "openSync", instance.openSync)
	setFunction(instance.rt, exports, "open", instance.open)
	setFunction(instance.rt, exports, "readDirSync", instance.readDirSync)
	setFunction(instance.rt, exports, "readDir", instance.readDir)
	setFunction(instance.rt, exports, "readFileSync", instance.readFileSync)
	setFunction(instance.rt, exports, "readFile", instance.readFile)
	setFunction(instance.rt, exports, "readTextFileSync", instance.readTextFileSync)
	setFunction(instance.rt, exports, "readTextFile", instance.readTextFile)
	setFunction(instance.rt, exports, "removeSync", instance.removeSync)
	setFunction(instance.rt, exports, "remove", instance.remove)
	setFunction(instance.rt, exports, "renameSync", instance.renameSync)
	setFunction(instance.rt, exports, "rename", instance.rename)
	setFunction(instance.rt, exports, "statSync", instance.statSync)
	setFunction(instance.rt, exports, "stat", instance.stat)
	setFunction(instance.rt, exports, "truncateSync", instance.truncateSync)
	setFunction(instance.rt, exports, "truncate", instance.truncate)
	setFunction(instance.rt, exports, "utimeSync", instance.utimeSync)
	setFunction(instance.rt, exports, "utime", instance.utime)
	setFunction(instance.rt, exports, "writeFileSync", instance.writeFileSync)
	setFunction(instance.rt, exports, "writeFile", instance.writeFile)
	setFunction(instance.rt, exports, "writeTextFileSync", instance.writeTextFileSync)
	setFunction(instance.rt, exports, "writeTextFile", instance.writeTextFile)
	_ = exports.Set("FsFile", newFsFileConstructor(instance.rt))
	bindNodeExports(instance, exports)
}

func bindPromiseExports(instance *moduleInstance, exports *goja.Object) {
	setFunction(instance.rt, exports, "chmod", instance.chmod)
	setFunction(instance.rt, exports, "chown", instance.chown)
	setFunction(instance.rt, exports, "copyFile", instance.copyFile)
	setFunction(instance.rt, exports, "create", instance.create)
	setFunction(instance.rt, exports, "makeTempDir", instance.makeTempDir)
	setFunction(instance.rt, exports, "makeTempFile", instance.makeTempFile)
	setFunction(instance.rt, exports, "mkdir", instance.mkdir)
	setFunction(instance.rt, exports, "open", instance.open)
	setFunction(instance.rt, exports, "readDir", instance.readDir)
	setFunction(instance.rt, exports, "readFile", instance.readFile)
	setFunction(instance.rt, exports, "readTextFile", instance.readTextFile)
	setFunction(instance.rt, exports, "remove", instance.remove)
	setFunction(instance.rt, exports, "rename", instance.rename)
	setFunction(instance.rt, exports, "stat", instance.stat)
	setFunction(instance.rt, exports, "truncate", instance.truncate)
	setFunction(instance.rt, exports, "utime", instance.utime)
	setFunction(instance.rt, exports, "writeFile", instance.writeFile)
	setFunction(instance.rt, exports, "writeTextFile", instance.writeTextFile)
}

func setFunction(rt *goja.Runtime, object *goja.Object, name string, fn func(goja.FunctionCall) goja.Value) {
	if err := object.Set(name, fn); err != nil {
		panic(err)
	}
}
