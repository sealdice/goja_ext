package util_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	utilpkg "github.com/dop251/goja_nodejs/util"
)

func TestUtil_Format(t *testing.T) {
	vm := goja.New()
	util := utilpkg.New(vm)

	var b bytes.Buffer
	util.Format(&b, "Test: %% %д %s %d, %j", vm.ToValue("string"), vm.ToValue(42), vm.NewObject())

	if res := b.String(); res != "Test: % %д string 42, {}" {
		t.Fatalf("Unexpected result: '%s'", res)
	}
}

func TestUtil_Format_NoArgs(t *testing.T) {
	vm := goja.New()
	util := utilpkg.New(vm)

	var b bytes.Buffer
	util.Format(&b, "Test: %s %d, %j")

	if res := b.String(); res != "Test: %s %d, %j" {
		t.Fatalf("Unexpected result: '%s'", res)
	}
}

func TestUtil_Format_LessArgs(t *testing.T) {
	vm := goja.New()
	util := utilpkg.New(vm)

	var b bytes.Buffer
	util.Format(&b, "Test: %s %d, %j", vm.ToValue("string"), vm.ToValue(42))

	if res := b.String(); res != "Test: string 42, %j" {
		t.Fatalf("Unexpected result: '%s'", res)
	}
}

func TestUtil_Format_MoreArgs(t *testing.T) {
	vm := goja.New()
	util := utilpkg.New(vm)

	var b bytes.Buffer
	util.Format(&b, "Test: %s %d, %j", vm.ToValue("string"), vm.ToValue(42), vm.NewObject(), vm.ToValue(42.42))

	if res := b.String(); res != "Test: string 42, {} 42.42" {
		t.Fatalf("Unexpected result: '%s'", res)
	}
}

func TestJSNoArgs(t *testing.T) {
	vm := goja.New()
	new(require.Registry).Enable(vm)

	if util, ok := require.Require(vm, utilpkg.ModuleName).(*goja.Object); ok {
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

func TestJSFormatNodeCompatibleValues(t *testing.T) {
	vm := goja.New()
	new(require.Registry).Enable(vm)
	require.Require(vm, utilpkg.ModuleName)

	tests := []struct {
		name string
		expr string
		want string
	}{
		{"object arguments", `require("util").format({score: 42}, {kind: "player"})`, `{ score: 42 } { kind: 'player' }`},
		{"extra object", `require("util").format("record=", {value: {score: 42}})`, `record= { value: { score: 42 } }`},
		{"undefined", `require("util").format(undefined)`, `undefined`},
		{"integer and float", `require("util").format("%i %f", "42.8", "3.5")`, `42 3.5`},
		{"symbol placeholders", `require("util").format("%s %d %i %f", Symbol("id"), Symbol("id"), Symbol("id"), Symbol("id"))`, `Symbol(id) NaN NaN NaN`},
		{"bigint placeholders", `require("util").format("%s %d %i %f", 42n, 42n, 42n, 42n)`, `42n 42n 42n 42`},
		{"object placeholders", `require("util").format("%s | %o | %O", {a: {b: 1}}, {a: 1}, {a: 1})`, `{ a: [Object] } | { a: 1 } | { a: 1 }`},
		{"placeholder depths", `require("util").format("%o | %O", {a:{b:{c:{d:{e:1}}}}}, {a:{b:{c:{d:1}}}})`, `{ a: { b: { c: { d: { e: 1 } } } } } | { a: { b: { c: [Object] } } }`},
		{"unknown placeholder", `require("util").format("%q", 1)`, `%q 1`},
		{"trailing percent", `require("util").format("value%")`, `value%`},
		{"circular JSON", `(() => { const o = {}; o.self = o; return require("util").format("%j", o); })()`, `[Circular]`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := vm.RunString(test.expr)
			if err != nil {
				t.Fatal(err)
			}
			if got := value.String(); got != test.want {
				t.Fatalf("format result = %q, want %q", got, test.want)
			}
		})
	}
}

func TestUtil_Inspect(t *testing.T) {
	vm := goja.New()
	new(require.Registry).Enable(vm)
	utilExports := require.Require(vm, utilpkg.ModuleName).(*goja.Object)
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
		{"nested object", `({ a: 1, b: { c: 2 } })`, `{ a: 1, b: { c: 2 } }`},
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
	utilExports := require.Require(vm, utilpkg.ModuleName).(*goja.Object)
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
	utilExports := require.Require(vm, utilpkg.ModuleName).(*goja.Object)
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
		if res.String() != `{ a: 1 }` {
			t.Fatalf("inspect(_, %s) = %q", opts, res.String())
		}
	}
}

func TestInspect_DepthOption(t *testing.T) {
	vm := goja.New()
	new(require.Registry).Enable(vm)
	utilExports := require.Require(vm, utilpkg.ModuleName).(*goja.Object)
	inspect, _ := goja.AssertFunction(utilExports.Get("inspect"))

	cases := []struct {
		name  string
		depth int
		want  string
	}{
		{"depth0", 0, `{ a: [Object] }`},
		{"depth1", 1, `{ a: { b: [Object] } }`},
		{"depthNegative", -1, `[Object]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			val, err := vm.RunString(`({a:{b:{c:1}}})`)
			if err != nil {
				t.Fatal(err)
			}
			res, err := inspect(utilExports, val, vm.ToValue(map[string]int{"depth": c.depth}))
			if err != nil {
				t.Fatal(err)
			}
			if res.String() != c.want {
				t.Fatalf("inspect(_, {depth:%d}): got %q want %q", c.depth, res.String(), c.want)
			}
		})
	}
}

func TestInspectNullDepthIsUnlimited(t *testing.T) {
	vm := goja.New()
	new(require.Registry).Enable(vm)
	utilExports := require.Require(vm, utilpkg.ModuleName).(*goja.Object)
	inspect, _ := goja.AssertFunction(utilExports.Get("inspect"))
	value, err := vm.RunString(`({a:{b:{c:{d:1}}}})`)
	if err != nil {
		t.Fatal(err)
	}
	options := vm.NewObject()
	if setErr := options.Set("depth", goja.Null()); setErr != nil {
		t.Fatal(setErr)
	}
	result, err := inspect(utilExports, value, options)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.String(), `{ a: { b: { c: { d: 1 } } } }`; got != want {
		t.Fatalf("inspect depth null = %q, want %q", got, want)
	}
}

func TestInspect_Functions(t *testing.T) {
	vm := goja.New()
	new(require.Registry).Enable(vm)
	utilExports := require.Require(vm, utilpkg.ModuleName).(*goja.Object)
	inspect, _ := goja.AssertFunction(utilExports.Get("inspect"))

	cases := []struct {
		name string
		js   string
		want string
	}{
		{"named", `(function foo(){})`, "[Function: foo]"},
		{"arrow", `(()=>{})`, "[Function (anonymous)]"},
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

func TestInspect_NumbersAndEmpty(t *testing.T) {
	vm := goja.New()
	new(require.Registry).Enable(vm)
	utilExports := require.Require(vm, utilpkg.ModuleName).(*goja.Object)
	inspect, _ := goja.AssertFunction(utilExports.Get("inspect"))

	cases := []struct {
		name string
		js   string
		want string
	}{
		{"int", `42`, "42"},
		{"float", `3.14`, "3.14"},
		{"empty array", `[]`, "[]"},
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

	res, err := inspect(utilExports)
	if err != nil {
		t.Fatal(err)
	}
	if res.String() != "undefined" {
		t.Fatalf("inspect(): got %q want undefined", res.String())
	}
}

func TestInspectBuiltInTypes(t *testing.T) {
	vm := goja.New()
	new(require.Registry).Enable(vm)
	utilExports := require.Require(vm, utilpkg.ModuleName).(*goja.Object)
	inspect, _ := goja.AssertFunction(utilExports.Get("inspect"))

	tests := []struct {
		name string
		expr string
		want string
	}{
		{"date", `new Date("2020-01-01T00:00:00.000Z")`, `2020-01-01T00:00:00.000Z`},
		{"regexp", `/x/gi`, `/x/gi`},
		{"map", `new Map([["a", 1], ["b", {c: 2}]])`, `Map(2) { 'a' => 1, 'b' => { c: 2 } }`},
		{"set", `new Set([1, "x"])`, `Set(2) { 1, 'x' }`},
		{"empty map", `new Map()`, `Map(0) {}`},
		{"empty set", `new Set()`, `Set(0) {}`},
		{"spoofed constructor", `({ constructor: Map, value: 1 })`, `{ constructor: [Function: Map], value: 1 }`},
		{"bigint", `42n`, `42n`},
		{"symbol", `Symbol("id")`, `Symbol(id)`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := vm.RunString(test.expr)
			if err != nil {
				t.Fatal(err)
			}
			result, err := inspect(utilExports, value)
			if err != nil {
				t.Fatal(err)
			}
			if got := result.String(); got != test.want {
				t.Fatalf("inspect result = %q, want %q", got, test.want)
			}
		})
	}
}

func TestInspectErrorIncludesNameMessageAndStack(t *testing.T) {
	vm := goja.New()
	new(require.Registry).Enable(vm)
	utilExports := require.Require(vm, utilpkg.ModuleName).(*goja.Object)
	inspect, _ := goja.AssertFunction(utilExports.Get("inspect"))
	value, err := vm.RunScript("error.js", `new TypeError("boom")`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := inspect(utilExports, value)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); !strings.Contains(got, "TypeError: boom") || !strings.Contains(got, "error.js:") {
		t.Fatalf("unexpected inspected error: %q", got)
	}
}
