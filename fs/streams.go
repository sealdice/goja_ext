package fs

import (
	"errors"
	"io"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/streams"
)

func newFileReadableStream(instance *moduleInstance, handle *FileHandle) goja.Value {
	rt := instance.rt
	stream, err := streams.NewReadableStream(rt, streams.ReadableStreamSource{
		Pull: func(controller *goja.Object) goja.Value {
			data := make([]byte, instance.core.ChunkSize())
			n, readErr := handle.Read(data)
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				callController(rt, controller, "error", jsError(rt, readErr))
				return goja.Undefined()
			}
			if n == 0 && errors.Is(readErr, io.EOF) {
				callController(rt, controller, "close")
				return goja.Undefined()
			}
			callController(rt, controller, "enqueue", bytesValue(rt, data[:n]))
			return goja.Undefined()
		},
		Cancel: func(reason goja.Value) goja.Value {
			_ = reason
			return goja.Undefined()
		},
	})
	if err != nil {
		panicJSError(rt, err)
	}
	return stream
}

func newFileWritableStream(instance *moduleInstance, handle *FileHandle) goja.Value {
	rt := instance.rt
	stream, err := streams.NewWritableStream(rt, streams.WritableStreamSink{
		Write: func(chunk goja.Value, _ *goja.Object) goja.Value {
			data, err := bytesFromValue(rt, chunk)
			if err != nil {
				panicJSError(rt, err)
			}
			if err := handle.WriteAll(data); err != nil {
				panicJSError(rt, err)
			}
			return goja.Undefined()
		},
		Close: func() goja.Value {
			if err := handle.Sync(); err != nil {
				panicJSError(rt, err)
			}
			return goja.Undefined()
		},
		Abort: func(reason goja.Value) goja.Value {
			_ = reason
			return goja.Undefined()
		},
	})
	if err != nil {
		panicJSError(rt, err)
	}
	return stream
}

func (m *moduleInstance) writeReadableStream(
	name string,
	input goja.Value,
	options writeFileOptions,
	text bool,
) goja.Value {
	handle, err := m.openWriteHandle(name, options)
	if err != nil {
		return rejectedPromise(m.rt, err)
	}

	streamValue := input
	if text {
		encoder, newErr := m.rt.New(streams.Exports(m.rt).Get("TextEncoderStream"))
		if newErr != nil {
			_ = handle.Close()
			return rejectedPromise(m.rt, newErr)
		}
		piped, pipeErr := callObjectMethod(
			input.ToObject(m.rt),
			"pipeThrough",
			encoder,
		)
		if pipeErr != nil {
			_ = handle.Close()
			return rejectedPromise(m.rt, pipeErr)
		}
		streamValue = piped
	}

	consumed, consumeErr := streams.ConsumeReadableStream(m.rt, streamValue, func(chunk goja.Value) goja.Value {
		data, err := bytesFromValue(m.rt, chunk)
		if err != nil {
			panicJSError(m.rt, err)
		}
		if err := handle.WriteAll(data); err != nil {
			panicJSError(m.rt, err)
		}
		return goja.Undefined()
	})
	if consumeErr != nil {
		_ = handle.Close()
		return rejectedPromise(m.rt, consumeErr)
	}

	result, resolve, reject := m.rt.NewPromise()
	thenPromise(
		m.rt,
		m.rt.ToValue(consumed),
		func(goja.FunctionCall) goja.Value {
			closeErr := handle.Close()
			if closeErr != nil {
				_ = reject(jsError(m.rt, closeErr))
			} else {
				_ = resolve(goja.Undefined())
			}
			return goja.Undefined()
		},
		func(call goja.FunctionCall) goja.Value {
			_ = handle.Close()
			_ = reject(call.Argument(0))
			return goja.Undefined()
		},
	)
	return m.rt.ToValue(result)
}

func (m *moduleInstance) openWriteHandle(name string, options writeFileOptions) (*FileHandle, error) {
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
	return m.core.OpenFile(name, flags, options.mode)
}

func callController(rt *goja.Runtime, controller *goja.Object, name string, args ...goja.Value) {
	fn, ok := goja.AssertFunction(controller.Get(name))
	if !ok {
		panic(rt.NewTypeError("ReadableStream controller method %s is not callable", name))
	}
	if _, err := fn(controller, args...); err != nil {
		panic(err)
	}
}

func callObjectMethod(object *goja.Object, name string, args ...goja.Value) (goja.Value, error) {
	fn, ok := goja.AssertFunction(object.Get(name))
	if !ok {
		return nil, errors.New(name + " is not callable")
	}
	return fn(object, args...)
}

func thenPromise(
	rt *goja.Runtime,
	value goja.Value,
	onFulfilled func(goja.FunctionCall) goja.Value,
	onRejected func(goja.FunctionCall) goja.Value,
) {
	then, ok := goja.AssertFunction(value.ToObject(rt).Get("then"))
	if !ok {
		panic(rt.NewTypeError("expected a Promise"))
	}
	if _, err := then(
		value.ToObject(rt),
		rt.ToValue(onFulfilled),
		rt.ToValue(onRejected),
	); err != nil {
		panic(err)
	}
}

func rejectedPromise(rt *goja.Runtime, err error) goja.Value {
	promise, _, reject := rt.NewPromise()
	_ = reject(jsError(rt, err))
	return rt.ToValue(promise)
}
