package console

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
	"github.com/sealdice/goja_ext/util"
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
		o.Set("log", c.log(c.printer.Log))
		o.Set("error", c.log(c.printer.Error))
		o.Set("warn", c.log(c.printer.Warn))
		o.Set("info", c.log(c.printer.Log))
		o.Set("debug", c.log(c.printer.Log))
		o.Set("trace", c.trace)
	}
}

func Enable(runtime *goja.Runtime) {
	runtime.Set("console", require.Require(runtime, ModuleName))
}

func init() {
	require.RegisterCoreModule(ModuleName, Require)
}
