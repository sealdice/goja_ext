package abort

import (
	"testing"

	"github.com/dop251/goja"
)

func newRT(t *testing.T) *goja.Runtime {
	t.Helper()
	rt := goja.New()
	Enable(rt)
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
	_, err := rt.RunString(`
		const c = new AbortController();
		c.abort();
		try { c.signal.throwIfAborted(); throw "not-aborted"; } catch (e) { globalThis.__caught = String(e); }
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := rt.Get("__caught").String(); got == "not-aborted" {
		t.Fatalf("expected throwIfAborted to throw, got %s", got)
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

func TestAbortSignal_ListenerAddedAfterAbortFiresImmediately(t *testing.T) {
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
	if v.String() != "late" {
		t.Fatalf("got %s", v.String())
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
