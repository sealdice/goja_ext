package fs_test

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/fs"
	"github.com/spf13/afero"
)

func TestEnsureCoreReusesRuntimeCore(t *testing.T) {
	rt := goja.New()
	backend := afero.NewMemMapFs()

	first, err := fs.EnsureCore(rt, fs.WithFS(backend), fs.WithCwd("/workspace"))
	if err != nil {
		t.Fatalf("EnsureCore() error = %v", err)
	}
	second, err := fs.EnsureCore(rt, fs.WithFS(backend), fs.WithCwd("/workspace"))
	if err != nil {
		t.Fatalf("EnsureCore() second call error = %v", err)
	}
	if first != second {
		t.Fatal("EnsureCore() returned different Core instances")
	}
	if first.Backend() != backend {
		t.Fatal("EnsureCore() returned a Core with a different backend")
	}
}
