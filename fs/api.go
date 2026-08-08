package fs

import (
	"errors"
	"io"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/buffer"
	"github.com/sealdice/goja_ext/streams"
	"github.com/spf13/afero"
)

type writeFileOptions struct {
	appendMode bool
	create     bool
	createNew  bool
	truncate   bool
	mode       os.FileMode
}

type tempPathOptions struct {
	dir    string
	prefix string
	suffix string
}

func (m *moduleInstance) cwd(goja.FunctionCall) goja.Value {
	return m.rt.ToValue(m.core.Cwd())
}

func (m *moduleInstance) chdir(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "directory")
	if err := m.core.Chdir(name); err != nil {
		panicJSError(m.rt, err)
	}
	return goja.Undefined()
}

func (m *moduleInstance) mkdirSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	options := objectOrEmpty(m.rt, call.Argument(1))
	recursive := propertyBool(options, "recursive", false)
	mode := propertyMode(options, "mode", 0o777)
	if err := m.core.Mkdir(name, mode, recursive); err != nil {
		panicJSError(m.rt, err)
	}
	return goja.Undefined()
}

func (m *moduleInstance) mkdir(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	options := objectOrEmpty(m.rt, call.Argument(1))
	recursive := propertyBool(options, "recursive", false)
	mode := propertyMode(options, "mode", 0o777)
	return m.promiseCall(
		func() (any, error) { return nil, m.core.Mkdir(name, mode, recursive) },
		nil,
	)
}

func (m *moduleInstance) removeSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	options := objectOrEmpty(m.rt, call.Argument(1))
	if err := m.core.Remove(name, propertyBool(options, "recursive", false)); err != nil {
		panicJSError(m.rt, err)
	}
	return goja.Undefined()
}

func (m *moduleInstance) remove(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	options := objectOrEmpty(m.rt, call.Argument(1))
	recursive := propertyBool(options, "recursive", false)
	return m.promiseCall(
		func() (any, error) { return nil, m.core.Remove(name, recursive) },
		nil,
	)
}

func (m *moduleInstance) renameSync(call goja.FunctionCall) goja.Value {
	oldname := requiredPath(m.rt, call.Argument(0), "oldpath")
	newname := requiredPath(m.rt, call.Argument(1), "newpath")
	if err := m.core.Rename(oldname, newname); err != nil {
		panicJSError(m.rt, err)
	}
	return goja.Undefined()
}

func (m *moduleInstance) rename(call goja.FunctionCall) goja.Value {
	oldname := requiredPath(m.rt, call.Argument(0), "oldpath")
	newname := requiredPath(m.rt, call.Argument(1), "newpath")
	return m.promiseCall(
		func() (any, error) { return nil, m.core.Rename(oldname, newname) },
		nil,
	)
}

func (m *moduleInstance) copyFileSync(call goja.FunctionCall) goja.Value {
	from := m.core.ResolvePath(requiredPath(m.rt, call.Argument(0), "fromPath"))
	to := m.core.ResolvePath(requiredPath(m.rt, call.Argument(1), "toPath"))
	if err := copyFile(m.core.backend, from, to); err != nil {
		panicJSError(m.rt, wrapPathError("copyFile", from, to, err))
	}
	return goja.Undefined()
}

func (m *moduleInstance) copyFile(call goja.FunctionCall) goja.Value {
	from := requiredPath(m.rt, call.Argument(0), "fromPath")
	to := requiredPath(m.rt, call.Argument(1), "toPath")
	return m.promiseCall(func() (any, error) {
		err := copyFile(m.core.backend, m.core.ResolvePath(from), m.core.ResolvePath(to))
		return nil, wrapPathError("copyFile", m.core.ResolvePath(from), m.core.ResolvePath(to), err)
	}, nil)
}

func (m *moduleInstance) chmodSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	mode := requiredInt(m.rt, call.Argument(1), "mode")
	if err := m.core.Chmod(name, os.FileMode(mode)); err != nil {
		panicJSError(m.rt, err)
	}
	return goja.Undefined()
}

func (m *moduleInstance) chmod(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	mode := requiredInt(m.rt, call.Argument(1), "mode")
	return m.promiseCall(
		func() (any, error) { return nil, m.core.Chmod(name, os.FileMode(mode)) },
		nil,
	)
}

func (m *moduleInstance) chownSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	uid := requiredInt(m.rt, call.Argument(1), "uid")
	gid := requiredInt(m.rt, call.Argument(2), "gid")
	if err := m.core.Chown(name, int(uid), int(gid)); err != nil {
		panicJSError(m.rt, err)
	}
	return goja.Undefined()
}

func (m *moduleInstance) chown(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	uid := requiredInt(m.rt, call.Argument(1), "uid")
	gid := requiredInt(m.rt, call.Argument(2), "gid")
	return m.promiseCall(
		func() (any, error) { return nil, m.core.Chown(name, int(uid), int(gid)) },
		nil,
	)
}

func (m *moduleInstance) utimeSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	atime := timeValue(m.rt, call.Argument(1), "atime")
	mtime := timeValue(m.rt, call.Argument(2), "mtime")
	if err := m.core.Chtimes(name, atime, mtime); err != nil {
		panicJSError(m.rt, err)
	}
	return goja.Undefined()
}

func (m *moduleInstance) utime(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	atime := timeValue(m.rt, call.Argument(1), "atime")
	mtime := timeValue(m.rt, call.Argument(2), "mtime")
	return m.promiseCall(
		func() (any, error) { return nil, m.core.Chtimes(name, atime, mtime) },
		nil,
	)
}

func (m *moduleInstance) statSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	info, err := m.core.Stat(name)
	if err != nil {
		panicJSError(m.rt, err)
	}
	return fileInfoValue(m.rt, info)
}

func (m *moduleInstance) stat(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	return m.promiseCall(func() (any, error) {
		return m.core.Stat(name)
	}, func(rt *goja.Runtime, value any) goja.Value {
		return fileInfoValue(rt, value.(os.FileInfo))
	})
}

func (m *moduleInstance) truncateSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	length := nonNegativeLength(m.rt, call.Argument(1))
	file, err := m.core.OpenFile(name, openWrite, 0)
	if err == nil {
		err = file.Truncate(length)
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
	}
	if err != nil {
		panicJSError(m.rt, wrapPathError("truncate", m.core.ResolvePath(name), "", err))
	}
	return goja.Undefined()
}

func (m *moduleInstance) truncate(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	length := nonNegativeLength(m.rt, call.Argument(1))
	return m.promiseCall(func() (any, error) {
		file, err := m.core.OpenFile(name, openWrite, 0)
		if err != nil {
			return nil, err
		}
		err = file.Truncate(length)
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
		return nil, wrapPathError("truncate", m.core.ResolvePath(name), "", err)
	}, nil)
}

func (m *moduleInstance) readFileSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	enc := encodingFromOptions(m.rt, call.Argument(1))
	data, err := m.readFileBytes(name)
	if err != nil {
		panicJSError(m.rt, err)
	}
	if enc != "" && enc != "buffer" {
		return buffer.EncodeBytes(m.rt, data, m.rt.ToValue(enc))
	}
	return bytesValue(m.rt, data)
}

func (m *moduleInstance) readFile(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	enc := encodingFromOptions(m.rt, call.Argument(1))
	return m.nodeCall(call, func() (any, error) {
		return m.readFileBytes(name)
	}, func(rt *goja.Runtime, value any) goja.Value {
		data := value.([]byte)
		if enc != "" && enc != "buffer" {
			return buffer.EncodeBytes(rt, data, rt.ToValue(enc))
		}
		return bytesValue(rt, data)
	})
}

func (m *moduleInstance) readTextFileSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	data, err := m.readFileBytes(name)
	if err != nil {
		panicJSError(m.rt, err)
	}
	return m.rt.ToValue(string(data))
}

func (m *moduleInstance) readTextFile(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	return m.promiseCall(func() (any, error) {
		data, err := m.readFileBytes(name)
		return string(data), err
	}, func(rt *goja.Runtime, value any) goja.Value {
		return rt.ToValue(value.(string))
	})
}

func (m *moduleInstance) writeFileSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	if streams.IsReadableStream(m.rt, call.Argument(1)) {
		panicJSError(m.rt, errors.New("stream input is not supported by writeFileSync"))
	}
	data, err := bytesFromValue(m.rt, call.Argument(1))
	if err != nil {
		panicJSError(m.rt, err)
	}
	options := writeFileOptionsFromObject(objectOrEmpty(m.rt, call.Argument(2)))
	if err := m.writeFileBytes(name, data, options); err != nil {
		panicJSError(m.rt, err)
	}
	return goja.Undefined()
}

func (m *moduleInstance) writeFile(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	dataValue := call.Argument(1)
	options := writeFileOptionsFromObject(objectOrEmpty(m.rt, call.Argument(2)))
	if streams.IsReadableStream(m.rt, dataValue) {
		if !m.streams {
			panicJSError(m.rt, errors.New("WHATWG Streams integration is disabled"))
		}
		return m.writeReadableStream(name, dataValue, options, false)
	}
	data, err := bytesFromValue(m.rt, dataValue)
	if err != nil {
		panicJSError(m.rt, err)
	}
	return m.promiseCall(func() (any, error) {
		return nil, m.writeFileBytes(name, data, options)
	}, nil)
}

func (m *moduleInstance) writeTextFileSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	if streams.IsReadableStream(m.rt, call.Argument(1)) {
		panicJSError(m.rt, errors.New("stream input is not supported by writeTextFileSync"))
	}
	options := writeFileOptionsFromObject(objectOrEmpty(m.rt, call.Argument(2)))
	if err := m.writeFileBytes(name, []byte(call.Argument(1).String()), options); err != nil {
		panicJSError(m.rt, err)
	}
	return goja.Undefined()
}

func (m *moduleInstance) writeTextFile(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	dataValue := call.Argument(1)
	options := writeFileOptionsFromObject(objectOrEmpty(m.rt, call.Argument(2)))
	if streams.IsReadableStream(m.rt, dataValue) {
		if !m.streams {
			panicJSError(m.rt, errors.New("WHATWG Streams integration is disabled"))
		}
		return m.writeReadableStream(name, dataValue, options, true)
	}
	data := []byte(dataValue.String())
	return m.promiseCall(func() (any, error) {
		return nil, m.writeFileBytes(name, data, options)
	}, nil)
}

func (m *moduleInstance) openSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	flags := parseOpenFlags(m.rt, call.Argument(1))
	mode := openMode(m.rt, call.Argument(1))
	if goja.IsString(call.Argument(1)) {
		if arg := call.Argument(2); arg != nil && !goja.IsUndefined(arg) {
			mode = os.FileMode(arg.ToInteger())
		}
	}
	handle, err := m.core.OpenFile(name, flags, mode)
	if err != nil {
		panicJSError(m.rt, err)
	}
	return bindFsFile(m, handle)
}

func (m *moduleInstance) open(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	flags := parseOpenFlags(m.rt, call.Argument(1))
	mode := openMode(m.rt, call.Argument(1))
	if goja.IsString(call.Argument(1)) {
		if arg := call.Argument(2); arg != nil && !goja.IsUndefined(arg) {
			mode = os.FileMode(arg.ToInteger())
		}
	}
	return m.nodeCall(call, func() (any, error) {
		return m.core.OpenFile(name, flags, mode)
	}, func(rt *goja.Runtime, value any) goja.Value {
		return bindFsFile(m.withRuntime(rt), value.(*FileHandle))
	})
}

func (m *moduleInstance) createSync(call goja.FunctionCall) goja.Value {
	return m.openSync(goja.FunctionCall{
		This: call.This,
		Arguments: []goja.Value{call.Argument(0), m.rt.ToValue(map[string]any{
			"read": true, "write": true, "truncate": true, "create": true,
		})},
	})
}

func (m *moduleInstance) create(call goja.FunctionCall) goja.Value {
	return m.open(goja.FunctionCall{
		This: call.This,
		Arguments: []goja.Value{call.Argument(0), m.rt.ToValue(map[string]any{
			"read": true, "write": true, "truncate": true, "create": true,
		})},
	})
}

func (m *moduleInstance) makeTempFileSync(call goja.FunctionCall) goja.Value {
	options := m.tempPathOptionsFromObject(objectOrEmpty(m.rt, call.Argument(0)))
	file, err := m.makeTempFileHandle(options)
	if err != nil {
		panicJSError(m.rt, err)
	}
	name := file.file.Name()
	_ = file.Close()
	return m.rt.ToValue(name)
}

func (m *moduleInstance) makeTempFile(call goja.FunctionCall) goja.Value {
	options := m.tempPathOptionsFromObject(objectOrEmpty(m.rt, call.Argument(0)))
	return m.promiseCall(func() (any, error) {
		file, err := m.makeTempFileHandle(options)
		if err != nil {
			return nil, err
		}
		name := file.file.Name()
		err = file.Close()
		return name, err
	}, func(rt *goja.Runtime, value any) goja.Value {
		return rt.ToValue(value.(string))
	})
}

func (m *moduleInstance) makeTempDirSync(call goja.FunctionCall) goja.Value {
	options := m.tempPathOptionsFromObject(objectOrEmpty(m.rt, call.Argument(0)))
	name, err := m.makeTempDirName(options)
	if err != nil {
		panicJSError(m.rt, err)
	}
	return m.rt.ToValue(name)
}

func (m *moduleInstance) makeTempDir(call goja.FunctionCall) goja.Value {
	options := m.tempPathOptionsFromObject(objectOrEmpty(m.rt, call.Argument(0)))
	return m.promiseCall(func() (any, error) {
		return m.makeTempDirName(options)
	}, func(rt *goja.Runtime, value any) goja.Value {
		return rt.ToValue(value.(string))
	})
}

func (m *moduleInstance) readDirSync(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	entries, err := m.readDirEntries(name)
	if err != nil {
		panicJSError(m.rt, err)
	}
	array := m.rt.NewArray()
	for i, info := range entries {
		_ = array.Set(strconv.Itoa(i), dirEntryValue(m.rt, info))
	}
	return array
}

func (m *moduleInstance) readDir(call goja.FunctionCall) goja.Value {
	name := requiredPath(m.rt, call.Argument(0), "path")
	entries, err := m.readDirEntries(name)
	if err != nil {
		panicJSError(m.rt, err)
	}
	return asyncDirIterator(m.rt, entries)
}

func (m *moduleInstance) readFileBytes(name string) ([]byte, error) {
	file, err := m.core.backend.Open(m.core.ResolvePath(name))
	if err != nil {
		return nil, wrapPathError("readFile", m.core.ResolvePath(name), "", err)
	}
	data, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr == nil {
		readErr = closeErr
	}
	return data, wrapPathError("readFile", m.core.ResolvePath(name), "", readErr)
}

func (m *moduleInstance) writeFileBytes(name string, data []byte, options writeFileOptions) error {
	flags := openWrite
	if options.appendMode {
		flags |= openAppend
	}
	if options.truncate {
		flags |= openTruncate
	}
	if options.create {
		flags |= openCreate
	}
	if options.createNew {
		flags |= openCreateNew
	}
	file, err := m.core.OpenFile(name, flags, options.mode)
	if err != nil {
		return err
	}
	writeErr := file.WriteAll(data)
	closeErr := file.Close()
	if writeErr == nil {
		writeErr = closeErr
	}
	return wrapPathError("writeFile", m.core.ResolvePath(name), "", writeErr)
}

func writeFileOptionsFromObject(options *goja.Object) writeFileOptions {
	appendMode := propertyBool(options, "append", false)
	return writeFileOptions{
		appendMode: appendMode,
		create:     propertyBool(options, "create", true),
		createNew:  propertyBool(options, "createNew", false),
		truncate:   propertyBoolDefault(options, "truncate", !appendMode),
		mode:       propertyMode(options, "mode", 0o666),
	}
}

func (m *moduleInstance) tempPathOptionsFromObject(options *goja.Object) tempPathOptions {
	return tempPathOptions{
		dir:    propertyString(options, "dir", m.core.Cwd()),
		prefix: propertyString(options, "prefix", "tmp"),
		suffix: propertyString(options, "suffix", ""),
	}
}

func (m *moduleInstance) makeTempFileHandle(options tempPathOptions) (*FileHandle, error) {
	file, err := afero.TempFile(m.core.backend, m.core.ResolvePath(options.dir), options.prefix+"*"+options.suffix)
	if err != nil {
		return nil, wrapPathError("makeTempFile", m.core.ResolvePath(options.dir), "", err)
	}
	return &FileHandle{core: m.core, file: file}, nil
}

func (m *moduleInstance) makeTempDirName(options tempPathOptions) (string, error) {
	resolvedDir := m.core.ResolvePath(options.dir)
	if options.suffix == "" {
		name, err := afero.TempDir(m.core.backend, resolvedDir, options.prefix)
		return name, wrapPathError("makeTempDir", resolvedDir, "", err)
	}

	file, err := afero.TempFile(m.core.backend, resolvedDir, options.prefix+"*"+options.suffix)
	if err != nil {
		return "", wrapPathError("makeTempDir", resolvedDir, "", err)
	}
	name := file.Name()
	closeErr := file.Close()
	removeErr := m.core.backend.Remove(name)
	if closeErr != nil {
		if removeErr == nil {
			_ = m.core.backend.Remove(name)
		}
		return "", wrapPathError("makeTempDir", name, "", closeErr)
	}
	if removeErr != nil {
		return "", wrapPathError("makeTempDir", name, "", removeErr)
	}
	if err := m.core.backend.Mkdir(name, 0o700); err != nil {
		return "", wrapPathError("makeTempDir", name, "", err)
	}
	return name, nil
}

func (m *moduleInstance) readDirEntries(name string) ([]os.FileInfo, error) {
	file, err := m.core.backend.Open(m.core.ResolvePath(name))
	if err != nil {
		return nil, wrapPathError("readDir", m.core.ResolvePath(name), "", err)
	}
	entries, readErr := file.Readdir(-1)
	closeErr := file.Close()
	if readErr == nil {
		readErr = closeErr
	}
	return entries, wrapPathError("readDir", m.core.ResolvePath(name), "", readErr)
}

func (m *moduleInstance) withRuntime(rt *goja.Runtime) *moduleInstance {
	copy := *m
	copy.rt = rt
	return &copy
}

func (m *moduleInstance) promiseCall(
	op func() (any, error),
	convert func(*goja.Runtime, any) goja.Value,
) goja.Value {
	promise, resolve, reject := m.rt.NewPromise()
	run := func() {
		value, err := op()
		settle := func(rt *goja.Runtime) {
			if err != nil {
				_ = reject(jsError(rt, err))
				return
			}
			if convert == nil {
				_ = resolve(goja.Undefined())
				return
			}
			_ = resolve(convert(rt, value))
		}
		if m.scheduler != nil {
			_ = m.scheduler.RunOnLoop(settle)
		} else {
			settle(m.rt)
		}
	}
	if m.scheduler == nil {
		run()
	} else {
		go run()
	}
	return m.rt.ToValue(promise)
}

func copyFile(backend afero.Fs, from, to string) (err error) {
	source, err := backend.Open(from)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := source.Close(); err == nil {
			err = closeErr
		}
	}()
	target, err := backend.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr == nil {
		copyErr = closeErr
	}
	return copyErr
}

func jsError(rt *goja.Runtime, err error) *goja.Object {
	if err == nil {
		err = errors.New("fs error")
	}
	object := rt.NewGoError(err)
	_ = object.Set("code", errorCode(err))
	op, path, dest := errorDetails(err)
	if op != "" {
		_ = object.Set("syscall", op)
	}
	if path != "" {
		_ = object.Set("path", path)
	}
	if dest != "" {
		_ = object.Set("dest", dest)
	}
	return object
}

func jsErrorValue(rt *goja.Runtime, err error) goja.Value {
	var exception *goja.Exception
	if errors.As(err, &exception) {
		return exception.Value()
	}
	return jsError(rt, err)
}

func panicJSError(rt *goja.Runtime, err error) {
	panic(jsErrorValue(rt, err))
}

func requiredPath(rt *goja.Runtime, value goja.Value, name string) string {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		panic(rt.NewTypeError(`The "%s" argument is required.`, name))
	}
	return inputPath(value.String())
}

func requiredInt(rt *goja.Runtime, value goja.Value, name string) int64 {
	if value == nil || goja.IsUndefined(value) || !goja.IsNumber(value) {
		panic(rt.NewTypeError(`The "%s" argument must be a number.`, name))
	}
	return value.ToInteger()
}

func timeValue(rt *goja.Runtime, value goja.Value, name string) time.Time {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		panic(rt.NewTypeError(`The "%s" argument must be a number or Date.`, name))
	}
	if exported, ok := value.Export().(time.Time); ok {
		return exported
	}
	if !goja.IsNumber(value) {
		panic(rt.NewTypeError(`The "%s" argument must be a number or Date.`, name))
	}
	seconds := value.ToFloat()
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		panic(rt.NewTypeError(`The "%s" argument must be a finite number or Date.`, name))
	}
	wholeSeconds := math.Floor(seconds)
	nanoseconds := math.Trunc((seconds - wholeSeconds) * float64(time.Second))
	return time.Unix(int64(wholeSeconds), int64(nanoseconds))
}

func nonNegativeLength(rt *goja.Runtime, value goja.Value) int64 {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return 0
	}
	length := value.ToInteger()
	if length < 0 {
		return 0
	}
	return length
}

func objectOrEmpty(rt *goja.Runtime, value goja.Value) *goja.Object {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return rt.NewObject()
	}
	return value.ToObject(rt)
}

func propertyBool(object *goja.Object, name string, defaultValue bool) bool {
	value := object.Get(name)
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return defaultValue
	}
	return value.ToBoolean()
}

func propertyBoolDefault(object *goja.Object, name string, defaultValue bool) bool {
	return propertyBool(object, name, defaultValue)
}

func propertyString(object *goja.Object, name, defaultValue string) string {
	value := object.Get(name)
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return defaultValue
	}
	return value.String()
}

func propertyMode(object *goja.Object, name string, defaultValue os.FileMode) os.FileMode {
	value := object.Get(name)
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return defaultValue
	}
	return os.FileMode(value.ToInteger())
}

func bytesFromValue(rt *goja.Runtime, value goja.Value) ([]byte, error) {
	if goja.IsString(value) {
		return []byte(value.String()), nil
	}
	var data []byte
	var panicValue *goja.Exception
	if exception := rt.Try(func() {
		data = buffer.DecodeBytes(rt, value, goja.Undefined())
	}); exception != nil {
		panicValue = exception
	}
	if panicValue != nil {
		return nil, panicValue
	}
	return append([]byte(nil), data...), nil
}

func bytesValue(rt *goja.Runtime, data []byte) goja.Value {
	arrayBuffer := rt.NewArrayBuffer(append([]byte(nil), data...))
	typed, err := rt.New(rt.Get("Uint8Array"), rt.ToValue(arrayBuffer))
	if err != nil {
		panic(err)
	}
	return typed
}

func fileInfoValue(rt *goja.Runtime, info os.FileInfo) *goja.Object {
	return nodeStatsValue(rt, info)
}

func dateValue(rt *goja.Runtime, value time.Time) goja.Value {
	date, err := rt.New(rt.Get("Date"), rt.ToValue(value.UnixNano()/int64(time.Millisecond)))
	if err != nil {
		panic(err)
	}
	return date
}

func dirEntryValue(rt *goja.Runtime, info os.FileInfo) *goja.Object {
	object := rt.NewObject()
	_ = object.Set("name", info.Name())
	_ = object.Set("isFile", func(goja.FunctionCall) goja.Value { return rt.ToValue(info.Mode().IsRegular()) })
	_ = object.Set("isDirectory", func(goja.FunctionCall) goja.Value { return rt.ToValue(info.IsDir()) })
	_ = object.Set("isSymlink", func(goja.FunctionCall) goja.Value { return rt.ToValue(info.Mode()&os.ModeSymlink != 0) })
	return object
}

func asyncDirIterator(rt *goja.Runtime, entries []os.FileInfo) *goja.Object {
	index := 0
	iterator := rt.NewObject()
	_ = iterator.Set("next", func(goja.FunctionCall) goja.Value {
		promise, resolve, _ := rt.NewPromise()
		result := rt.NewObject()
		if index >= len(entries) {
			_ = result.Set("value", goja.Undefined())
			_ = result.Set("done", true)
		} else {
			_ = result.Set("value", dirEntryValue(rt, entries[index]))
			_ = result.Set("done", false)
			index++
		}
		_ = resolve(result)
		return rt.ToValue(promise)
	})
	if symbol, ok := rt.Get("Symbol").ToObject(rt).Get("asyncIterator").(*goja.Symbol); ok {
		_ = iterator.SetSymbol(symbol, func(goja.FunctionCall) goja.Value {
			return iterator
		})
	}
	return iterator
}

func parseOpenFlags(rt *goja.Runtime, value goja.Value) int {
	if goja.IsString(value) {
		return parseNodeFlags(rt, value.String())
	}
	options := objectOrEmpty(rt, value)
	read := propertyBool(options, "read", false)
	write := propertyBool(options, "write", false)
	appendMode := propertyBool(options, "append", false)
	truncate := propertyBool(options, "truncate", false)
	create := propertyBool(options, "create", false)
	createNew := propertyBool(options, "createNew", false)
	if !read && !write && !appendMode {
		panic(rt.NewTypeError(`"options" requires at least one option to be true`))
	}
	if truncate && !write {
		panic(rt.NewTypeError(`"truncate" option requires "write" to be true`))
	}
	if (create || createNew) && !write && !appendMode {
		panic(rt.NewTypeError(`"create" or "createNew" requires "write" or "append"`))
	}
	flags := 0
	if read {
		flags |= openRead
	}
	if write {
		flags |= openWrite
	}
	if appendMode {
		flags |= openAppend | openWrite
	}
	if truncate {
		flags |= openTruncate
	}
	if create {
		flags |= openCreate
	}
	if createNew {
		flags |= openCreateNew
	}
	return flags
}

func openMode(rt *goja.Runtime, value goja.Value) os.FileMode {
	return propertyMode(objectOrEmpty(rt, value), "mode", 0o666)
}
