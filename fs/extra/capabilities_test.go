package extra_test

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	rootfs "github.com/sealdice/goja_ext/fs"
	"github.com/sealdice/goja_ext/fs/extra"
	"github.com/spf13/afero"
)

func TestAferoLinkCapabilitiesUseRealSymlinkOperations(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.txt")
	link := filepath.Join(directory, "link.txt")
	created := filepath.Join(directory, "created.txt")
	if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.txt", link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	backend := afero.NewOsFs()
	core, err := rootfs.NewCore(
		rootfs.WithFS(backend),
		rootfs.WithCwd(directory),
		rootfs.WithExtraCapabilities(extra.FromAfero(backend)...),
	)
	if err != nil {
		t.Fatal(err)
	}
	info, err := core.Lstat("link.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("lstat mode = %v", info.Mode())
	}
	resolved, err := core.Realpath("link.txt")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != target {
		t.Fatalf("realpath = %q, want %q", resolved, target)
	}
	read, err := core.Readlink("link.txt")
	if err != nil {
		t.Fatal(err)
	}
	if read != "target.txt" {
		t.Fatalf("readlink = %q", read)
	}
	if err := core.Symlink("target.txt", "created.txt"); err != nil {
		t.Fatal(err)
	}
	if createdInfo, err := os.Lstat(created); err != nil || createdInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("created symlink info=%v err=%v", createdInfo, err)
	}
}

func TestAferoMemMapDoesNotPretendToSupportLstatOrRealpath(t *testing.T) {
	backend := afero.NewMemMapFs()
	if err := afero.WriteFile(backend, "/value.txt", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	core, err := rootfs.NewCore(
		rootfs.WithFS(backend),
		rootfs.WithCwd("/"),
		rootfs.WithExtraCapabilities(extra.FromAfero(backend)...),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.Lstat("value.txt"); !errors.Is(err, syscall.ENOSYS) {
		t.Fatalf("lstat error = %v", err)
	}
	if _, err := core.Realpath("value.txt"); !errors.Is(err, syscall.ENOSYS) {
		t.Fatalf("realpath error = %v", err)
	}
}

func TestRealpathDetectsSymlinkLoop(t *testing.T) {
	directory := t.TempDir()
	if err := os.Symlink("b", filepath.Join(directory, "a")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink("a", filepath.Join(directory, "b")); err != nil {
		t.Fatal(err)
	}
	backend := afero.NewOsFs()
	core, err := rootfs.NewCore(
		rootfs.WithFS(backend),
		rootfs.WithCwd(directory),
		rootfs.WithExtraCapabilities(extra.FromAfero(backend)...),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.Realpath("a"); !errors.Is(err, syscall.ELOOP) {
		t.Fatalf("realpath loop error = %v", err)
	}
}
