package timers_test

import (
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/require"
)

func runWithLoop(t *testing.T, script string) string {
	t.Helper()
	loop := eventloop.NewEventLoop(eventloop.EnableConsole(false))
	go loop.StartInForeground()
	t.Cleanup(func() { loop.Stop() })

	done := make(chan struct{})
	var result string
	loop.RunOnLoop(func(vm *goja.Runtime) {
		if _, err := vm.RunString(script); err != nil {
			t.Errorf("run: %v", err)
		}
		var poll func()
		poll = func() {
			loop.SetTimeout(func(vm *goja.Runtime) {
				if value := vm.Get("__result"); value != nil && !goja.IsUndefined(value) {
					result = value.String()
					close(done)
					return
				}
				poll()
			}, 5*time.Millisecond)
		}
		poll()
	})
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
	return result
}

func TestTimersModuleSurface(t *testing.T) {
	out := runWithLoop(t, `
		const t = require("timers");
		globalThis.__result = String([
			typeof t.setTimeout, typeof t.setInterval, typeof t.setImmediate,
			typeof t.clearTimeout, typeof t.clearInterval, typeof t.clearImmediate,
		].join(","));
	`)
	if out != "function,function,function,function,function,function" {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestTimersModuleIdentityWithGlobal(t *testing.T) {
	out := runWithLoop(t, `
		const t = require("timers");
		const n = require("node:timers");
		globalThis.__result = String(t.setTimeout === setTimeout && t.setInterval === setInterval && t === n);
	`)
	if out != "true" {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestTimersPromisesSetTimeout(t *testing.T) {
	out := runWithLoop(t, `
		const tp = require("timers/promises");
		Promise.all([
			tp.setTimeout(1, "a"),
			tp.setImmediate("b"),
		]).then(function (vals) {
			globalThis.__result = String(vals.join(","));
		});
	`)
	if out != "a,b" {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestTimersPromisesSetInterval(t *testing.T) {
	out := runWithLoop(t, `
		const tp = require("timers/promises");
		const it = tp.setInterval(1, "x");
		const collected = [];
		const run = function () {
			it.next().then(function (r) {
				if (r.done) { globalThis.__result = collected.join(","); return; }
				collected.push(r.value);
				run();
			});
		};
		run();
		setTimeout(function () { it.return(); }, 10);
	`)
	if strings.Count(out, "x") < 3 {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestTimersWithoutLoopThrows(t *testing.T) {
	rt := goja.New()
	registry := new(require.Registry)
	registry.Enable(rt)
	_, err := rt.RunString(`
		const t = require("timers");
		try {
			t.setTimeout(function () {}, 1);
			throw new Error("should have thrown");
		} catch (e) {
			if (e.message.indexOf("event loop") === -1) throw e;
			"ok";
		}
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestTimersClearInterval(t *testing.T) {
	out := runWithLoop(t, `
		const t = require("timers");
		let cnt = 0;
		const id = t.setInterval(function () { cnt++; }, 5);
		setTimeout(function () { t.clearInterval(id); }, 20);
		setTimeout(function () {
			const first = cnt;
			setTimeout(function () {
				globalThis.__result = String(first === cnt);
			}, 25);
		}, 35);
	`)
	if out != "true" {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestTimersSchedulerWait(t *testing.T) {
	out := runWithLoop(t, `
		const tp = require("timers/promises");
		tp.scheduler.wait(5).then(function (v) {
			globalThis.__result = String(v === undefined);
		});
	`)
	if out != "true" {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestTimersResolveGlobalsDynamically(t *testing.T) {
	rt := goja.New()
	registry := new(require.Registry)
	registry.Enable(rt)
	if _, err := rt.RunString(`globalThis.__timers = require("timers")`); err != nil {
		t.Fatal(err)
	}
	if err := rt.Set("setTimeout", func(goja.FunctionCall) goja.Value {
		return rt.ToValue("late timer")
	}); err != nil {
		t.Fatal(err)
	}
	value, err := rt.RunString(`__timers.setTimeout()`)
	if err != nil {
		t.Fatal(err)
	}
	if got := value.String(); got != "late timer" {
		t.Fatalf("dynamic timer result = %q", got)
	}
}

func TestTimersPromisesAbortAndCleanup(t *testing.T) {
	out := runWithLoop(t, `
		const tp = require("timers/promises");
		let handler;
		let removed = 0;
		const signal = {
			aborted: false,
			reason: "stop",
			addEventListener(type, fn) { handler = fn; },
			removeEventListener(type, fn) { if (handler === fn) removed++; }
		};
		tp.setTimeout(100, "late", { signal }).then(
			() => { globalThis.__result = "resolved"; },
			(error) => { globalThis.__result = error.name + "|" + error.cause + "|" + removed; }
		);
		signal.aborted = true;
		handler();
	`)
	if out != "AbortError|stop|1" {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestTimersPromisesRefFalseUnrefsHandle(t *testing.T) {
	out := runWithLoop(t, `
		const original = globalThis.setTimeout;
		let unrefed = 0;
		globalThis.setTimeout = function (fn, delay) {
			const handle = original(fn, delay);
			const originalUnref = handle.unref;
			handle.unref = function () { unrefed++; return originalUnref.call(handle); };
			return handle;
		};
		const tp = require("timers/promises");
		tp.setTimeout(1, "ok", { ref: false }).then(function (value) {
			globalThis.__result = value + "|" + unrefed;
		});
		globalThis.setTimeout = original;
	`)
	if out != "ok|1" {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestTimersPromisesIntervalAbortRejectsPendingNext(t *testing.T) {
	out := runWithLoop(t, `
		const tp = require("timers/promises");
		let handler;
		let removed = 0;
		const signal = {
			aborted: false,
			reason: 42,
			addEventListener(type, fn) { handler = fn; },
			removeEventListener(type, fn) { if (handler === fn) removed++; }
		};
		const iterator = tp.setInterval(100, "tick", { signal });
		iterator.next().then(
			() => { globalThis.__result = "resolved"; },
			(error) => { globalThis.__result = error.name + "|" + error.cause + "|" + removed; }
		);
		signal.aborted = true;
		handler();
	`)
	if out != "AbortError|42|1" {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestTimersClearTimeout(t *testing.T) {
	out := runWithLoop(t, `
		const t = require("timers");
		let fired = 0;
		const id = t.setTimeout(function () { fired++; }, 5);
		t.clearTimeout(id);
		setTimeout(function () {
			globalThis.__result = String(fired);
		}, 30);
	`)
	if out != "0" {
		t.Fatalf("unexpected: %s", out)
	}
}
