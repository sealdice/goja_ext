package streams_test

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/streams"
)

func TestGoReadableStreamSourceAndConsumer(t *testing.T) {
	rt := goja.New()
	chunks := []string{"a", "b"}
	index := 0

	stream, err := streams.NewReadableStream(rt, streams.ReadableStreamSource{
		Pull: func(controller *goja.Object) goja.Value {
			if index == len(chunks) {
				callMethod(t, controller, "close")
				return goja.Undefined()
			}
			callMethod(t, controller, "enqueue", rt.ToValue(chunks[index]))
			index++
			return goja.Undefined()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !streams.IsReadableStream(rt, stream) {
		t.Fatal("constructed object is not recognized as a ReadableStream")
	}
	if streams.IsWritableStream(rt, stream) {
		t.Fatal("readable stream was recognized as writable")
	}

	var consumed string
	promise, err := streams.ConsumeReadableStream(rt, stream, func(chunk goja.Value) goja.Value {
		consumed += chunk.String()
		return goja.Undefined()
	})
	if err != nil {
		t.Fatal(err)
	}
	if promise.State() != goja.PromiseStateFulfilled {
		t.Fatalf("consume promise state: %v, result: %v", promise.State(), promise.Result())
	}
	if consumed != "ab" {
		t.Fatalf("unexpected consumed chunks: %q", consumed)
	}
}

func TestGoWritableStreamSink(t *testing.T) {
	rt := goja.New()
	var written string
	closed := false

	stream, err := streams.NewWritableStream(rt, streams.WritableStreamSink{
		Write: func(chunk goja.Value, _ *goja.Object) goja.Value {
			written += chunk.String()
			return goja.Undefined()
		},
		Close: func() goja.Value {
			closed = true
			return goja.Undefined()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !streams.IsWritableStream(rt, stream) {
		t.Fatal("constructed object is not recognized as a WritableStream")
	}

	if err = rt.Set("__stream", stream); err != nil {
		t.Fatal(err)
	}
	value, err := rt.RunString(`
		(function () {
			const writer = __stream.getWriter();
			return writer.write("a")
				.then(function () { return writer.write("b"); })
				.then(function () { return writer.close(); });
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	promise := value.Export().(*goja.Promise)
	if promise.State() != goja.PromiseStateFulfilled {
		t.Fatalf("write promise state: %v, result: %v", promise.State(), promise.Result())
	}
	if written != "ab" {
		t.Fatalf("unexpected written chunks: %q", written)
	}
	if !closed {
		t.Fatal("sink was not closed")
	}
}

func callMethod(t *testing.T, object *goja.Object, name string, args ...goja.Value) goja.Value {
	t.Helper()
	method, ok := goja.AssertFunction(object.Get(name))
	if !ok {
		t.Fatalf("%s is not callable", name)
	}
	value, err := method(object, args...)
	if err != nil {
		t.Fatalf("%s failed: %v", name, err)
	}
	return value
}
