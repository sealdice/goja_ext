package abort_test

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/abort"
	"github.com/sealdice/goja_ext/require"
	"github.com/sealdice/goja_ext/runtimehost"
)

type directScheduler struct{ rt *goja.Runtime }

func (s *directScheduler) Runtime() *goja.Runtime { return s.rt }
func (s *directScheduler) RunOnLoop(fn func(*goja.Runtime)) bool {
	fn(s.rt)
	return true
}

func TestEnableAndRequireShareCanonicalConstructors(t *testing.T) {
	rt := goja.New()
	new(require.Registry).Enable(rt)
	abort.Enable(rt)

	value, err := rt.RunString(`
		const abort = require("abort");
		AbortController === abort.AbortController &&
		AbortSignal === abort.AbortSignal &&
		require("abort") === abort;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if !value.ToBoolean() {
		t.Fatal("global and CommonJS abort constructors are not canonical")
	}
}

func newRT(t *testing.T) *goja.Runtime {
	t.Helper()
	rt := goja.New()
	if err := runtimehost.BindScheduler(rt, &directScheduler{rt: rt}); err != nil {
		t.Fatal(err)
	}
	abort.Enable(rt)
	// setTimeout stub for AbortSignal.timeout
	_ = rt.Set("setTimeout", func(call goja.FunctionCall) goja.Value {
		if fn, ok := goja.AssertFunction(call.Argument(0)); ok {
			_, _ = fn(goja.Undefined())
		}
		return goja.Undefined()
	})
	return rt
}

func TestAbortController_BasicAndReason(t *testing.T) {
	rt := newRT(t)
	v, err := rt.RunString(`
		const c = new AbortController();
		const results = [];
		results.push(c.signal.aborted);
		c.signal.addEventListener("abort", (e) => results.push(e.type, e.reason));
		c.abort("stop");
		results.push(c.signal.aborted, c.signal.reason);
		JSON.stringify(results);
	`)
	if err != nil {
		t.Fatal(err)
	}
	want := `[false,"abort","stop",true,"stop"]`
	if v.String() != want {
		t.Fatalf("got %s want %s", v.String(), want)
	}
}

func TestAbortSignal_StaticAbortAndTimeout(t *testing.T) {
	rt := newRT(t)
	v, err := rt.RunString(`
		const a = AbortSignal.abort("done");
		const b = AbortSignal.timeout(5);
		JSON.stringify([a.aborted, a.reason, b.aborted, b.reason]);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != `[true,"done",true,"TimeoutError"]` {
		t.Fatalf("got %s", v.String())
	}
}

func TestAbortSignal_ThrowIfAborted(t *testing.T) {
	rt := newRT(t)
	value, err := rt.RunString(`
		const c = new AbortController();
		const reason = { marker: "same" };
		c.abort(reason);
		try { c.signal.throwIfAborted(); false; } catch (e) { e === reason; }
	`)
	if err != nil {
		t.Fatal(err)
	}
	if !value.ToBoolean() {
		t.Fatal("throwIfAborted did not throw the original reason")
	}
}

func TestAbortController_DoubleAbortIsNoop(t *testing.T) {
	rt := newRT(t)
	v, err := rt.RunString(`
		const c = new AbortController();
		let fires = 0;
		c.signal.addEventListener("abort", () => { fires++; });
		c.abort("first");
		c.abort("second");
		JSON.stringify([c.signal.reason, fires]);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != `["first",1]` {
		t.Fatalf("got %s", v.String())
	}
}

func TestAbortController_DefaultReason(t *testing.T) {
	rt := newRT(t)
	v, err := rt.RunString(`
		const c = new AbortController();
		c.abort();
		c.signal.reason;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "Aborted" {
		t.Fatalf("got %s", v.String())
	}
}

func TestAbortSignal_ListenerAddedAfterAbortDoesNotFire(t *testing.T) {
	rt := newRT(t)
	v, err := rt.RunString(`
		const c = new AbortController();
		c.abort("late");
		let captured = null;
		c.signal.addEventListener("abort", (e) => { captured = e.reason; });
		captured;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if !goja.IsNull(v) {
		t.Fatalf("got %s", v.String())
	}
}

func TestAbortSignalTimeoutRequiresScheduler(t *testing.T) {
	rt := goja.New()
	abort.Enable(rt)
	_, err := rt.RunString(`AbortSignal.timeout(1)`)
	if err == nil || !strings.Contains(err.Error(), "scheduler") {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestAbortSignal_RemoveEventListener(t *testing.T) {
	rt := newRT(t)
	v, err := rt.RunString(`
		const c = new AbortController();
		let fires = 0;
		const h = () => { fires++; };
		c.signal.addEventListener("abort", h);
		c.signal.removeEventListener("abort", h);
		c.abort("x");
		JSON.stringify([fires]);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != `[0]` {
		t.Fatalf("got %s", v.String())
	}
}
