package console

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	"github.com/dop251/goja_nodejs/util"
)

const ModuleName = "console"

type Console struct {
	runtime *goja.Runtime
	util    *goja.Object
	printer Printer
}

type Printer interface {
	Log(string)
	Warn(string)
	Error(string)
}

func (c *Console) log(p func(string)) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		p(c.format(call.Arguments))

		return nil
	}
}

func (c *Console) format(arguments []goja.Value) string {
	format, ok := goja.AssertFunction(c.util.Get("format"))
	if !ok {
		panic(c.runtime.NewTypeError("util.format is not a function"))
	}
	ret, err := format(c.util, arguments...)
	if err != nil {
		panic(err)
	}
	return ret.String()
}

func (c *Console) trace(call goja.FunctionCall) goja.Value {
	var trace strings.Builder
	trace.WriteString("Trace")
	if message := c.format(call.Arguments); message != "" {
		trace.WriteString(": ")
		trace.WriteString(message)
	}
	for _, frame := range c.runtime.CaptureCallStack(0, nil) {
		position := frame.Position()
		if position.Filename == "" {
			continue
		}
		fmt.Fprintf(
			&trace,
			"\n    at %s (%s:%d:%d)",
			frame.FuncName(),
			position.Filename,
			position.Line,
			position.Column,
		)
	}
	c.printer.Error(trace.String())
	return goja.Undefined()
}

func Require(runtime *goja.Runtime, module *goja.Object) {
	requireWithPrinter(defaultStdPrinter)(runtime, module)
}

func RequireWithPrinter(printer Printer) require.ModuleLoader {
	return requireWithPrinter(printer)
}

func requireWithPrinter(printer Printer) require.ModuleLoader {
	return func(runtime *goja.Runtime, module *goja.Object) {
		c := &Console{
			runtime: runtime,
			printer: printer,
		}

		c.util = require.Require(runtime, util.ModuleName).(*goja.Object)

		o := module.Get("exports").(*goja.Object)
		set := func(name string, value interface{}) {
			if err := o.Set(name, value); err != nil {
				panic(err)
			}
		}
		set("log", c.log(c.printer.Log))
		set("error", c.log(c.printer.Error))
		set("warn", c.log(c.printer.Warn))
		set("info", c.log(c.printer.Log))
		set("debug", c.log(c.printer.Log))
		set("trace", c.trace)
	}
}

func Enable(runtime *goja.Runtime) {
	if err := runtime.Set("console", require.Require(runtime, ModuleName)); err != nil {
		panic(err)
	}
}

func init() { //nolint:gochecknoinits // auto-register core module via blank import
	require.RegisterCoreModule(ModuleName, Require)
}
