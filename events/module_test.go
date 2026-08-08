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
		JSON.stringify([
			l.length, r.length, l[0] === a, l[1] === b,
			r[0] === a, r[1] !== b, r[1].listener === b,
			e.listenerCount("x")
		]);
	`)
	if out != `[2,2,true,true,true,true,true,2]` {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestEventEmitterMaxListenersIsPerInstanceAndChainable(t *testing.T) {
	rt := newRT(t)
	out := runScript(t, rt, `
		const { EventEmitter } = require("events");
		const first = new EventEmitter();
		const second = new EventEmitter();
		const returned = first.setMaxListeners(3);
		JSON.stringify([
			returned === first,
			first.getMaxListeners(),
			second.getMaxListeners(),
			EventEmitter.defaultMaxListeners
		]);
	`)
	if out != `[true,3,10,10]` {
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

func TestEventOnceRemoval(t *testing.T) {
	rt := newRT(t)
	out := runScript(t, rt, `
		const { EventEmitter } = require("events");
		const e = new EventEmitter();
		let cnt = 0;
		e.once("x", function () { cnt++; });
		e.emit("x");
		const after1 = cnt;
		const after1Count = e.listenerCount("x");
		e.emit("x");
		JSON.stringify([cnt, after1, after1Count]);
	`)
	if out != `[1,1,0]` {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestEventPrepend(t *testing.T) {
	rt := newRT(t)
	out := runScript(t, rt, `
		const { EventEmitter } = require("events");
		const e = new EventEmitter();
		const order = [];
		e.on("x", function () { order.push("a"); });
		e.prependListener("x", function () { order.push("b"); });
		e.emit("x");
		JSON.stringify(order);
	`)
	if out != `["b","a"]` {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestEventRemoveAllListeners(t *testing.T) {
	rt := newRT(t)
	out := runScript(t, rt, `
		const { EventEmitter } = require("events");
		const e1 = new EventEmitter();
		e1.on("a", function () {}).on("a", function () {});
		e1.removeAllListeners("a");
		const a = [e1.listenerCount("a"), e1.emit("a")];
		const e2 = new EventEmitter();
		e2.on("a", function () {}).on("b", function () {});
		e2.removeAllListeners();
		const b = [e2.listenerCount("a"), e2.listenerCount("b")];
		JSON.stringify([a, b]);
	`)
	if out != `[[0,false],[0,0]]` {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestEventErrorWithListener(t *testing.T) {
	rt := newRT(t)
	out := runScript(t, rt, `
		const { EventEmitter } = require("events");
		const e = new EventEmitter();
		let got = null;
		e.on("error", function (err) { got = err; });
		const err = new Error("x");
		e.emit("error", err);
		JSON.stringify([got === err, got.message]);
	`)
	if out != `[true,"x"]` {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestEventEmitReturnValue(t *testing.T) {
	rt := newRT(t)
	out := runScript(t, rt, `
		const { EventEmitter } = require("events");
		const e = new EventEmitter();
		const before = e.emit("nope");
		e.on("has", function () {});
		const after = e.emit("has");
		JSON.stringify([before, after]);
	`)
	if out != `[false,true]` {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestEventThisBinding(t *testing.T) {
	rt := newRT(t)
	out := runScript(t, rt, `
		const { EventEmitter } = require("events");
		const e = new EventEmitter();
		let self = null;
		e.on("x", function () { self = this; });
		e.emit("x");
		JSON.stringify([self === e]);
	`)
	if out != `[true]` {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestEventEventNames(t *testing.T) {
	rt := newRT(t)
	out := runScript(t, rt, `
		const { EventEmitter } = require("events");
		const e = new EventEmitter();
		e.on("a", function () {}).on("b", function () {}).on("c", function () {});
		const names = e.eventNames();
		JSON.stringify([names.length, names.indexOf("a") !== -1, names.indexOf("b") !== -1, names.indexOf("c") !== -1]);
	`)
	if out != `[3,true,true,true]` {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestEventOnIterator(t *testing.T) {
	rt := newRT(t)
	runScript(t, rt, `
		const events = require("events");
		const { EventEmitter } = events;
		const e = new EventEmitter();
		const iter = events.on(e, "data");
		e.emit("data", 1);
		e.emit("data", 2);
		const collected = [];
		iter.next().then(function (r1) {
			collected.push(r1.value);
			return iter.next();
		}).then(function (r2) {
			collected.push(r2.value);
			return iter.return();
		}).then(function () {
			globalThis.__result = JSON.stringify(collected);
		});
	`)
	out := runScript(t, rt, `globalThis.__result;`)
	if out != `[[1],[2]]` {
		t.Fatalf("unexpected: %s", out)
	}
}
