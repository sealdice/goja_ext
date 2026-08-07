package node

import (
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/eventloop"
)

func runNodeStreamsScript(t *testing.T, script string) string {
	t.Helper()

	loop := eventloop.NewEventLoop(eventloop.EnableConsole(false))
	go loop.StartInForeground()
	t.Cleanup(func() { loop.Stop() })

	done := make(chan struct{})
	var result string
	var runErr error
	var rec any

	loop.RunOnLoop(func(vm *goja.Runtime) {
		defer func() {
			rec = recover()
			loop.SetTimeout(func(vm *goja.Runtime) {
				if value := vm.Get("__result"); value != nil && !goja.IsUndefined(value) {
					result = value.String()
				}
				close(done)
			}, 10*time.Millisecond)
		}()
		_, runErr = vm.RunString(script)
	})

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for node streams script")
	}
	if rec != nil {
		t.Fatalf("panic: %v", rec)
	}
	if runErr != nil {
		t.Fatalf("script failed: %v", runErr)
	}
	return result
}

func TestNodeStreamModuleSurface(t *testing.T) {
	result := runNodeStreamsScript(t, `
		const stream = require("stream");
		const nodeStream = require("node:stream");
		if (stream.Readable !== nodeStream.Readable) throw new Error("alias identity");
		if (typeof stream.Readable !== "function") throw new Error("Readable missing");
		if (typeof stream.Writable !== "function") throw new Error("Writable missing");
		if (typeof stream.Duplex !== "function") throw new Error("Duplex missing");
		if (typeof stream.Transform !== "function") throw new Error("Transform missing");
		if (typeof stream.PassThrough !== "function") throw new Error("PassThrough missing");
		globalThis.__result = String([
			typeof stream.pipeline,
			typeof stream.finished,
			typeof stream.addAbortSignal,
			typeof stream.Readable.from,
		].join(","));
	`)
	if result != "function,function,function,function" {
		t.Fatalf("unexpected node stream surface: %s", result)
	}
}

func TestNodeTransformAndEvents(t *testing.T) {
	result := runNodeStreamsScript(t, `
		const stream = require("stream");
		const { Buffer } = require("buffer");
		const output = [];
		const transform = new stream.Transform({
			transform(chunk, encoding, callback) {
				callback(null, Buffer.from(chunk).toString("utf8").toUpperCase());
			},
		});
		transform.on("data", function (chunk) {
			output.push(Buffer.from(chunk).toString("utf8"));
		});
		transform.on("end", function () {
			globalThis.__result = output.join(",");
		});
		transform.write("a");
		transform.write("b");
		transform.end("c");
	`)
	if result != "A,B,C" {
		t.Fatalf("unexpected node transform output: %s", result)
	}
}
