package process_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/process"
	"github.com/sealdice/goja_ext/require"
)

func TestProcessEnvStructure(t *testing.T) {
	vm := goja.New()

	new(require.Registry).Enable(vm)
	process.Enable(vm)

	if c := vm.Get("process"); c == nil {
		t.Fatal("process not found")
	}

	if c, err := vm.RunString("process.env"); c == nil || err != nil {
		t.Fatal("error accessing process.env")
	}
}

func TestProcessEnvValuesArtificial(t *testing.T) {
	t.Setenv("GOJA_IS_AWESOME", "true")

	vm := goja.New()

	new(require.Registry).Enable(vm)
	process.Enable(vm)

	jsRes, err := vm.RunString("process.env['GOJA_IS_AWESOME']")

	if err != nil {
		t.Fatalf("Error executing: %s", err)
	}

	if jsRes.String() != "true" {
		t.Fatalf("Error executing: got %s but expected %s", jsRes, "true")
	}
}

func TestProcessEnvValuesBrackets(t *testing.T) {
	vm := goja.New()

	new(require.Registry).Enable(vm)
	process.Enable(vm)

	for _, e := range os.Environ() {
		envKeyValue := strings.SplitN(e, "=", 2)
		jsExpr := fmt.Sprintf("process.env['%s']", envKeyValue[0])

		jsRes, err := vm.RunString(jsExpr)

		if err != nil {
			t.Fatalf("Error executing %s: %s", jsExpr, err)
		}

		if jsRes.String() != envKeyValue[1] {
			t.Fatalf("Error executing %s: got %s but expected %s", jsExpr, jsRes, envKeyValue[1])
		}
	}
}

func TestProcessCwdIsRuntimeLocal(t *testing.T) {
	hostCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	vm := goja.New()
	new(require.Registry).Enable(vm)
	process.Enable(vm)
	if err = vm.Set("target", target); err != nil {
		t.Fatal(err)
	}
	value, err := vm.RunString(`
		const same = require("process") === process;
		process.chdir(target);
		JSON.stringify([same, process.cwd()]);
	`)
	if err != nil {
		t.Fatal(err)
	}
	want := `[true,"` + filepath.ToSlash(target) + `"]`
	if got := value.String(); got != want {
		t.Fatalf("runtime cwd result = %s, want %s", got, want)
	}
	if current, err := os.Getwd(); err != nil || current != hostCwd {
		t.Fatalf("host cwd changed to %q (err=%v), want %q", current, err, hostCwd)
	}
}
