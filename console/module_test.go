package console_test

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/console"
	"github.com/sealdice/goja_ext/require"
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
