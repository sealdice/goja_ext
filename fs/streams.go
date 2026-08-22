package fs

import (
	"errors"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/streams"
)

type fileHandleReadCloser struct {
	handle *FileHandle
}

func (r *fileHandleReadCloser) Read(p []byte) (int, error) { return r.handle.Read(p) }

// Close 等待在途读完成后物理关闭文件，只能在后台 goroutine 调用。
func (r *fileHandleReadCloser) Close() error { return r.handle.closeAndWait() }

type fileHandleWriteCloser struct {
	handle *FileHandle
}

func (w *fileHandleWriteCloser) Write(p []byte) (int, error) {
	if err := w.handle.WriteAll(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Close 等待在途写完成后物理关闭文件，只能在后台 goroutine 调用。
func (w *fileHandleWriteCloser) Close() error { return w.handle.closeAndWait() }

func newFileReadableStream(instance *moduleInstance, handle *FileHandle) goja.Value {
	rt := instance.rt
	stream, err := streams.NewReadableStreamFromReader(
		rt,
		instance.scheduler,
		&fileHandleReadCloser{handle: handle},
		streams.WithChunkSize(instance.core.ChunkSize()),
		streams.WithMapError(jsErrorValue),
		streams.WithOnCancel(func(rt *goja.Runtime, reason goja.Value) goja.Value {
			_ = reason
			return instance.promiseCall(func() (any, error) {
				return nil, handle.closeAndWait()
			}, nil)
		}),
	)
	if err != nil {
		panicJSError(rt, err)
	}
	return stream.Stream()
}

func newFileWritableStream(instance *moduleInstance, handle *FileHandle) goja.Value {
	rt := instance.rt
	stream, err := streams.NewWritableStreamToWriter(
		rt,
		instance.scheduler,
		&fileHandleWriteCloser{handle: handle},
		streams.WithDecodeChunk(bytesFromValue),
		streams.WithMapWriteError(jsErrorValue),
	)
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
	rt := m.rt
	result, resolve, reject := rt.NewPromise()
	settled := false
	var subscription *abortSubscription

	settleAbort := func(reason goja.Value) {
		if settled {
			return
		}
		settled = true
		subscription.cleanup(rt)
		_ = reject(valueOrUndefined(reason))
	}
	var validationError goja.Value
	subscription, validationError = subscribeAbortSignal(rt, options.signal, settleAbort)
	if validationError != nil {
		settled = true
		_ = reject(validationError)
		return rt.ToValue(result)
	}
	if settled {
		return rt.ToValue(result)
	}

	settleOpen := func(handle *FileHandle, openErr error) {
		if openErr != nil {
			if !settled {
				settled = true
				subscription.cleanup(rt)
				_ = reject(jsErrorValue(rt, openErr))
			}
			return
		}
		if settled {
			go func() { _ = handle.closeAndWait() }()
			return
		}

		streamValue := input
		if text {
			encoder, err := rt.New(streams.Exports(rt).Get("TextEncoderStream"))
			if err != nil {
				settled = true
				subscription.cleanup(rt)
				_ = handle.Close()
				_ = reject(jsErrorValue(rt, err))
				return
			}
			streamValue, err = callObjectMethod(input.ToObject(rt), "pipeThrough", encoder)
			if err != nil {
				settled = true
				subscription.cleanup(rt)
				_ = handle.Close()
				_ = reject(jsErrorValue(rt, err))
				return
			}
		}

		destination := newFileWritableStream(m, handle)
		pipeArgs := []goja.Value{destination}
		if options.signal != nil && !goja.IsUndefined(options.signal) && !goja.IsNull(options.signal) {
			pipeOptions := rt.NewObject()
			_ = pipeOptions.Set("signal", options.signal)
			pipeArgs = append(pipeArgs, pipeOptions)
		}
		piped, err := callObjectMethod(streamValue.ToObject(rt), "pipeTo", pipeArgs...)
		if err != nil {
			settled = true
			subscription.cleanup(rt)
			_ = handle.Close()
			_ = reject(jsErrorValue(rt, err))
			return
		}
		thenPromise(
			rt,
			piped,
			func(goja.FunctionCall) goja.Value {
				if !settled {
					settled = true
					subscription.cleanup(rt)
					_ = resolve(goja.Undefined())
				}
				return goja.Undefined()
			},
			func(call goja.FunctionCall) goja.Value {
				if !settled {
					settled = true
					subscription.cleanup(rt)
					_ = reject(call.Argument(0))
				}
				return goja.Undefined()
			},
		)
	}

	open := func() {
		handle, err := m.openWriteHandle(name, options)
		if m.scheduler == nil {
			settleOpen(handle, err)
			return
		}
		if !m.scheduler.RunOnLoop(func(*goja.Runtime) { settleOpen(handle, err) }) && handle != nil {
			_ = handle.closeAndWait()
		}
	}
	if m.scheduler == nil {
		open()
	} else {
		go open()
	}
	return rt.ToValue(result)
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
	_ = reject(jsErrorValue(rt, err))
	return rt.ToValue(promise)
}
