package console

import (
	"fmt"
	"path"
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
	tag     func(*goja.Runtime) string
	filter  func(Entry) bool
	counts  map[string]int
	timers  map[string]time.Time
	groups  int
}

type Printer interface {
	Log(string)
	Warn(string)
	Error(string)
}

// ModuleTag labels a console line with the basename of the innermost script
// frame that directly issued it (the file whose code called console.log/warn/
// error). It returns "" when no script frame is on the stack, e.g. when the
// line is emitted directly from native host code.
func ModuleTag(runtime *goja.Runtime) string {
	for _, frame := range runtime.CaptureCallStack(0, nil) {
		if name := frame.SrcName(); name != "" && name != "<native>" {
			return path.Base(name)
		}
	}
	return ""
}

// Entry describes a single console log line before it is emitted. It carries
// the resolved source tag and the console method so hosts can route or filter
// output per source.
type Entry struct {
	Tag     string
	Method  string
	Message string
}

// Config configures a tagged, filterable console. The zero value is valid and
// behaves like the default console: messages are printed via default stdout
// without any prefix or filtering.
type Config struct {
	// Printer receives formatted messages. If nil, default stdout/stderr is used.
	Printer Printer
	// Tag resolves the source label for the current log line, or returns ""
	// to leave the line untagged. If nil, no stack capture is performed and
	// messages are passed through unchanged. ModuleTag is a convenient
	// implementation that labels a line with the basename of the innermost
	// script file that issued it.
	Tag func(runtime *goja.Runtime) string
	// Filter drops a line before it is printed when it returns false. It is
	// only invoked for lines that resolved a non-empty tag. Nil means emit all.
	Filter func(Entry) bool
}

func (c *Console) log(method string, p func(string)) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		c.print(method, p, c.format(call.Arguments))

		return goja.Undefined()
	}
}

func (c *Console) print(method string, p func(string), message string) {
	indent := strings.Repeat("  ", c.groups)
	message = indent + strings.ReplaceAll(message, "\n", "\n"+indent)

	if c.tag != nil {
		if tag := c.tag(c.runtime); tag != "" {
			if c.filter != nil && !c.filter(Entry{Tag: tag, Method: method, Message: message}) {
				return
			}
			message = "[" + tag + "] " + message
		}
	}
	p(message)
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
	c.print("trace", c.printer.Error, trace.String())
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
	c.print("assert", c.printer.Error, message)
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
	c.print("dir", c.printer.Log, result.String())
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
	c.print("count", c.printer.Log, fmt.Sprintf("%s: %d", name, c.counts[name]))
	return goja.Undefined()
}

func (c *Console) countReset(call goja.FunctionCall) goja.Value {
	name := label(call)
	if _, exists := c.counts[name]; !exists {
		c.print("countReset", c.printer.Warn, fmt.Sprintf("Count for '%s' does not exist", name))
		return goja.Undefined()
	}
	delete(c.counts, name)
	return goja.Undefined()
}

func (c *Console) time(call goja.FunctionCall) goja.Value {
	name := label(call)
	if _, exists := c.timers[name]; exists {
		c.print("time", c.printer.Warn, fmt.Sprintf("Label '%s' already exists for console.time()", name))
		return goja.Undefined()
	}
	c.timers[name] = time.Now()
	return goja.Undefined()
}

func (c *Console) timerMessage(method, name string, arguments []goja.Value) (string, bool) {
	started, exists := c.timers[name]
	if !exists {
		c.print(method, c.printer.Warn, fmt.Sprintf("No such label '%s' for console.%s()", name, method))
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
		c.print("timeLog", c.printer.Log, message)
	}
	return goja.Undefined()
}

func (c *Console) timeEnd(call goja.FunctionCall) goja.Value {
	name := label(call)
	if message, ok := c.timerMessage("timeEnd", name, nil); ok {
		delete(c.timers, name)
		c.print("timeEnd", c.printer.Log, message)
	}
	return goja.Undefined()
}

func (c *Console) group(call goja.FunctionCall) goja.Value {
	c.print("group", c.printer.Log, c.format(call.Arguments))
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
	requireWithConfig(Config{})(runtime, module)
}

func RequireWithPrinter(printer Printer) require.ModuleLoader {
	return requireWithConfig(Config{Printer: printer})
}

// RequireWithConfig returns a console module loader wired to the given Config.
// Register it as a native module (see require.Registry.RegisterNativeModule)
// before enabling console so it takes precedence over the default core module.
func RequireWithConfig(cfg Config) require.ModuleLoader {
	return requireWithConfig(cfg)
}

func requireWithConfig(cfg Config) require.ModuleLoader {
	return func(runtime *goja.Runtime, module *goja.Object) {
		printer := cfg.Printer
		if printer == nil {
			printer = defaultStdPrinter
		}
		c := &Console{
			runtime: runtime,
			printer: printer,
			tag:     cfg.Tag,
			filter:  cfg.Filter,
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
		set("log", c.log("log", c.printer.Log))
		set("error", c.log("error", c.printer.Error))
		set("warn", c.log("warn", c.printer.Warn))
		set("info", c.log("info", c.printer.Log))
		set("debug", c.log("debug", c.printer.Log))
		set("trace", c.trace)
		set("assert", c.assert)
		set("dir", c.dir)
		set("dirxml", c.log("dirxml", c.printer.Log))
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
