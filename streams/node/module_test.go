package node

import (
	"testing"
	"time"

	"github.com/dop251/goja"
	_ "github.com/sealdice/goja_ext/abort"
	_ "github.com/sealdice/goja_ext/buffer"
	"github.com/sealdice/goja_ext/eventloop"
	webstreams "github.com/sealdice/goja_ext/streams"
)

func TestNodeStreamInitializationIsPrivateAndWebStreamsAreLazy(t *testing.T) {
	rt := goja.New()
	if webstreams.Initialized(rt) {
		t.Fatal("fresh runtime already has Web Streams")
	}
	exports := Exports(rt)
	if webstreams.Initialized(rt) {
		t.Fatal("Node streams eagerly initialized Web Streams")
	}
	if err := rt.Set("nodeStream", exports); err != nil {
		t.Fatal(err)
	}
	value, err := rt.RunString(`
		Reflect.ownKeys(globalThis)
			.map(function (key) { return String(key); })
			.filter(function (key) { return key.indexOf("goja_ext") !== -1; })
			.join(",");
	`)
	if err != nil {
		t.Fatal(err)
	}
	if leaked := value.String(); leaked != "" {
		t.Fatalf("private globals leaked: %s", leaked)
	}
	for _, name := range []string{"self", "queueMicrotask", "__goja_ext_canonical_events", "__goja_ext_streams_canonical"} {
		value := rt.Get(name)
		if value != nil && !goja.IsUndefined(value) {
			t.Errorf("global %s leaked as %s", name, value.String())
		}
	}

	_, err = rt.RunString(`
		const readable = new nodeStream.Readable({ read() { this.push(null); } });
		nodeStream.Readable.toWeb(readable);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if !webstreams.Initialized(rt) {
		t.Fatal("Web adapter did not lazily initialize Web Streams")
	}
}

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

func TestNodeFinished(t *testing.T) {
	result := runNodeStreamsScript(t, `
		const stream = require("stream");
		const w = new stream.Writable({
			write(chunk, encoding, cb) { cb(null); },
		});
		stream.finished(w, function (err) {
			globalThis.__result = String(err);
		});
		w.write("a");
		w.end();
	`)
	if result != "null" {
		t.Fatalf("unexpected finished result: %s", result)
	}
}

func TestNodeFinishedDestroyError(t *testing.T) {
	result := runNodeStreamsScript(t, `
		const stream = require("stream");
		const w = new stream.Writable({
			write(chunk, encoding, cb) { cb(null); },
		});
		stream.finished(w, { cleanup: true }, function (err) {
			globalThis.__result = String(err);
		});
		w.destroy(new Error("kaboom"));
	`)
	if result != "Error: kaboom" {
		t.Fatalf("unexpected finished error result: %s", result)
	}
}

func TestNodeAddAbortSignal(t *testing.T) {
	result := runNodeStreamsScript(t, `
		const stream = require("stream");
		const { AbortController } = require("abort");
		const ac = new AbortController();
		const r = new stream.Readable({ read() {} });
		stream.addAbortSignal(ac.signal, r);
		r.on("error", function (err) {
			globalThis.__result = err.message;
		});
		ac.abort(new Error("boom"));
	`)
	if result != "boom" {
		t.Fatalf("unexpected addAbortSignal result: %s", result)
	}
}
