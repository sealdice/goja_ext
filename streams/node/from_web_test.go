package node

import (
	"testing"
)

func TestNodeReadableToWeb(t *testing.T) {
	result := runNodeStreamsScript(t, `
		const stream = require("stream");
		const r = new stream.Readable({
			read(size) {
				this.push("hello");
				this.push("world");
				this.push(null);
			},
		});
		const web = stream.Readable.toWeb(r);
		const reader = web.getReader();
		const chunks = [];
		const pump = function (r) {
			if (r.done) {
				globalThis.__result = chunks.join("|");
				return;
			}
			chunks.push(String.fromCharCode.apply(null, r.value));
			reader.read().then(pump);
		};
		reader.read().then(pump);
	`)
	if result != "hello|world" {
		t.Fatalf("unexpected: %s", result)
	}
}

func TestNodeReadableFromWeb(t *testing.T) {
	result := runNodeStreamsScript(t, `
		const stream = require("stream");
		const { ReadableStream } = require("streams");
		const web = new ReadableStream({
			start(c) {
				c.enqueue("x");
				c.enqueue("y");
				c.close();
			},
		});
		const r = stream.Readable.fromWeb(web);
		const out = [];
		r.on("data", function (d) {
			out.push(String(d));
		});
		r.on("end", function () {
			globalThis.__result = out.join("|");
		});
	`)
	if result != "x|y" {
		t.Fatalf("unexpected: %s", result)
	}
}

func TestNodeWritableToWeb(t *testing.T) {
	result := runNodeStreamsScript(t, `
		const stream = require("stream");
		const collected = [];
		const w = new stream.Writable({
			write(chunk, encoding, cb) {
				collected.push(String.fromCharCode.apply(null, chunk));
				cb(null);
			},
			final(cb) { cb(null); },
		});
		const web = stream.Writable.toWeb(w);
		const writer = web.getWriter();
		writer.write("hi").then(function () {
			return writer.close();
		}).then(function () {
			globalThis.__result = collected.join("|");
		});
	`)
	if result != "hi" {
		t.Fatalf("unexpected: %s", result)
	}
}

func TestNodeStreamUsesCanonicalEventEmitter(t *testing.T) {
	result := runNodeStreamsScript(t, `
		const stream = require("stream");
		const { EventEmitter } = require("events");
		const r = new stream.Readable();
		const w = new stream.Writable();
		globalThis.__result = String(
			r instanceof EventEmitter && w instanceof EventEmitter
		);
	`)
	if result != "true" {
		t.Fatalf("unexpected: %s", result)
	}
}

func TestNodePipelineAndFinished(t *testing.T) {
	result := runNodeStreamsScript(t, `
		const stream = require("stream");
		const { Buffer } = require("buffer");
		const src = stream.Readable.from(["a", "b", "c"]);
		const out = [];
		const sink = new stream.Writable({
			write(chunk, encoding, cb) {
				out.push(Buffer.from(chunk).toString("utf8"));
				cb(null);
			},
		});
		stream.pipeline(src, sink, function (err) {
			globalThis.__result = (err ? "ERR:" + err.message : out.join(","));
		});
	`)
	if result != "a,b,c" {
		t.Fatalf("unexpected: %s", result)
	}
}

func TestNodeWritableFromWeb(t *testing.T) {
	result := runNodeStreamsScript(t, `
		const stream = require("stream");
		const { WritableStream } = require("streams");
		const collected = [];
		const web = new WritableStream({
			write(chunk) { collected.push(String(chunk)); return Promise.resolve(); },
			close() { return Promise.resolve(); },
		});
		const w = stream.Writable.fromWeb(web);
		w.write("hi");
		w.end();
		let p = Promise.resolve();
		for (let i = 0; i < 10; i++) p = p.then(function () {});
		p.then(function () {
			globalThis.__result = collected.join("|");
		});
	`)
	if result != "hi" {
		t.Fatalf("unexpected Writable.fromWeb result: %s", result)
	}
}

func TestNodeDuplexPair(t *testing.T) {
	result := runNodeStreamsScript(t, `
		const stream = require("stream");
		const pair = stream.duplexPair();
		const a = pair[0];
		const b = pair[1];
		const received = [];
		b.on("data", function (chunk) {
			received.push(String.fromCharCode.apply(null, chunk));
		});
		b.on("end", function () {
			globalThis.__result = received.join(",");
		});
		a.write("hello");
		a.end();
	`)
	if result != "hello" {
		t.Fatalf("unexpected duplexPair result: %s", result)
	}
}

func TestNodeObjectMode(t *testing.T) {
	result := runNodeStreamsScript(t, `
		const stream = require("stream");
		const r = stream.Readable.from([{ a: 1 }, { b: 2 }]);
		const out = [];
		r.on("data", function (d) { out.push(JSON.stringify(d)); });
		r.on("end", function () {
			globalThis.__result = out.join(",");
		});
	`)
	if result != "{\"a\":1},{\"b\":2}" {
		t.Fatalf("unexpected objectMode result: %s", result)
	}
}
