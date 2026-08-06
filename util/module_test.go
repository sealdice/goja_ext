package util

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
)

func TestUtil_Format(t *testing.T) {
	vm := goja.New()
	util := New(vm)

	var b bytes.Buffer
	util.Format(&b, "Test: %% %д %s %d, %j", vm.ToValue("string"), vm.ToValue(42), vm.NewObject())

	if res := b.String(); res != "Test: % %д string 42, {}" {
		t.Fatalf("Unexpected result: '%s'", res)
	}
}

func TestUtil_Format_NoArgs(t *testing.T) {
	vm := goja.New()
	util := New(vm)

	var b bytes.Buffer
	util.Format(&b, "Test: %s %d, %j")

	if res := b.String(); res != "Test: %s %d, %j" {
		t.Fatalf("Unexpected result: '%s'", res)
	}
}

func TestUtil_Format_LessArgs(t *testing.T) {
	vm := goja.New()
	util := New(vm)

	var b bytes.Buffer
	util.Format(&b, "Test: %s %d, %j", vm.ToValue("string"), vm.ToValue(42))

	if res := b.String(); res != "Test: string 42, %j" {
		t.Fatalf("Unexpected result: '%s'", res)
	}
}

func TestUtil_Format_MoreArgs(t *testing.T) {
	vm := goja.New()
	util := New(vm)

	var b bytes.Buffer
	util.Format(&b, "Test: %s %d, %j", vm.ToValue("string"), vm.ToValue(42), vm.NewObject(), vm.ToValue(42.42))

	if res := b.String(); res != "Test: string 42, {} 42.42" {
		t.Fatalf("Unexpected result: '%s'", res)
	}
}

func TestJSNoArgs(t *testing.T) {
	vm := goja.New()
	new(require.Registry).Enable(vm)

	if util, ok := require.Require(vm, ModuleName).(*goja.Object); ok {
		if format, ok := goja.AssertFunction(util.Get("format")); ok {
			res, err := format(util)
			if err != nil {
				t.Fatal(err)
			}
			if v := res.Export(); v != "" {
				t.Fatalf("Unexpected result: %v", v)
			}
		}
	}
}

func TestUtil_Inspect(t *testing.T) {
	vm := goja.New()
	new(require.Registry).Enable(vm)
	utilExports := require.Require(vm, ModuleName).(*goja.Object)
	inspect, ok := goja.AssertFunction(utilExports.Get("inspect"))
	if !ok {
		t.Fatal("util.inspect not found")
	}

	cases := []struct {
		name string
		js   string
		want string
	}{
		{"empty object", `({})`, "{}"},
		{"nested object", `({ a: 1, b: { c: 2 } })`, `{ 'a': 1, 'b': { 'c': 2 } }`},
		{"array", `[1, "x", true]`, `[ 1, 'x', true ]`},
		{"string", `"hi"`, `'hi'`},
		{"undefined", `undefined`, "undefined"},
		{"null", `null`, "null"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			val, err := vm.RunString(c.js)
			if err != nil {
				t.Fatal(err)
			}
			res, err := inspect(utilExports, val)
			if err != nil {
				t.Fatal(err)
			}
			if res.String() != c.want {
				t.Fatalf("inspect(%s): got %q want %q", c.js, res.String(), c.want)
			}
		})
	}
}

func TestUtil_Inspect_Circular(t *testing.T) {
	vm := goja.New()
	new(require.Registry).Enable(vm)
	utilExports := require.Require(vm, ModuleName).(*goja.Object)
	inspect, _ := goja.AssertFunction(utilExports.Get("inspect"))

	_, err := vm.RunString(`const o = {}; o.self = o;`)
	if err != nil {
		t.Fatal(err)
	}
	val := vm.Get("o")
	res, err := inspect(utilExports, val)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.String(), "[Circular]") {
		t.Fatalf("expected [Circular] in output, got %q", res.String())
	}
}

func TestUtil_Inspect_UndefinedOrNullOptions(t *testing.T) {
	vm := goja.New()
	new(require.Registry).Enable(vm)
	utilExports := require.Require(vm, ModuleName).(*goja.Object)
	inspect, _ := goja.AssertFunction(utilExports.Get("inspect"))

	for _, opts := range []string{"undefined", "null"} {
		val, err := vm.RunString(`({ a: 1 })`)
		if err != nil {
			t.Fatal(err)
		}
		// inspect(value, undefined|null) must NOT throw and must equal inspect(value).
		var optsVal goja.Value
		if opts == "undefined" {
			optsVal = goja.Undefined()
		} else {
			optsVal = goja.Null()
		}
		res, err := inspect(utilExports, val, optsVal)
		if err != nil {
			t.Fatalf("inspect(_, %s) errored: %v", opts, err)
		}
		if res.String() != `{ 'a': 1 }` {
			t.Fatalf("inspect(_, %s) = %q", opts, res.String())
		}
	}
}
