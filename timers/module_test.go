package timers

import (
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/eventloop"
	"github.com/sealdice/goja_ext/require"
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
