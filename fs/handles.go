package fs

import (
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/dop251/goja"
)

type readResult struct {
	data []byte
	eof  bool
}

func newFsFileConstructor(rt *goja.Runtime) goja.Value {
	return rt.ToValue(func(goja.ConstructorCall) *goja.Object {
		panic(rt.NewTypeError("'FsFile' cannot be constructed directly; use open() or openSync()"))
	})
}

func bindFsFile(instance *moduleInstance, handle *FileHandle) *goja.Object {
	rt := instance.rt
	object := rt.NewObject()
	_ = object.Set("writeSync", func(call goja.FunctionCall) goja.Value {
		data, err := bytesFromValue(rt, call.Argument(0))
		if err != nil {
			panicJSError(rt, err)
		}
		n, err := handle.Write(data)
		if err != nil {
			panicJSError(rt, err)
		}
		return rt.ToValue(n)
	})
	_ = object.Set("write", func(call goja.FunctionCall) goja.Value {
		data, err := bytesFromValue(rt, call.Argument(0))
		if err != nil {
			panicJSError(rt, err)
		}
		return instance.promiseCall(func() (any, error) {
			return handle.Write(data)
		}, func(rt *goja.Runtime, value any) goja.Value {
			return rt.ToValue(value.(int))
		})
	})
	_ = object.Set("readSync", func(call goja.FunctionCall) goja.Value {
		target := call.Argument(0).ToObject(rt)
		size := typedArrayByteLength(target)
		data := make([]byte, size)
		n, err := handle.Read(data)
		if err != nil && !errors.Is(err, io.EOF) {
			panicJSError(rt, err)
		}
		if err := writeIntoTypedArray(target, data[:n]); err != nil {
			panicJSError(rt, err)
		}
		if n == 0 && errors.Is(err, io.EOF) {
			return goja.Null()
		}
		return rt.ToValue(n)
	})
	_ = object.Set("read", func(call goja.FunctionCall) goja.Value {
		target := call.Argument(0).ToObject(rt)
		size := typedArrayByteLength(target)
		return instance.promiseCall(func() (any, error) {
			data := make([]byte, size)
			n, err := handle.Read(data)
			if err != nil && !errors.Is(err, io.EOF) {
				return nil, err
			}
			return readResult{data: append([]byte(nil), data[:n]...), eof: n == 0 && errors.Is(err, io.EOF)}, nil
		}, func(rt *goja.Runtime, value any) goja.Value {
			result := value.(readResult)
			if err := writeIntoTypedArray(target, result.data); err != nil {
				panicJSError(rt, err)
			}
			if result.eof {
				return goja.Null()
			}
			return rt.ToValue(len(result.data))
		})
	})
	_ = object.Set("seekSync", func(call goja.FunctionCall) goja.Value {
		offset := requiredInt(rt, call.Argument(0), "offset")
		whence := parseWhence(rt, call.Argument(1))
		position, err := handle.Seek(offset, whence)
		if err != nil {
			panicJSError(rt, err)
		}
		return rt.ToValue(position)
	})
	_ = object.Set("seek", func(call goja.FunctionCall) goja.Value {
		offset := requiredInt(rt, call.Argument(0), "offset")
		whence := parseWhence(rt, call.Argument(1))
		return instance.promiseCall(func() (any, error) {
			return handle.Seek(offset, whence)
		}, func(rt *goja.Runtime, value any) goja.Value {
			return rt.ToValue(value.(int64))
		})
	})
	_ = object.Set("truncateSync", func(call goja.FunctionCall) goja.Value {
		if err := handle.Truncate(nonNegativeLength(rt, call.Argument(0))); err != nil {
			panicJSError(rt, err)
		}
		return goja.Undefined()
	})
	_ = object.Set("truncate", func(call goja.FunctionCall) goja.Value {
		length := nonNegativeLength(rt, call.Argument(0))
		return instance.promiseCall(func() (any, error) {
			return nil, handle.Truncate(length)
		}, nil)
	})
	_ = object.Set("statSync", func(goja.FunctionCall) goja.Value {
		info, err := handle.Stat()
		if err != nil {
			panicJSError(rt, err)
		}
		return fileInfoValue(rt, info)
	})
	_ = object.Set("stat", func(goja.FunctionCall) goja.Value {
		return instance.promiseCall(func() (any, error) {
			return handle.Stat()
		}, func(rt *goja.Runtime, value any) goja.Value {
			return fileInfoValue(rt, value.(os.FileInfo))
		})
	})
	_ = object.Set("syncSync", func(goja.FunctionCall) goja.Value {
		if err := handle.Sync(); err != nil {
			panicJSError(rt, err)
		}
		return goja.Undefined()
	})
	_ = object.Set("syncDataSync", object.Get("syncSync"))
	_ = object.Set("sync", func(goja.FunctionCall) goja.Value {
		return instance.promiseCall(func() (any, error) {
			return nil, handle.Sync()
		}, nil)
	})
	_ = object.Set("syncData", object.Get("sync"))
	_ = object.Set("utimeSync", func(call goja.FunctionCall) goja.Value {
		atime := timeValue(rt, call.Argument(0), "atime")
		mtime := timeValue(rt, call.Argument(1), "mtime")
		if err := handle.Chtimes(atime, mtime); err != nil {
			panicJSError(rt, err)
		}
		return goja.Undefined()
	})
	_ = object.Set("utime", func(call goja.FunctionCall) goja.Value {
		atime := timeValue(rt, call.Argument(0), "atime")
		mtime := timeValue(rt, call.Argument(1), "mtime")
		return instance.promiseCall(func() (any, error) {
			return nil, handle.Chtimes(atime, mtime)
		}, nil)
	})
	_ = object.Set("close", func(goja.FunctionCall) goja.Value {
		if err := handle.Close(); err != nil {
			panicJSError(rt, err)
		}
		return goja.Undefined()
	})
	_ = object.Set("isTerminal", func(goja.FunctionCall) goja.Value {
		return rt.ToValue(false)
	})

	var readable goja.Value
	var writable goja.Value
	_ = object.DefineAccessorProperty(
		"readable",
		rt.ToValue(func() goja.Value {
			if !instance.streams {
				panicJSError(rt, errors.New("WHATWG Streams integration is disabled"))
			}
			if readable == nil {
				readable = newFileReadableStream(instance, handle)
			}
			return readable
		}),
		goja.Undefined(),
		goja.FLAG_TRUE,
		goja.FLAG_TRUE,
	)
	_ = object.DefineAccessorProperty(
		"writable",
		rt.ToValue(func() goja.Value {
			if !instance.streams {
				panicJSError(rt, errors.New("WHATWG Streams integration is disabled"))
			}
			if writable == nil {
				writable = newFileWritableStream(instance, handle)
			}
			return writable
		}),
		goja.Undefined(),
		goja.FLAG_TRUE,
		goja.FLAG_TRUE,
	)
	return object
}

func parseWhence(rt *goja.Runtime, value goja.Value) int {
	if goja.IsString(value) {
		switch strings.ToLower(value.String()) {
		case "start":
			return io.SeekStart
		case "current":
			return io.SeekCurrent
		case "end":
			return io.SeekEnd
		}
		panic(rt.NewTypeError("invalid whence"))
	}
	return int(value.ToInteger())
}

func typedArrayByteLength(object *goja.Object) int {
	value := object.Get("byteLength")
	if value == nil || goja.IsUndefined(value) {
		return 0
	}
	return int(value.ToInteger())
}

func writeIntoTypedArray(target *goja.Object, data []byte) error {
	for index, value := range data {
		if err := target.Set(strconv.Itoa(index), value); err != nil {
			return err
		}
	}
	return nil
}
