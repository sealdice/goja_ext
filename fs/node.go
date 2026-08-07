package fs

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/dop251/goja"
	nodestreams "github.com/sealdice/goja_ext/streams/node"
)

// nodeCall dispatches an operation to a Node-style callback (when the last
// argument is a function) or to the existing Promise path.
func (m *moduleInstance) nodeCall(call goja.FunctionCall, op func() (any, error), convert func(*goja.Runtime, any) goja.Value) goja.Value {
	args := call.Arguments
	if len(args) > 0 {
		if cb, ok := goja.AssertFunction(args[len(args)-1]); ok {
			m.runCallbackOp(cb, op, convert)
			return goja.Undefined()
		}
	}
	return m.promiseCall(op, convert)
}

func (m *moduleInstance) runCallbackOp(cb goja.Callable, op func() (any, error), convert func(*goja.Runtime, any) goja.Value) {
	run := func() {
		value, err := op()
		settle := func(rt *goja.Runtime) {
			if err != nil {
				_, _ = cb(goja.Undefined(), jsError(rt, err))
				return
			}
			if convert == nil {
				_, _ = cb(goja.Undefined(), goja.Null())
				return
			}
			_, _ = cb(goja.Undefined(), goja.Null(), convert(rt, value))
		}
		if m.loop != nil {
			_ = m.loop.RunOnLoop(settle)
		} else {
			settle(m.rt)
		}
	}
	if m.loop == nil {
		run()
	} else {
		go run()
	}
}

// nodeCallbackWrapper wraps an existing Promise-returning method so it also
// supports the Node callback form when the last argument is a function.
func nodeCallbackWrapper(rt *goja.Runtime, original goja.Value) func(goja.FunctionCall) goja.Value {
	orig, ok := goja.AssertFunction(original)
	if !ok {
		return func(call goja.FunctionCall) goja.Value { return original }
	}
	return func(call goja.FunctionCall) goja.Value {
		args := call.Arguments
		if len(args) > 0 {
			if cb, ok := goja.AssertFunction(args[len(args)-1]); ok {
				promise, err := orig(call.This, args[:len(args)-1]...)
				if err != nil {
					panic(err)
				}
				if promise != nil {
					thenPromise(rt, promise,
						func(call goja.FunctionCall) goja.Value {
							_, _ = cb(goja.Undefined(), goja.Null(), call.Argument(0))
							return goja.Undefined()
						},
						func(call goja.FunctionCall) goja.Value {
							_, _ = cb(goja.Undefined(), call.Argument(0))
							return goja.Undefined()
						},
					)
				}
				return goja.Undefined()
			}
		}
		value, err := orig(call.This, args...)
		if err != nil {
			panic(err)
		}
		return value
	}
}

// encodingFromOptions reads the encoding option, which may be a plain string
// (e.g. "utf8") or { encoding: "utf8" }.
func encodingFromOptions(rt *goja.Runtime, value goja.Value) string {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return ""
	}
	if goja.IsString(value) {
		return value.String()
	}
	return propertyString(value.ToObject(rt), "encoding", "")
}

// --- Node additions ---------------------------------------------------------

func (m *moduleInstance) appendFileSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	data, err := bytesFromValue(m.rt, call.Argument(1))
	if err != nil {
		panicJSError(m.rt, err)
	}
	options := writeFileOptionsFromObject(objectOrEmpty(m.rt, call.Argument(2)))
	options.appendMode = true
	options.truncate = false
	if err := m.writeFileBytes(name, data, options); err != nil {
		panicJSError(m.rt, err)
	}
	return goja.Undefined()
}

func (m *moduleInstance) appendFile(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	dataValue := call.Argument(1)
	options := writeFileOptionsFromObject(objectOrEmpty(m.rt, call.Argument(2)))
	options.appendMode = true
	options.truncate = false
	return m.nodeCall(call, func() (any, error) {
		data, err := bytesFromValue(m.rt, dataValue)
		if err != nil {
			return nil, err
		}
		return nil, m.writeFileBytes(name, data, options)
	}, nil)
}

func (m *moduleInstance) existsSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	_, err := m.core.Stat(name)
	return m.rt.ToValue(err == nil)
}

func (m *moduleInstance) exists(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	return m.nodeCall(call, func() (any, error) {
		_, err := m.core.Stat(name)
		if err != nil {
			return false, nil
		}
		return true, nil
	}, func(rt *goja.Runtime, value any) goja.Value {
		return rt.ToValue(value.(bool))
	})
}

func (m *moduleInstance) accessSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	if _, err := m.core.Stat(name); err != nil {
		panicJSError(m.rt, err)
	}
	return goja.Undefined()
}

func (m *moduleInstance) access(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	return m.nodeCall(call, func() (any, error) {
		_, err := m.core.Stat(name)
		return nil, err
	}, nil)
}

func (m *moduleInstance) unlinkSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	if err := m.core.Remove(name, false); err != nil {
		panicJSError(m.rt, err)
	}
	return goja.Undefined()
}

func (m *moduleInstance) unlink(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	return m.nodeCall(call, func() (any, error) {
		return nil, m.core.Remove(name, false)
	}, nil)
}

func (m *moduleInstance) rmdirSync(call goja.FunctionCall) goja.Value {
	return m.unlinkSync(call)
}

func (m *moduleInstance) rmdir(call goja.FunctionCall) goja.Value {
	return m.unlink(call)
}

func (m *moduleInstance) rmSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	options := objectOrEmpty(m.rt, call.Argument(1))
	recursive := propertyBool(options, "recursive", false)
	force := propertyBool(options, "force", false)
	if err := m.core.Remove(name, recursive); err != nil && !(force && os.IsNotExist(err)) {
		panicJSError(m.rt, err)
	}
	return goja.Undefined()
}

func (m *moduleInstance) rm(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	options := objectOrEmpty(m.rt, call.Argument(1))
	recursive := propertyBool(options, "recursive", false)
	force := propertyBool(options, "force", false)
	return m.nodeCall(call, func() (any, error) {
		err := m.core.Remove(name, recursive)
		if force && os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}, nil)
}

func (m *moduleInstance) realpathSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	resolved := m.core.ResolvePath(name)
	info, err := m.core.backend.Stat(resolved)
	if err != nil {
		panicJSError(m.rt, wrapPathError("realpath", resolved, "", err))
	}
	_ = info
	return m.rt.ToValue(resolved)
}

func (m *moduleInstance) realpath(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	return m.nodeCall(call, func() (any, error) {
		resolved := m.core.ResolvePath(name)
		if _, err := m.core.backend.Stat(resolved); err != nil {
			return nil, wrapPathError("realpath", resolved, "", err)
		}
		return resolved, nil
	}, func(rt *goja.Runtime, value any) goja.Value {
		return rt.ToValue(value.(string))
	})
}

func (m *moduleInstance) lstatSync(call goja.FunctionCall) goja.Value {
	return m.statSync(call)
}

func (m *moduleInstance) lstat(call goja.FunctionCall) goja.Value {
	return m.stat(call)
}

// --- createReadStream / createWriteStream ----------------------------------

const nodeReadStreamSource = `(function (Readable, readFn, closeFn, opts) {
	let stream = null;
	let position = opts.start;
	stream = new Readable({
		highWaterMark: opts.highWaterMark || 65536,
		read(cb) {
			let want = opts.highWaterMark || 65536;
			if (opts.end !== null) {
				if (position > opts.end) { stream.push(null); return; }
				want = Math.min(want, opts.end - position + 1);
			}
			if (want <= 0) { stream.push(null); return; }
			const chunk = readFn(want);
			if (chunk === null) { stream.push(null); }
			else { position += chunk.length; stream.push(chunk); }
		},
	});
	if (opts.encoding) stream.setEncoding(opts.encoding);
	stream.on("close", closeFn);
	return stream;
})`

const nodeWriteStreamSource = `(function (Writable, writeFn, closeFn, opts) {
	const stream = new Writable({
		highWaterMark: opts.highWaterMark || 65536,
		write(chunk, encoding, cb) {
			try { writeFn(chunk); cb(null); } catch (e) { cb(e); }
		},
	});
	stream.on("finish", closeFn);
	stream.on("error", closeFn);
	return stream;
})`

func (m *moduleInstance) createReadStream(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	options := objectOrEmpty(m.rt, call.Argument(1))
	enc := propertyString(options, "encoding", "")
	start := propertyInt(options, "start", 0)
	hwm := propertyInt(options, "highWaterMark", 0)
	endValue := goja.Null()
	if end := propertyIntOrNull(options, "end"); end >= 0 {
		endValue = m.rt.ToValue(end)
	}

	handle, err := m.core.OpenFile(name, openRead, 0)
	if err != nil {
		panicJSError(m.rt, err)
	}
	if start > 0 {
		if _, err := handle.Seek(int64(start), io.SeekStart); err != nil {
			_ = handle.Close()
			panicJSError(m.rt, err)
		}
	}

	rt := m.rt
	readFn := rt.ToValue(func(call goja.FunctionCall) goja.Value {
		maxBytes := int(call.Argument(0).ToInteger())
		if maxBytes <= 0 {
			maxBytes = 64 * 1024
		}
		data := make([]byte, maxBytes)
		n, readErr := handle.Read(data)
		if readErr != nil && !isEOF(readErr) {
			panicJSError(rt, readErr)
		}
		if n == 0 {
			return goja.Null()
		}
		return bytesValue(rt, data[:n])
	})
	closeFn := rt.ToValue(func(goja.FunctionCall) goja.Value {
		_ = handle.Close()
		return goja.Undefined()
	})

	value, err := rt.RunString(nodeReadStreamSource)
	if err != nil {
		_ = handle.Close()
		panic(err)
	}
	fn, ok := goja.AssertFunction(value)
	if !ok {
		_ = handle.Close()
		panic(rt.NewTypeError("internal: read stream factory"))
	}
	result, err := fn(
		goja.Undefined(),
		nodestreams.Exports(rt).Get("Readable"),
		readFn,
		closeFn,
		rt.ToValue(map[string]interface{}{
			"start":         start,
			"end":           endValue,
			"encoding":      enc,
			"highWaterMark": hwm,
		}),
	)
	if err != nil {
		_ = handle.Close()
		panic(err)
	}
	return result
}

func (m *moduleInstance) createWriteStream(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	options := objectOrEmpty(m.rt, call.Argument(1))
	flags := propertyString(options, "flags", "w")

	var goFlags int
	switch flags {
	case "w":
		goFlags = openWrite | openTruncate | openCreate
	case "a", "a+":
		goFlags = openWrite | openAppend | openCreate
	case "wx":
		goFlags = openWrite | openCreateNew
	case "ax":
		goFlags = openWrite | openAppend | openCreateNew
	default:
		goFlags = openWrite | openTruncate | openCreate
	}
	handle, err := m.core.OpenFile(name, goFlags, 0o666)
	if err != nil {
		panicJSError(m.rt, err)
	}

	rt := m.rt
	writeFn := rt.ToValue(func(call goja.FunctionCall) goja.Value {
		data, err := bytesFromValue(rt, call.Argument(0))
		if err != nil {
			panicJSError(rt, err)
		}
		if err := handle.WriteAll(data); err != nil {
			panicJSError(rt, err)
		}
		return goja.Undefined()
	})
	closeFn := rt.ToValue(func(goja.FunctionCall) goja.Value {
		_ = handle.Close()
		return goja.Undefined()
	})

	value, err := rt.RunString(nodeWriteStreamSource)
	if err != nil {
		_ = handle.Close()
		panic(err)
	}
	fn, ok := goja.AssertFunction(value)
	if !ok {
		_ = handle.Close()
		panic(rt.NewTypeError("internal: write stream factory"))
	}
	result, err := fn(
		goja.Undefined(),
		nodestreams.Exports(rt).Get("Writable"),
		writeFn,
		closeFn,
		rt.ToValue(map[string]interface{}{}),
	)
	if err != nil {
		_ = handle.Close()
		panic(err)
	}
	return result
}

func isEOF(err error) bool {
	return err == io.EOF || errors.Is(err, io.EOF)
}

func propertyInt(object *goja.Object, name string, defaultValue int) int {
	value := object.Get(name)
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return defaultValue
	}
	return int(value.ToInteger())
}

func propertyIntOrNull(object *goja.Object, name string) int64 {
	value := object.Get(name)
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return -1
	}
	return value.ToInteger()
}

// parseNodeFlags converts a Node.js open flag string to the core flag set.
func parseNodeFlags(rt *goja.Runtime, flags string) int {
	switch strings.TrimSpace(flags) {
	case "r":
		return openRead
	case "r+":
		return openRead | openWrite
	case "w":
		return openWrite | openTruncate | openCreate
	case "w+":
		return openRead | openWrite | openTruncate | openCreate
	case "a":
		return openWrite | openAppend | openCreate
	case "a+":
		return openRead | openWrite | openAppend | openCreate
	case "wx", "w+x":
		return openWrite | openCreateNew
	case "ax", "a+x":
		return openWrite | openAppend | openCreateNew
	}
	panic(rt.NewTypeError("Unknown file open flags: " + flags))
}

// bindNodeExports adds the Node.js-style API surface on top of the Deno-style
// exports.
func bindNodeExports(instance *moduleInstance, exports *goja.Object) {
	rt := instance.rt

	// Callback support for existing Promise-returning methods.
	for _, name := range []string{
		"chmod", "chown", "copyFile", "create", "makeTempDir", "makeTempFile",
		"mkdir", "open", "readDir", "readFile", "readTextFile", "remove",
		"rename", "stat", "truncate", "utime", "writeFile", "writeTextFile",
	} {
		if value := exports.Get(name); value != nil && !goja.IsUndefined(value) {
			_ = exports.Set(name, nodeCallbackWrapper(rt, value))
		}
	}

	setNodeFn := func(name string, fn func(goja.FunctionCall) goja.Value) {
		if err := exports.Set(name, fn); err != nil {
			panic(err)
		}
	}
	setNodeFn("appendFileSync", instance.appendFileSync)
	setNodeFn("appendFile", instance.appendFile)
	setNodeFn("existsSync", instance.existsSync)
	setNodeFn("exists", instance.exists)
	setNodeFn("accessSync", instance.accessSync)
	setNodeFn("access", instance.access)
	setNodeFn("unlinkSync", instance.unlinkSync)
	setNodeFn("unlink", instance.unlink)
	setNodeFn("rmdirSync", instance.rmdirSync)
	setNodeFn("rmdir", instance.rmdir)
	setNodeFn("rmSync", instance.rmSync)
	setNodeFn("rm", instance.rm)
	setNodeFn("realpathSync", instance.realpathSync)
	setNodeFn("realpath", instance.realpath)
	setNodeFn("lstatSync", instance.lstatSync)
	setNodeFn("lstat", instance.lstat)
	setNodeFn("createReadStream", instance.createReadStream)
	setNodeFn("createWriteStream", instance.createWriteStream)
	setNodeFn("readStream", instance.createReadStream)
	setNodeFn("writeStream", instance.createWriteStream)
	_ = exports.Set("constants", nodeConstants(rt))
}
