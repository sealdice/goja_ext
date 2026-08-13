package structuredclone_test

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
	"github.com/sealdice/goja_ext/structuredclone"
)

func TestStructuredClone_PrimitivesAndContainers(t *testing.T) {
	rt := goja.New()
	structuredclone.Enable(rt)
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
	structuredclone.Enable(rt)
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
	structuredclone.Enable(rt)
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
	structuredclone.Enable(rt)
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
	structuredclone.Enable(rt)
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
	structuredclone.Enable(rt)
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
	structuredclone.Enable(rt)
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
	structuredclone.Enable(rt)
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
	structuredclone.Enable(rt)
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
	structuredclone.Enable(rt)
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
	structuredclone.Enable(rt)
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

func TestStructuredClone_RegExp(t *testing.T) {
	rt := goja.New()
	structuredclone.Enable(rt)
	v, err := rt.RunString(`
		const original = /ab+/gi;
		original.lastIndex = 4;
		const copy = structuredClone(original);
		JSON.stringify([copy instanceof RegExp, copy.source, copy.flags, copy.lastIndex, copy === original]);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := v.String(), `[true,"ab+","gi",0,false]`; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestStructuredClone_ArrayBufferAndViews(t *testing.T) {
	rt := goja.New()
	structuredclone.Enable(rt)
	v, err := rt.RunString(`
		const buffer = new ArrayBuffer(6);
		const bytes = new Uint8Array(buffer);
		bytes.set([1, 2, 3, 4, 5, 6]);
		const original = {
			buffer,
			view: new Uint16Array(buffer, 2, 2),
			dataView: new DataView(buffer, 1, 4)
		};
		const copy = structuredClone(original);
		bytes[2] = 99;
		JSON.stringify([
			copy.buffer instanceof ArrayBuffer,
			copy.view instanceof Uint16Array,
			copy.dataView instanceof DataView,
			copy.view.buffer === copy.buffer,
			copy.dataView.buffer === copy.buffer,
			copy.view.byteOffset,
			copy.view.length,
			Array.from(new Uint8Array(copy.buffer))
		]);
	`)
	if err != nil {
		t.Fatal(err)
	}
	want := `[true,true,true,true,true,2,2,[1,2,3,4,5,6]]`
	if v.String() != want {
		t.Fatalf("got %s want %s", v.String(), want)
	}
}

func TestStructuredClone_UnsupportedValuesThrowDataCloneError(t *testing.T) {
	rt := goja.New()
	structuredclone.Enable(rt)
	v, err := rt.RunString(`
		const values = [
			() => {},
			Symbol("x"),
			new WeakMap(),
			Promise.resolve(1),
			{ nested: () => {} }
		];
		values.map((value) => {
			try {
				structuredClone(value);
				return "missing";
			} catch (error) {
				return error.name + ":" + error.code;
			}
		}).join(",");
	`)
	if err != nil {
		t.Fatal(err)
	}
	want := "DataCloneError:25,DataCloneError:25,DataCloneError:25,DataCloneError:25,DataCloneError:25"
	if v.String() != want {
		t.Fatalf("got %s want %s", v.String(), want)
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
