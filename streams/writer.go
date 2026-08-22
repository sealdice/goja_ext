package streams

import (
	"errors"
	"io"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/runtimehost"
)

// WriterOption configures NewWritableStreamToWriter.
type WriterOption func(*writerConfig)

type writerConfig struct {
	decodeChunk func(rt *goja.Runtime, chunk goja.Value) ([]byte, error)
	mapError    func(rt *goja.Runtime, err error) goja.Value
}

// WithDecodeChunk overrides how JavaScript chunks are decoded to bytes. The
// default accepts ArrayBuffer and ArrayBufferView values.
func WithDecodeChunk(fn func(rt *goja.Runtime, chunk goja.Value) ([]byte, error)) WriterOption {
	return func(config *writerConfig) { config.decodeChunk = fn }
}

// WithMapWriteError overrides how decode and write failures become the
// rejected write reason.
func WithMapWriteError(fn func(rt *goja.Runtime, err error) goja.Value) WriterOption {
	return func(config *writerConfig) { config.mapError = fn }
}

// NewWritableStreamToWriter returns a WritableStream that decodes chunks on
// the loop, writes them on a background goroutine (inline when scheduler is
// nil), and closes the writer on close, abort, and any failure.
func NewWritableStreamToWriter(
	rt *goja.Runtime,
	scheduler runtimehost.Scheduler,
	writer io.WriteCloser,
	opts ...WriterOption,
) (*goja.Object, error) {
	if writer == nil {
		return nil, errors.New("streams: writer is required")
	}
	config := writerConfig{
		decodeChunk: decodeChunkBytes,
		mapError: func(rt *goja.Runtime, err error) goja.Value {
			return errorValue(rt, err)
		},
	}
	for _, opt := range opts {
		opt(&config)
	}

	run := func(op func() error) goja.Value {
		promise, resolve, reject := rt.NewPromise()
		exec := func() {
			err := op()
			settle := func(rt *goja.Runtime) {
				if err != nil {
					_ = reject(config.mapError(rt, err))
					return
				}
				_ = resolve(goja.Undefined())
			}
			if scheduler == nil {
				settle(rt)
				return
			}
			_ = scheduler.RunOnLoop(settle)
		}
		if scheduler == nil {
			exec()
		} else {
			go exec()
		}
		return rt.ToValue(promise)
	}

	return NewWritableStream(rt, WritableStreamSink{
		Write: func(chunk goja.Value, _ *goja.Object) goja.Value {
			data, decodeErr := config.decodeChunk(rt, chunk)
			if decodeErr != nil {
				return run(func() error {
					_ = writer.Close()
					return decodeErr
				})
			}
			return run(func() error {
				if err := writeAll(writer, data); err != nil {
					_ = writer.Close()
					return err
				}
				return nil
			})
		},
		Close: func() goja.Value {
			return run(writer.Close)
		},
		Abort: func(reason goja.Value) goja.Value {
			_ = reason
			return run(writer.Close)
		},
	})
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func decodeChunkBytes(rt *goja.Runtime, chunk goja.Value) ([]byte, error) {
	if chunk == nil || goja.IsUndefined(chunk) || goja.IsNull(chunk) {
		return nil, streamTypeError(rt, "streams: chunk must be an ArrayBuffer or ArrayBufferView")
	}
	switch data := chunk.Export().(type) {
	case []byte:
		return append([]byte(nil), data...), nil
	case goja.ArrayBuffer:
		return append([]byte(nil), data.Bytes()...), nil
	}
	return nil, streamTypeError(rt, "streams: chunk must be an ArrayBuffer or ArrayBufferView")
}

func streamTypeError(rt *goja.Runtime, message string) error {
	var exception error
	if caught := rt.Try(func() {
		panic(rt.NewTypeError(message))
	}); caught != nil {
		exception = caught
	}
	return exception
}
