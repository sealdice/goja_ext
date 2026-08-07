package events

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/abort"
	"github.com/sealdice/goja_ext/require"
)

func newRT(t *testing.T) *goja.Runtime {
	t.Helper()
	rt := goja.New()
	registry := new(require.Registry)
	registry.Enable(rt)
	RegisterWithRegistry(registry)
	abort.Enable(rt)
	return rt
}

func runScript(t *testing.T, rt *goja.Runtime, script string) string {
	t.Helper()
	v, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return v.String()
}

func TestEventEmitterBasicAndInstanceof(t *testing.T) {
	rt := newRT(t)
	out := runScript(t, rt, `
		const { EventEmitter } = require("events");
		const nodeEE = require("node:events").EventEmitter;
		if (EventEmitter !== nodeEE) throw new Error("alias identity");
		const e = new EventEmitter();
		const calls = [];
		e.on("hello", function (x) { calls.push(x); });
		e.emit("hello", "world");
		e.emit("hello", "again");
		JSON.stringify([calls, e instanceof EventEmitter, EventEmitter.EventEmitter === EventEmitter]);
	`)
	if out != `[["world","again"],true,true]` {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestEventEmitterSubclassAndCall(t *testing.T) {
	rt := newRT(t)
	out := runScript(t, rt, `
		const EventEmitter = require("events").EventEmitter;
		class MyEmitter extends EventEmitter {
			constructor() { super(); this.n = 0; }
			bump() { this.n++; this.emit("bumped", this.n); }
		}
		const m = new MyEmitter();
		let last = 0;
		m.on("bumped", (v) => { last = v; });
		m.bump(); m.bump();
		const plain = {};
		EventEmitter.call(plain);
		Object.setPrototypeOf(plain, EventEmitter.prototype);
		plain.on("x", function () {});
		const ok = plain.listenerCount("x") === 1;
		JSON.stringify([last, m instanceof EventEmitter, ok]);
	`)
	if out != `[2,true,true]` {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestEventEmitterListenersShape(t *testing.T) {
	rt := newRT(t)
	out := runScript(t, rt, `
		const { EventEmitter } = require("events");
		const e = new EventEmitter();
		function a() {} function b() {}
		e.on("x", a).once("x", b);
		const l = e.listeners("x");
		const r = e.rawListeners("x");
		JSON.stringify([l.length, r.length, l[0] === a, l[1] === b, e.listenerCount("x")]);
	`)
	if out != `[2,2,true,true,2]` {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestEventEmitterUnhandledErrorThrowsSync(t *testing.T) {
	rt := newRT(t)
	_, err := rt.RunString(`
		const { EventEmitter } = require("events");
		const e = new EventEmitter();
		try {
			e.emit("error", new Error("boom"));
			throw new Error("should have thrown");
		} catch (err) {
			if (err.message !== "boom") throw new Error("wrong error: " + err.message);
		}
		"ok";
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestEventEmitterNewListenerOrder(t *testing.T) {
	rt := newRT(t)
	out := runScript(t, rt, `
		const { EventEmitter } = require("events");
		const e = new EventEmitter();
		const order = [];
		function handler() {}
		e.on("newListener", function (name, fn) { order.push("nl:" + name); });
		e.on("removeListener", function (name, fn) { order.push("rl:" + name); });
		e.on("x", handler);
		e.removeListener("x", handler);
		JSON.stringify(order);
	`)
	if out != `["nl:removeListener","nl:x","rl:x"]` {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestEventsOnceWithSignal(t *testing.T) {
	rt := newRT(t)
	runScript(t, rt, `
		const events = require("events");
		const { AbortController } = globalThis;
		const e = new events.EventEmitter();
		const c = new AbortController();
		globalThis.__settled = "";
		events.once(e, "foo", { signal: c.signal }).then(
			(args) => { __settled = "ok:" + args.join(","); },
			(err) => { __settled = "aborted:" + err.name; }
		);
		c.abort(new Error("stop"));
	`)
	out := runScript(t, rt, `JSON.stringify(globalThis.__settled);`)
	if out != `"aborted:AbortError"` {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestEventsAddAbortListener(t *testing.T) {
	rt := newRT(t)
	out := runScript(t, rt, `
		const events = require("events");
		const { AbortController } = globalThis;
		const c = new AbortController();
		let fired = 0;
		events.addAbortListener(c.signal, function () { fired++; });
		c.abort();
		c.abort();
		JSON.stringify(fired);
	`)
	if out != `1` {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestEventsOnceAndGetEventListeners(t *testing.T) {
	rt := newRT(t)
	runScript(t, rt, `
		const events = require("events");
		const { EventEmitter } = events;
		const e = new EventEmitter();
		globalThis.__got = null;
		events.once(e, "foo").then(function (args) { __got = args; });
		e.emit("foo", 1, 2);
		globalThis.__n = events.getEventListeners(e, "foo").length;
	`)
	out := runScript(t, rt, `JSON.stringify([globalThis.__got, globalThis.__n]);`)
	if out != `[[1,2],0]` {
		t.Fatalf("unexpected: %s", out)
	}
}
