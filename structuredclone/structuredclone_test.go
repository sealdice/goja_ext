package structuredclone

import (
	"testing"

	"github.com/dop251/goja"
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
