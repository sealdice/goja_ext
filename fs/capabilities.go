package fs

import (
	"os"

	"github.com/dop251/goja"
)

func (m *moduleInstance) realpathSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	resolved, err := m.core.Realpath(name)
	if err != nil {
		panicJSError(m.rt, err)
	}
	return m.rt.ToValue(resolved)
}

func (m *moduleInstance) realpath(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	return m.promiseCall(func() (any, error) {
		return m.core.Realpath(name)
	}, func(rt *goja.Runtime, value any) goja.Value {
		return rt.ToValue(value.(string))
	})
}

func (m *moduleInstance) lstatSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	info, err := m.core.Lstat(name)
	if err != nil {
		panicJSError(m.rt, err)
	}
	return fileInfoValue(m.rt, info)
}

func (m *moduleInstance) lstat(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	return m.promiseCall(func() (any, error) {
		return m.core.Lstat(name)
	}, func(rt *goja.Runtime, value any) goja.Value {
		return fileInfoValue(rt, value.(os.FileInfo))
	})
}

func (m *moduleInstance) readlinkSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	target, err := m.core.Readlink(name)
	if err != nil {
		panicJSError(m.rt, err)
	}
	return m.rt.ToValue(target)
}

func (m *moduleInstance) readlink(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	return m.promiseCall(func() (any, error) {
		return m.core.Readlink(name)
	}, func(rt *goja.Runtime, value any) goja.Value {
		return rt.ToValue(value.(string))
	})
}

func (m *moduleInstance) symlinkSync(call goja.FunctionCall) goja.Value {
	target := requiredPath(m.rt, call.Argument(0), "target")
	name := requiredPath(m.rt, call.Argument(1), "path")
	if err := m.core.Symlink(target, name); err != nil {
		panicJSError(m.rt, err)
	}
	return goja.Undefined()
}

func (m *moduleInstance) symlink(call goja.FunctionCall) goja.Value {
	target := requiredPath(m.rt, call.Argument(0), "target")
	name := requiredPath(m.rt, call.Argument(1), "path")
	return m.promiseCall(func() (any, error) {
		return nil, m.core.Symlink(target, name)
	}, nil)
}

func (m *moduleInstance) linkSync(call goja.FunctionCall) goja.Value {
	existing := requiredPath(m.rt, call.Argument(0), "existingPath")
	name := requiredPath(m.rt, call.Argument(1), "newPath")
	if err := m.core.Link(existing, name); err != nil {
		panicJSError(m.rt, err)
	}
	return goja.Undefined()
}

func (m *moduleInstance) link(call goja.FunctionCall) goja.Value {
	existing := requiredPath(m.rt, call.Argument(0), "existingPath")
	name := requiredPath(m.rt, call.Argument(1), "newPath")
	return m.promiseCall(func() (any, error) {
		return nil, m.core.Link(existing, name)
	}, nil)
}
