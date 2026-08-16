package console

import (
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	"github.com/dop251/goja_nodejs/util"
)

const ModuleName = "console"

type Console struct {
	runtime *goja.Runtime
	util    *goja.Object
	printer Printer
	counts  map[string]int
	timers  map[string]time.Time
	groups  int
}

type Printer interface {
	Log(string)
	Warn(string)
	Error(string)
}

func (c *Console) log(p func(string)) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		c.print(p, c.format(call.Arguments))

		return goja.Undefined()
	}
}

func (c *Console) print(p func(string), message string) {
	indent := strings.Repeat("  ", c.groups)
	p(indent + strings.ReplaceAll(message, "\n", "\n"+indent))
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
	c.print(c.printer.Error, trace.String())
	return goja.Undefined()
}

func (c *Console) assert(call goja.FunctionCall) goja.Value {
	if call.Argument(0).ToBoolean() {
		return goja.Undefined()
	}
	message := "Assertion failed"
	if len(call.Arguments) > 1 {
		message += ": " + c.format(call.Arguments[1:])
	}
	c.print(c.printer.Error, message)
	return goja.Undefined()
}

func (c *Console) dir(call goja.FunctionCall) goja.Value {
	inspect, ok := goja.AssertFunction(c.util.Get("inspect"))
	if !ok {
		panic(c.runtime.NewTypeError("util.inspect is not a function"))
	}
	result, err := inspect(c.util, call.Argument(0), call.Argument(1))
	if err != nil {
		panic(err)
	}
	c.print(c.printer.Log, result.String())
	return goja.Undefined()
}

func label(call goja.FunctionCall) string {
	value := call.Argument(0)
	if goja.IsUndefined(value) {
		return "default"
	}
	return value.String()
}

func (c *Console) count(call goja.FunctionCall) goja.Value {
	name := label(call)
	c.counts[name]++
	c.print(c.printer.Log, fmt.Sprintf("%s: %d", name, c.counts[name]))
	return goja.Undefined()
}

func (c *Console) countReset(call goja.FunctionCall) goja.Value {
	name := label(call)
	if _, exists := c.counts[name]; !exists {
		c.print(c.printer.Warn, fmt.Sprintf("Count for '%s' does not exist", name))
		return goja.Undefined()
	}
	delete(c.counts, name)
	return goja.Undefined()
}

func (c *Console) time(call goja.FunctionCall) goja.Value {
	name := label(call)
	if _, exists := c.timers[name]; exists {
		c.print(c.printer.Warn, fmt.Sprintf("Label '%s' already exists for console.time()", name))
		return goja.Undefined()
	}
	c.timers[name] = time.Now()
	return goja.Undefined()
}

func (c *Console) timerMessage(method, name string, arguments []goja.Value) (string, bool) {
	started, exists := c.timers[name]
	if !exists {
		c.print(c.printer.Warn, fmt.Sprintf("No such label '%s' for console.%s()", name, method))
		return "", false
	}
	message := fmt.Sprintf("%s: %.3fms", name, float64(time.Since(started))/float64(time.Millisecond))
	if len(arguments) > 0 {
		message += " " + c.format(arguments)
	}
	return message, true
}

func (c *Console) timeLog(call goja.FunctionCall) goja.Value {
	name := label(call)
	if message, ok := c.timerMessage("timeLog", name, call.Arguments[1:]); ok {
		c.print(c.printer.Log, message)
	}
	return goja.Undefined()
}

func (c *Console) timeEnd(call goja.FunctionCall) goja.Value {
	name := label(call)
	if message, ok := c.timerMessage("timeEnd", name, nil); ok {
		delete(c.timers, name)
		c.print(c.printer.Log, message)
	}
	return goja.Undefined()
}

func (c *Console) group(call goja.FunctionCall) goja.Value {
	c.print(c.printer.Log, c.format(call.Arguments))
	c.groups++
	return goja.Undefined()
}

func (c *Console) groupEnd(goja.FunctionCall) goja.Value {
	if c.groups > 0 {
		c.groups--
	}
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
			counts:  make(map[string]int),
			timers:  make(map[string]time.Time),
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
		set("assert", c.assert)
		set("dir", c.dir)
		set("dirxml", c.log(c.printer.Log))
		set("count", c.count)
		set("countReset", c.countReset)
		set("time", c.time)
		set("timeLog", c.timeLog)
		set("timeEnd", c.timeEnd)
		set("group", c.group)
		set("groupCollapsed", c.group)
		set("groupEnd", c.groupEnd)
		set("clear", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
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
