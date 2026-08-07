package streams

import (
	"errors"
	"fmt"

	"github.com/dop251/goja"
)

// ReadableStreamSource adapts Go callbacks to an underlying source dictionary.
// Callbacks run on the runtime goroutine and may return a Promise.
type ReadableStreamSource struct {
	Type                  string
	AutoAllocateChunkSize *uint64
	Start                 func(controller *goja.Object) goja.Value
	Pull                  func(controller *goja.Object) goja.Value
	Cancel                func(reason goja.Value) goja.Value
}

// WritableStreamSink adapts Go callbacks to an underlying sink dictionary.
// Callbacks run on the runtime goroutine and may return a Promise.
type WritableStreamSink struct {
	Start func(controller *goja.Object) goja.Value
	Write func(chunk goja.Value, controller *goja.Object) goja.Value
	Close func() goja.Value
	Abort func(reason goja.Value) goja.Value
}

// ChunkConsumer processes one readable-stream chunk. Returning a Promise
// applies backpressure until that Promise settles.
type ChunkConsumer func(chunk goja.Value) goja.Value

// NewReadableStream constructs a canonical ReadableStream backed by Go
// callbacks. At most one queuing strategy may be supplied.
func NewReadableStream(
	rt *goja.Runtime,
	source ReadableStreamSource,
	strategy ...goja.Value,
) (*goja.Object, error) {
	if len(strategy) > 1 {
		return nil, errors.New("streams: NewReadableStream accepts at most one strategy")
	}

	sourceObject := rt.NewObject()
	if source.Type != "" {
		if err := sourceObject.Set("type", source.Type); err != nil {
			return nil, err
		}
	}
	if source.AutoAllocateChunkSize != nil {
		if err := sourceObject.Set("autoAllocateChunkSize", *source.AutoAllocateChunkSize); err != nil {
			return nil, err
		}
	}
	if source.Start != nil {
		if err := sourceObject.Set("start", func(call goja.FunctionCall) goja.Value {
			return valueOrUndefined(source.Start(call.Argument(0).ToObject(rt)))
		}); err != nil {
			return nil, err
		}
	}
	if source.Pull != nil {
		if err := sourceObject.Set("pull", func(call goja.FunctionCall) goja.Value {
			return valueOrUndefined(source.Pull(call.Argument(0).ToObject(rt)))
		}); err != nil {
			return nil, err
		}
	}
	if source.Cancel != nil {
		if err := sourceObject.Set("cancel", func(call goja.FunctionCall) goja.Value {
			return valueOrUndefined(source.Cancel(call.Argument(0)))
		}); err != nil {
			return nil, err
		}
	}

	args := []goja.Value{sourceObject}
	if len(strategy) == 1 {
		args = append(args, valueOrUndefined(strategy[0]))
	}
	return rt.New(getExports(rt).Get("ReadableStream"), args...)
}

// NewWritableStream constructs a canonical WritableStream backed by Go
// callbacks. At most one queuing strategy may be supplied.
func NewWritableStream(
	rt *goja.Runtime,
	sink WritableStreamSink,
	strategy ...goja.Value,
) (*goja.Object, error) {
	if len(strategy) > 1 {
		return nil, errors.New("streams: NewWritableStream accepts at most one strategy")
	}

	sinkObject := rt.NewObject()
	if sink.Start != nil {
		if err := sinkObject.Set("start", func(call goja.FunctionCall) goja.Value {
			return valueOrUndefined(sink.Start(call.Argument(0).ToObject(rt)))
		}); err != nil {
			return nil, err
		}
	}
	if sink.Write != nil {
		if err := sinkObject.Set("write", func(call goja.FunctionCall) goja.Value {
			return valueOrUndefined(sink.Write(
				call.Argument(0),
				call.Argument(1).ToObject(rt),
			))
		}); err != nil {
			return nil, err
		}
	}
	if sink.Close != nil {
		if err := sinkObject.Set("close", func(goja.FunctionCall) goja.Value {
			return valueOrUndefined(sink.Close())
		}); err != nil {
			return nil, err
		}
	}
	if sink.Abort != nil {
		if err := sinkObject.Set("abort", func(call goja.FunctionCall) goja.Value {
			return valueOrUndefined(sink.Abort(call.Argument(0)))
		}); err != nil {
			return nil, err
		}
	}

	args := []goja.Value{sinkObject}
	if len(strategy) == 1 {
		args = append(args, valueOrUndefined(strategy[0]))
	}
	return rt.New(getExports(rt).Get("WritableStream"), args...)
}

// IsReadableStream reports whether value is a ReadableStream created by this
// package in rt.
func IsReadableStream(rt *goja.Runtime, value goja.Value) bool {
	return instanceOfExport(rt, value, "ReadableStream")
}

// IsWritableStream reports whether value is a WritableStream created by this
// package in rt.
func IsWritableStream(rt *goja.Runtime, value goja.Value) bool {
	return instanceOfExport(rt, value, "WritableStream")
}

func instanceOfExport(rt *goja.Runtime, value goja.Value, name string) bool {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return false
	}
	var matches bool
	if exception := rt.Try(func() {
		matches = rt.InstanceOf(value, getExports(rt).Get(name).ToObject(rt))
	}); exception != nil {
		return false
	}
	return matches
}

// ConsumeReadableStream reads a default ReadableStream sequentially. The next
// read starts only after consume's return value has fulfilled.
func ConsumeReadableStream(
	rt *goja.Runtime,
	stream goja.Value,
	consume ChunkConsumer,
) (*goja.Promise, error) {
	if consume == nil {
		return nil, errors.New("streams: chunk consumer is required")
	}
	if !IsReadableStream(rt, stream) {
		return nil, errors.New("streams: value is not a ReadableStream")
	}

	streamObject := stream.ToObject(rt)
	readerValue, err := callObjectMethod(streamObject, "getReader")
	if err != nil {
		return nil, fmt.Errorf("streams: acquire reader: %w", err)
	}
	reader := readerValue.ToObject(rt)
	result, resolve, reject := rt.NewPromise()
	settled := false

	release := func() {
		_, _ = callObjectMethod(reader, "releaseLock")
	}
	fulfill := func() {
		if settled {
			return
		}
		settled = true
		release()
		_ = resolve(goja.Undefined())
	}
	fail := func(reason goja.Value) {
		if settled {
			return
		}
		settled = true
		release()
		_ = reject(valueOrUndefined(reason))
	}

	var pump func()
	pump = func() {
		if settled {
			return
		}
		readResult, readErr := callObjectMethod(reader, "read")
		if readErr != nil {
			fail(errorValue(rt, readErr))
			return
		}
		then(
			rt,
			readResult,
			func(call goja.FunctionCall) goja.Value {
				item := call.Argument(0).ToObject(rt)
				if item.Get("done").ToBoolean() {
					fulfill()
					return goja.Undefined()
				}

				var consumed goja.Value
				if exception := rt.Try(func() {
					consumed = consume(item.Get("value"))
				}); exception != nil {
					cancelReader(rt, reader, exception.Value(), fail)
					return goja.Undefined()
				}
				then(
					rt,
					asPromise(rt, valueOrUndefined(consumed)),
					func(goja.FunctionCall) goja.Value {
						pump()
						return goja.Undefined()
					},
					func(call goja.FunctionCall) goja.Value {
						cancelReader(rt, reader, call.Argument(0), fail)
						return goja.Undefined()
					},
				)
				return goja.Undefined()
			},
			func(call goja.FunctionCall) goja.Value {
				fail(call.Argument(0))
				return goja.Undefined()
			},
		)
	}
	pump()
	return result, nil
}

func cancelReader(
	rt *goja.Runtime,
	reader *goja.Object,
	reason goja.Value,
	done func(goja.Value),
) {
	cancelResult, err := callObjectMethod(reader, "cancel", valueOrUndefined(reason))
	if err != nil {
		done(reason)
		return
	}
	then(
		rt,
		cancelResult,
		func(goja.FunctionCall) goja.Value {
			done(reason)
			return goja.Undefined()
		},
		func(goja.FunctionCall) goja.Value {
			done(reason)
			return goja.Undefined()
		},
	)
}

func callObjectMethod(
	object *goja.Object,
	name string,
	args ...goja.Value,
) (goja.Value, error) {
	method, ok := goja.AssertFunction(object.Get(name))
	if !ok {
		return nil, fmt.Errorf("%s is not callable", name)
	}
	return method(object, args...)
}

func then(
	rt *goja.Runtime,
	promise goja.Value,
	onFulfilled func(goja.FunctionCall) goja.Value,
	onRejected func(goja.FunctionCall) goja.Value,
) {
	object := promise.ToObject(rt)
	method, ok := goja.AssertFunction(object.Get("then"))
	if !ok {
		panic(rt.NewTypeError("streams: expected a Promise-compatible value"))
	}
	if _, err := method(
		object,
		rt.ToValue(onFulfilled),
		rt.ToValue(onRejected),
	); err != nil {
		panic(err)
	}
}

func asPromise(rt *goja.Runtime, value goja.Value) goja.Value {
	promise, resolve, _ := rt.NewPromise()
	if err := resolve(valueOrUndefined(value)); err != nil {
		panic(err)
	}
	return rt.ToValue(promise)
}

func valueOrUndefined(value goja.Value) goja.Value {
	if value == nil {
		return goja.Undefined()
	}
	return value
}

func errorValue(rt *goja.Runtime, err error) goja.Value {
	if exception, ok := err.(*goja.Exception); ok {
		return exception.Value()
	}
	return rt.NewGoError(err)
}
