package structuredclone

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
)

func TestStructuredClone_PrimitivesAndContainers(t *testing.T) {
	rt := goja.New()
	Enable(rt)
	v, err := rt.RunString(`
		const original = { n: 1, s: "x", a: [1, 2, { y: true }] };
		const copy = structuredClone(original);
		original.a[0] = 99;
		original.n = 42;
		JSON.stringify([copy.n, copy.s, copy.a, original.a[0]]);
	`)
	if err != nil {
		t.Fatal(err)
	}
	want := `[1,"x",[1,2,{"y":true}],99]`
	if v.String() != want {
		t.Fatalf("got %s want %s", v.String(), want)
	}
}

func TestStructuredClone_Circular(t *testing.T) {
	rt := goja.New()
	Enable(rt)
	_, err := rt.RunString(`
		const o = { name: "root" };
		o.self = o;
		const copy = structuredClone(o);
		globalThis.__ok = copy.name === "root" && copy.self === copy;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if !rt.Get("__ok").ToBoolean() {
		t.Fatal("circular reference not preserved")
	}
}

func TestStructuredClone_PrimitivesAndDate(t *testing.T) {
	rt := goja.New()
	Enable(rt)
	v, err := rt.RunString(`
		const d = new Date(0);
		const d2 = structuredClone(d);
		JSON.stringify([structuredClone(42), structuredClone("hi"), structuredClone(true), structuredClone(null), d2.getTime()]);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != `[42,"hi",true,null,0]` {
		t.Fatalf("got %s", v.String())
	}
}

func TestStructuredClone_Map(t *testing.T) {
	rt := goja.New()
	Enable(rt)
	v, err := rt.RunString(`
		const m = new Map([["a", 1], ["b", { x: 2 }]]);
		const c = structuredClone(m);
		m.set("a", 999);
		JSON.stringify([c instanceof Map, c.get("a"), c.get("b").x, c.size]);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != `[true,1,2,2]` {
		t.Fatalf("got %s", v.String())
	}
}

func TestStructuredClone_Set(t *testing.T) {
	rt := goja.New()
	Enable(rt)
	v, err := rt.RunString(`
		const s = new Set([1, 2, 3]);
		const c = structuredClone(s);
		s.add(999);
		JSON.stringify([c instanceof Set, c.size, c.has(2), c.has(999)]);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != `[true,3,true,false]` {
		t.Fatalf("got %s", v.String())
	}
}

func TestStructuredClone_DateIndependent(t *testing.T) {
	rt := goja.New()
	Enable(rt)
	v, err := rt.RunString(`
		const d = new Date(123456789000);
		const c = structuredClone(d);
		JSON.stringify([c instanceof Date, c.getTime(), c === d]);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != `[true,123456789000,false]` {
		t.Fatalf("got %s", v.String())
	}
}

func TestStructuredClone_CircularInsideMap(t *testing.T) {
	rt := goja.New()
	Enable(rt)
	_, err := rt.RunString(`
		const m = new Map();
		const inner = { name: "inner" };
		inner.self = inner;
		m.set("obj", inner);
		m.set("self", m);
		const c = structuredClone(m);
		const ci = c.get("obj");
		globalThis.__ok = (c instanceof Map) && ci.self === ci && c.get("self") === c;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if !rt.Get("__ok").ToBoolean() {
		t.Fatal("circular references inside Map not preserved")
	}
}

func TestStructuredClone_NoArgThrows(t *testing.T) {
	rt := goja.New()
	Enable(rt)
	_, err := rt.RunString("structuredClone()")
	if err == nil {
		t.Fatal("expected TypeError for missing argument")
	}
	if !strings.Contains(err.Error(), "value argument is required") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestStructuredClone_UndefinedAndNull(t *testing.T) {
	rt := goja.New()
	Enable(rt)
	v, err := rt.RunString(`JSON.stringify([typeof structuredClone(undefined), structuredClone(null)])`)
	if err != nil {
		t.Fatal(err)
	}
	want := `["undefined",null]`
	if v.String() != want {
		t.Fatalf("got %s want %s", v.String(), want)
	}
}

func TestStructuredClone_CircularSet(t *testing.T) {
	rt := goja.New()
	Enable(rt)
	_, err := rt.RunString(`
		const s = new Set();
		const inner = { n: 1 };
		inner.s = s;
		s.add(inner);
		s.add(s);
		const c = structuredClone(s);
		let found = null;
		for (const v of c) {
			if (v !== c && v !== null && typeof v === "object" && v.n === 1) { found = v; }
		}
		globalThis.__ok = (c instanceof Set) && (c.size === 2) && (found !== null) && (found.s === c);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if !rt.Get("__ok").ToBoolean() {
		t.Fatal("circular references inside Set not preserved")
	}
}

func TestStructuredClone_InvalidDate(t *testing.T) {
	rt := goja.New()
	Enable(rt)
	v, err := rt.RunString(`
		const d = new Date(NaN);
		const c = structuredClone(d);
		JSON.stringify([c instanceof Date, Number.isNaN(c.getTime())])
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != `[true,true]` {
		t.Fatalf("got %s", v.String())
	}
}

func TestStructuredClone_JSONCloneFallback(t *testing.T) {
	rt := goja.New()
	Enable(rt)
	if _, err := rt.RunString(`structuredClone(/abc/)`); err != nil {
		t.Fatalf("regexp clone should not throw: %v", err)
	}
	if _, err := rt.RunString(`structuredClone(() => {})`); err != nil {
		t.Fatalf("function clone should not throw: %v", err)
	}
}

func TestStructuredClone_RequireExport(t *testing.T) {
	rt := goja.New()
	registry := new(require.Registry)
	registry.Enable(rt)
	v, err := rt.RunString(`
		const sc = require("structuredclone");
		JSON.stringify([typeof sc.structuredClone, sc.structuredClone(5)])
	`)
	if err != nil {
		t.Fatal(err)
	}
	want := `["function",5]`
	if v.String() != want {
		t.Fatalf("got %s want %s", v.String(), want)
	}
}
