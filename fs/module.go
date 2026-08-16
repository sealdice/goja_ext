package fs

import (
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/require"
	"github.com/dop251/goja_nodejs/runtimehost"
)

const (
	ModuleName         = "fs"
	PromisesModuleName = "fs/promises"
)

type moduleInstance struct {
	rt        *goja.Runtime
	core      *Core
	scheduler runtimehost.Scheduler
	streams   bool
}

var (
	runtimeCoreKey            = runtimehost.NewKey("fs.core")
	runtimeFullExportsKey     = runtimehost.NewKey("fs.exports")
	runtimePromisesExportsKey = runtimehost.NewKey("fs.promises.exports")
	runtimeGlobalExportsKey   = runtimehost.NewKey("fs.global.exports")

	errNoExports = errors.New("fs: exports not yet initialized")
)

type runtimeCoreState struct {
	core *Core
	cfg  config
	err  error
}

type runtimeExportsState struct {
	exports   *goja.Object
	scheduler runtimehost.Scheduler
	streams   bool
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
	registry.RegisterNativeModule(PromisesModuleName, promises)
}

func requireWithOptions(loop *eventloop.EventLoop, promises bool, opts ...Option) require.ModuleLoader {
	options := append([]Option(nil), opts...)
	return func(rt *goja.Runtime, module *goja.Object) {
		scheduler, err := schedulerForRuntime(rt, loop)
		if err != nil {
			panic(fmt.Errorf("fs: %w", err))
		}
		core, err := coreForRuntime(rt, options...)
		if err != nil {
			panic(fmt.Errorf("fs: configure backend: %w", err))
		}
		cfg, err := configFromOptions(options)
		if err != nil {
			panic(fmt.Errorf("fs: configure exports: %w", err))
		}
		if cached, err := cachedExports(rt, promises, scheduler, cfg); err != nil {
			if !errors.Is(err, errNoExports) {
				panic(err)
			}
		} else if cached != nil {
			if err := module.Set("exports", cached); err != nil {
				panic(err)
			}
			return
		}
		instance := &moduleInstance{rt: rt, core: core, scheduler: scheduler, streams: cfg.withStream}
		exports := module.Get("exports").ToObject(rt)
		if promises {
			bindPromiseExports(instance, exports)
		} else {
			bindFullExports(instance, exports)
		}
		runtimehost.Store(rt, exportsKey(promises), &runtimeExportsState{
			exports:   exports,
			scheduler: scheduler,
			streams:   cfg.withStream,
		})
	}
}

func cachedExports(rt *goja.Runtime, promises bool, scheduler runtimehost.Scheduler, cfg config) (*goja.Object, error) {
	value, ok := runtimehost.Load(rt, exportsKey(promises))
	if !ok {
		return nil, errNoExports
	}
	state, ok := value.(*runtimeExportsState)
	if !ok {
		return nil, errors.New("fs: invalid cached exports state")
	}
	if state.scheduler != scheduler {
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

// EnsureCore returns the canonical Afero-backed Core associated with rt.
// Modules that share filesystem state should use this entry point instead of
// constructing a separate Core for the same runtime.
func EnsureCore(rt *goja.Runtime, opts ...Option) (*Core, error) {
	if len(opts) == 0 {
		if value, ok := runtimehost.Load(rt, runtimeCoreKey); ok {
			state, ok := value.(*runtimeCoreState)
			if !ok {
				return nil, errors.New("fs: invalid runtime core state")
			}
			if state.err != nil {
				return nil, state.err
			}
			if err := runtimehost.BindCwdProvider(rt, state.core); err != nil {
				return nil, fmt.Errorf("bind cwd provider: %w", err)
			}
			return state.core, nil
		}
	}
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

func coreForRuntime(rt *goja.Runtime, opts ...Option) (*Core, error) {
	return EnsureCore(rt, opts...)
}

func Enable(rt *goja.Runtime, opts ...Option) error {
	return enable(rt, nil, opts...)
}

func EnableWithLoop(rt *goja.Runtime, loop *eventloop.EventLoop, opts ...Option) error {
	if loop == nil {
		return errors.New("fs: event loop is required")
	}
	return enable(rt, loop, opts...)
}

func enable(rt *goja.Runtime, loop *eventloop.EventLoop, opts ...Option) error {
	scheduler, err := schedulerForRuntime(rt, loop)
	if err != nil {
		return fmt.Errorf("fs: %w", err)
	}
	core, err := coreForRuntime(rt, opts...)
	if err != nil {
		return err
	}
	cfg, err := configFromOptions(opts)
	if err != nil {
		return err
	}
	if cached, err := cachedGlobalExports(rt, scheduler, cfg); err != nil {
		if !errors.Is(err, errNoExports) {
			return err
		}
	} else if cached != nil {
		return rt.Set("fs", cached)
	}
	instance := &moduleInstance{rt: rt, core: core, scheduler: scheduler, streams: cfg.withStream}
	exports := rt.NewObject()
	bindDenoExports(instance, exports)
	runtimehost.Store(rt, runtimeGlobalExportsKey, &runtimeExportsState{
		exports:   exports,
		scheduler: scheduler,
		streams:   cfg.withStream,
	})
	return rt.Set("fs", exports)
}

func cachedGlobalExports(rt *goja.Runtime, scheduler runtimehost.Scheduler, cfg config) (*goja.Object, error) {
	value, ok := runtimehost.Load(rt, runtimeGlobalExportsKey)
	if !ok {
		return nil, errNoExports
	}
	state, ok := value.(*runtimeExportsState)
	if !ok {
		return nil, errors.New("fs: invalid cached global exports state")
	}
	if state.scheduler != scheduler {
		return nil, errors.New("fs: global exports already use a different scheduler")
	}
	if state.streams != cfg.withStream {
		return nil, errors.New("fs: global exports already use a different stream mode")
	}
	return state.exports, nil
}

func schedulerForRuntime(rt *goja.Runtime, loop *eventloop.EventLoop) (runtimehost.Scheduler, error) {
	if loop == nil {
		return nil, nil //nolint:nilnil // nil scheduler means synchronous mode (no event loop)
	}
	if err := runtimehost.ValidateScheduler(rt, loop); err != nil {
		return nil, err
	}
	if err := runtimehost.BindScheduler(rt, loop); err != nil {
		return nil, err
	}
	return loop, nil
}

func Require(rt *goja.Runtime, module *goja.Object) {
	loader := RequireWithOptions()
	loader(rt, module)
}

func RequirePromises(rt *goja.Runtime, module *goja.Object) {
	loader := RequirePromisesWithOptions()
	loader(rt, module)
}

func init() { //nolint:gochecknoinits // auto-register core module via blank import
	require.RegisterNativeModule(ModuleName, Require)
	require.RegisterNativeModule(PromisesModuleName, RequirePromises)
}

func bindFullExports(instance *moduleInstance, exports *goja.Object) {
	bindDenoExports(instance, exports)
}

func bindDenoExports(instance *moduleInstance, exports *goja.Object) {
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
	bindDenoExtraExports(instance, exports)
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
	bindDenoPromiseExtraExports(instance, exports)
}

func bindDenoExtraExports(instance *moduleInstance, exports *goja.Object) {
	if instance.core.HasLstat() {
		setFunction(instance.rt, exports, "lstatSync", instance.lstatSync)
		setFunction(instance.rt, exports, "lstat", instance.lstat)
	}
	if instance.core.HasRealpath() {
		setFunction(instance.rt, exports, "realPathSync", instance.realpathSync)
		setFunction(instance.rt, exports, "realPath", instance.realpath)
	}
	if instance.core.HasReadlink() {
		setFunction(instance.rt, exports, "readLinkSync", instance.readlinkSync)
		setFunction(instance.rt, exports, "readLink", instance.readlink)
	}
	if instance.core.HasSymlink() {
		setFunction(instance.rt, exports, "symlinkSync", instance.symlinkSync)
		setFunction(instance.rt, exports, "symlink", instance.symlink)
	}
	if instance.core.HasLink() {
		setFunction(instance.rt, exports, "linkSync", instance.linkSync)
		setFunction(instance.rt, exports, "link", instance.link)
	}
}

func bindDenoPromiseExtraExports(instance *moduleInstance, exports *goja.Object) {
	if instance.core.HasLstat() {
		setFunction(instance.rt, exports, "lstat", instance.lstat)
	}
	if instance.core.HasRealpath() {
		setFunction(instance.rt, exports, "realPath", instance.realpath)
	}
	if instance.core.HasReadlink() {
		setFunction(instance.rt, exports, "readLink", instance.readlink)
	}
	if instance.core.HasSymlink() {
		setFunction(instance.rt, exports, "symlink", instance.symlink)
	}
	if instance.core.HasLink() {
		setFunction(instance.rt, exports, "link", instance.link)
	}
}

func setFunction(rt *goja.Runtime, object *goja.Object, name string, fn func(goja.FunctionCall) goja.Value) {
	if err := object.Set(name, fn); err != nil {
		panic(err)
	}
}
