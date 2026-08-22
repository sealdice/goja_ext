package util

import (
	"bytes"
	"math/big"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
)

const ModuleName = "util"

type Util struct {
	runtime *goja.Runtime
}

func (u *Util) format(f rune, val goja.Value, w *bytes.Buffer) bool {
	switch f {
	case 's':
		if isSymbol(val) || isBigInt(val) {
			w.WriteString(inspectValue(u.runtime, val, 0, newSeenSet()))
		} else if obj, ok := val.(*goja.Object); ok && !isFunction(obj) {
			w.WriteString(inspectValue(u.runtime, val, 0, newSeenSet()))
		} else {
			w.WriteString(val.String())
		}
	case 'd':
		if isSymbol(val) {
			w.WriteString("NaN")
		} else if isBigInt(val) {
			w.WriteString(inspectValue(u.runtime, val, 0, newSeenSet()))
		} else {
			w.WriteString(val.ToNumber().String())
		}
	case 'i':
		if isSymbol(val) {
			w.WriteString("NaN")
		} else if isBigInt(val) {
			w.WriteString(inspectValue(u.runtime, val, 0, newSeenSet()))
		} else {
			w.WriteString(u.callNumberParser("parseInt", val))
		}
	case 'f':
		if isSymbol(val) {
			w.WriteString("NaN")
		} else {
			w.WriteString(u.callNumberParser("parseFloat", val))
		}
	case 'j':
		if json, ok := u.runtime.Get("JSON").(*goja.Object); ok {
			if stringify, ok := goja.AssertFunction(json.Get("stringify")); ok {
				res, err := stringify(json, val)
				if err != nil && strings.Contains(strings.ToLower(err.Error()), "circular") {
					w.WriteString("[Circular]")
				} else if err != nil {
					panic(err)
				}
				if err == nil {
					w.WriteString(res.String())
				}
			}
		}
	case 'o':
		w.WriteString(inspectValue(u.runtime, val, 4, newSeenSet()))
	case 'O':
		w.WriteString(inspectValue(u.runtime, val, defaultDepth, newSeenSet()))
	case '%':
		w.WriteByte('%')
		return false
	default:
		w.WriteByte('%')
		w.WriteRune(f)
		return false
	}
	return true
}

func isSymbol(value goja.Value) bool {
	_, ok := value.(*goja.Symbol)
	return ok
}

func isBigInt(value goja.Value) bool {
	_, ok := value.Export().(*big.Int)
	return ok
}

func (u *Util) callNumberParser(name string, val goja.Value) string {
	fn, ok := goja.AssertFunction(u.runtime.Get(name))
	if !ok {
		return "NaN"
	}
	result, err := fn(goja.Undefined(), val)
	if err != nil {
		panic(err)
	}
	return result.String()
}

func (u *Util) Format(b *bytes.Buffer, f string, args ...goja.Value) {
	pct := false
	argNum := 0
	for _, chr := range f {
		if pct {
			if argNum < len(args) {
				if u.format(chr, args[argNum], b) {
					argNum++
				}
			} else {
				b.WriteByte('%')
				b.WriteRune(chr)
			}
			pct = false
		} else {
			if chr == '%' {
				pct = true
			} else {
				b.WriteRune(chr)
			}
		}
	}
	if pct {
		b.WriteByte('%')
	}

	for _, arg := range args[argNum:] {
		b.WriteByte(' ')
		u.writeExtraArgument(b, arg)
	}
}

func (u *Util) writeExtraArgument(b *bytes.Buffer, arg goja.Value) {
	if _, ok := arg.Export().(string); ok {
		b.WriteString(arg.String())
		return
	}
	b.WriteString(inspectValue(u.runtime, arg, defaultDepth, newSeenSet()))
}

func (u *Util) js_format(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		return u.runtime.ToValue("")
	}

	if _, ok := call.Argument(0).Export().(string); !ok {
		var b bytes.Buffer
		for i, arg := range call.Arguments {
			if i > 0 {
				b.WriteByte(' ')
			}
			u.writeExtraArgument(&b, arg)
		}
		return u.runtime.ToValue(b.String())
	}

	var b bytes.Buffer
	fmt := call.Argument(0).String()

	var args []goja.Value
	if len(call.Arguments) > 0 {
		args = call.Arguments[1:]
	}
	u.Format(&b, fmt, args...)

	return u.runtime.ToValue(b.String())
}

func Require(runtime *goja.Runtime, module *goja.Object) {
	u := &Util{
		runtime: runtime,
	}
	obj := module.Get("exports").(*goja.Object)
	if err := obj.Set("format", u.js_format); err != nil {
		panic(err)
	}
	if err := obj.Set("inspect", Inspect(runtime)); err != nil {
		panic(err)
	}
}

func New(runtime *goja.Runtime) *Util {
	return &Util{
		runtime: runtime,
	}
}

func init() { //nolint:gochecknoinits // auto-register core module via blank import
	require.RegisterCoreModule(ModuleName, Require)
}
