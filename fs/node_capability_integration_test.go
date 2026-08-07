package fs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dop251/goja"
	rootfs "github.com/sealdice/goja_ext/fs"
	"github.com/sealdice/goja_ext/fs/extra"
	"github.com/sealdice/goja_ext/require"
	"github.com/spf13/afero"
)

func TestNodeFacadeUsesInjectedLinkCapabilities(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "target.txt"), []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.txt", filepath.Join(directory, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	backend := afero.NewOsFs()
	registry := require.NewRegistry()
	rootfs.RegisterWithOptions(registry,
		rootfs.WithFS(backend),
		rootfs.WithCwd(directory),
		rootfs.WithExtraCapabilities(extra.FromAfero(backend)...),
	)
	rt := goja.New()
	registry.Enable(rt)
	value, err := rt.RunString(`
		const fs = require("fs");
		const stat = fs.lstatSync("link.txt");
		const target = fs.readlinkSync("link.txt");
		const resolved = fs.realpathSync("link.txt");
		fs.symlinkSync("target.txt", "created.txt");
		[
			stat.isSymbolicLink(),
			target,
			resolved,
			fs.lstatSync("created.txt").isSymbolicLink()
		].join("|");
	`)
	if err != nil {
		t.Fatal(err)
	}
	want := "true|target.txt|" + filepath.Join(directory, "target.txt") + "|true"
	if got := value.String(); got != want {
		t.Fatalf("capability result = %q, want %q", got, want)
	}
}
