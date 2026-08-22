package console_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/console"
	"github.com/dop251/goja_nodejs/require"
)

func TestConsole(t *testing.T) {
	vm := goja.New()

	new(require.Registry).Enable(vm)
	console.Enable(vm)

	if c := vm.Get("console"); c == nil {
		t.Fatal("console not found")
	}

	if _, err := vm.RunString("console.log('')"); err != nil {
		t.Fatal("console.log() error", err)
	}

	if _, err := vm.RunString("console.error('')"); err != nil {
		t.Fatal("console.error() error", err)
	}

	if _, err := vm.RunString("console.warn('')"); err != nil {
		t.Fatal("console.warn() error", err)
	}

	if _, err := vm.RunString("console.info('')"); err != nil {
		t.Fatal("console.info() error", err)
	}

	if _, err := vm.RunString("console.debug('')"); err != nil {
		t.Fatal("console.debug() error", err)
	}
}

type capturedPrinter struct {
	stdout []string
	stderr []string
}

func (p *capturedPrinter) Log(s string)   { p.stdout = append(p.stdout, s) }
func (p *capturedPrinter) Warn(s string)  { p.stderr = append(p.stderr, s) }
func (p *capturedPrinter) Error(s string) { p.stderr = append(p.stderr, s) }

func newConsoleRuntime(t *testing.T, printer console.Printer) *goja.Runtime {
	t.Helper()
	vm := goja.New()
	registry := new(require.Registry)
	registry.Enable(vm)
	registry.RegisterNativeModule(console.ModuleName, console.RequireWithPrinter(printer))
	console.Enable(vm)
	return vm
}

func newConsoleRuntimeWithConfig(t *testing.T, cfg console.Config) *goja.Runtime {
	t.Helper()
	vm := goja.New()
	registry := new(require.Registry)
	registry.Enable(vm)
	registry.RegisterNativeModule(console.ModuleName, console.RequireWithConfig(cfg))
	console.Enable(vm)
	return vm
}

func TestConsoleFormatsObjectsLikeNode(t *testing.T) {
	printer := new(capturedPrinter)
	vm := newConsoleRuntime(t, printer)
	if _, err := vm.RunString(`console.log({ value: { score: 42 } }, [{ name: "user:1000" }])`); err != nil {
		t.Fatal(err)
	}
	if got, want := printer.stdout, []string{"{ value: { score: 42 } } [ { name: 'user:1000' } ]"}; !equalStrings(got, want) {
		t.Fatalf("stdout = %#v, want %#v", got, want)
	}
}

func TestConsoleAssertAndDir(t *testing.T) {
	printer := new(capturedPrinter)
	vm := newConsoleRuntime(t, printer)
	_, err := vm.RunString(`
		console.assert(true, "ignored");
		console.assert(false);
		console.assert(false, "score=%d", 42);
		console.dir({ value: { score: 42 } }, { depth: 0 });
		console.dirxml({ kind: "player" });
	`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"{ value: [Object] }", "{ kind: 'player' }"}; !equalStrings(printer.stdout, want) {
		t.Fatalf("stdout = %#v, want %#v", printer.stdout, want)
	}
	if want := []string{"Assertion failed", "Assertion failed: score=42"}; !equalStrings(printer.stderr, want) {
		t.Fatalf("stderr = %#v, want %#v", printer.stderr, want)
	}
}

func TestConsoleCountAndGroups(t *testing.T) {
	printer := new(capturedPrinter)
	vm := newConsoleRuntime(t, printer)
	_, err := vm.RunString(`
		console.count(); console.count(); console.count("jobs");
		console.countReset(); console.count();
		console.group("outer", { id: 1 });
		console.log("inside");
		console.groupCollapsed("inner");
		console.warn({ warning: true });
		console.groupEnd(); console.groupEnd(); console.groupEnd();
		console.log("outside");
		console.clear();
	`)
	if err != nil {
		t.Fatal(err)
	}
	wantOut := []string{
		"default: 1", "default: 2", "jobs: 1", "default: 1",
		"outer { id: 1 }", "  inside", "  inner", "outside",
	}
	if !equalStrings(printer.stdout, wantOut) {
		t.Fatalf("stdout = %#v, want %#v", printer.stdout, wantOut)
	}
	if want := []string{"    { warning: true }"}; !equalStrings(printer.stderr, want) {
		t.Fatalf("stderr = %#v, want %#v", printer.stderr, want)
	}
}

func TestConsoleTimers(t *testing.T) {
	printer := new(capturedPrinter)
	vm := newConsoleRuntime(t, printer)
	_, err := vm.RunString(`
		console.time("load");
		console.timeLog("load", { phase: 1 });
		console.timeEnd("load");
		console.timeEnd("missing");
	`)
	if err != nil {
		t.Fatal(err)
	}
	if len(printer.stdout) != 2 {
		t.Fatalf("stdout = %#v", printer.stdout)
	}
	if !regexp.MustCompile(`^load: [0-9.]+ms \{ phase: 1 \}$`).MatchString(printer.stdout[0]) {
		t.Fatalf("unexpected timeLog output %q", printer.stdout[0])
	}
	if !regexp.MustCompile(`^load: [0-9.]+ms$`).MatchString(printer.stdout[1]) {
		t.Fatalf("unexpected timeEnd output %q", printer.stdout[1])
	}
	if want := []string{"No such label 'missing' for console.timeEnd()"}; !equalStrings(printer.stderr, want) {
		t.Fatalf("stderr = %#v", printer.stderr)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestConsoleWithPrinter(t *testing.T) {
	var stdoutStr, stderrStr string

	printer := console.StdPrinter{
		StdoutPrint: func(s string) { stdoutStr += s },
		StderrPrint: func(s string) { stderrStr += s },
	}

	vm := goja.New()

	registry := new(require.Registry)
	registry.Enable(vm)
	registry.RegisterNativeModule(console.ModuleName, console.RequireWithPrinter(printer))
	console.Enable(vm)

	if c := vm.Get("console"); c == nil {
		t.Fatal("console not found")
	}

	_, err := vm.RunString(`
		console.log('a')
		console.error('b')
		console.warn('c')
		console.debug('d')
		console.info('e')
	`)
	if err != nil {
		t.Fatal(err)
	}

	if want := "ade"; stdoutStr != want {
		t.Fatalf("Unexpected stdout output: got %q, want %q", stdoutStr, want)
	}

	if want := "bc"; stderrStr != want {
		t.Fatalf("Unexpected stderr output: got %q, want %q", stderrStr, want)
	}
}

func TestConsoleTraceIncludesFormattedMessageAndStack(t *testing.T) {
	var stderr string
	printer := console.StdPrinter{
		StdoutPrint: func(string) {},
		StderrPrint: func(s string) { stderr += s },
	}

	vm := goja.New()
	registry := new(require.Registry)
	registry.Enable(vm)
	registry.RegisterNativeModule(console.ModuleName, console.RequireWithPrinter(printer))
	console.Enable(vm)

	_, err := vm.RunScript("trace_test.js", `
		function caller() { console.trace("value=%d", 2); }
		caller();
	`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "Trace: value=2") {
		t.Fatalf("trace message missing from %q", stderr)
	}
	if !strings.Contains(stderr, "at caller (trace_test.js:") {
		t.Fatalf("caller stack frame missing from %q", stderr)
	}
}

func TestConsoleTagPrefixesOutput(t *testing.T) {
	printer := new(capturedPrinter)
	vm := newConsoleRuntimeWithConfig(t, console.Config{Printer: printer, Tag: console.ModuleTag})

	_, err := vm.RunScript("plugin.js", `
		function helper(msg) { console.log(msg); }
		helper("hello");
	`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"[plugin.js] hello"}; !equalStrings(printer.stdout, want) {
		t.Fatalf("stdout = %#v, want %#v", printer.stdout, want)
	}
}

func TestConsoleWithoutTagStaysUnprefixed(t *testing.T) {
	printer := new(capturedPrinter)
	vm := newConsoleRuntimeWithConfig(t, console.Config{Printer: printer})

	_, err := vm.RunScript("plugin.js", `
		console.log("hello");
	`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"hello"}; !equalStrings(printer.stdout, want) {
		t.Fatalf("stdout = %#v, want %#v", printer.stdout, want)
	}
}

func TestConsoleFilterDropsByMethod(t *testing.T) {
	printer := new(capturedPrinter)
	vm := newConsoleRuntimeWithConfig(t, console.Config{
		Printer: printer,
		Tag:     console.ModuleTag,
		Filter:  func(e console.Entry) bool { return e.Method != "log" },
	})

	_, err := vm.RunScript("plugin.js", `
		console.log("dropped");
		console.warn("kept");
	`)
	if err != nil {
		t.Fatal(err)
	}
	if len(printer.stdout) != 0 {
		t.Fatalf("log lines should be dropped, stdout = %#v", printer.stdout)
	}
	if want := []string{"[plugin.js] kept"}; !equalStrings(printer.stderr, want) {
		t.Fatalf("stderr = %#v, want %#v", printer.stderr, want)
	}
}

func TestConsoleTagAttributesInnermostModule(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib.js")
	main := filepath.Join(dir, "main.js")
	if err := os.WriteFile(lib, []byte(`module.exports = function(){ console.log("from lib"); }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(main, []byte(`require("./lib")()`), 0o644); err != nil {
		t.Fatal(err)
	}

	printer := new(capturedPrinter)
	vm := newConsoleRuntimeWithConfig(t, console.Config{Printer: printer, Tag: console.ModuleTag})

	if _, err := vm.RunString("require('" + main + "')"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"[lib.js] from lib"}; !equalStrings(printer.stdout, want) {
		t.Fatalf("stdout = %#v, want %#v", printer.stdout, want)
	}
}
