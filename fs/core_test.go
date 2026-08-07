package fs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

func TestCoreMemMapFileLifecycle(t *testing.T) {
	backend := afero.NewMemMapFs()
	core, err := NewCore(WithFS(backend), WithCwd("/workspace"))
	if err != nil {
		t.Fatal(err)
	}

	if err := core.MkdirAll("docs", 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := core.OpenFile("docs/readme.txt", openWrite|openCreate|openTruncate, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	readFile, err := core.OpenFile("docs/readme.txt", openRead, 0)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(readFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("read data: %q", data)
	}
	if err := readFile.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCorePathAndClosedHandle(t *testing.T) {
	core, err := NewCore(WithFS(afero.NewMemMapFs()), WithCwd("/workspace"))
	if err != nil {
		t.Fatal(err)
	}
	if got := core.ResolvePath("a/../b"); got != "/workspace/b" {
		t.Fatalf("resolved path: %s", got)
	}
	file, err := core.OpenFile("file.txt", openWrite|openCreate, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("x")); err == nil {
		t.Fatal("write on closed handle succeeded")
	}
}

func TestCoreAferoBackendWrappers(t *testing.T) {
	base := afero.NewMemMapFs()
	if err := base.MkdirAll("/sandbox", 0o755); err != nil {
		t.Fatal(err)
	}
	core, err := NewCore(
		WithFS(afero.NewBasePathFs(base, "/sandbox")),
		WithCwd("/"),
	)
	if err != nil {
		t.Fatal(err)
	}
	file, err := core.OpenFile("nested.txt", openWrite|openCreate|openTruncate, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("inside")); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Stat("/sandbox/nested.txt"); err != nil {
		t.Fatalf("base path backend did not receive file: %v", err)
	}

	readonly, err := NewCore(
		WithFS(afero.NewReadOnlyFs(base)),
		WithCwd("/sandbox"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readonly.OpenFile("blocked.txt", openWrite|openCreate, 0o644); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("read-only backend error: %v", err)
	}
}

func TestCoreOsFsSmoke(t *testing.T) {
	dir := t.TempDir()
	core, err := NewCore(WithFS(afero.NewOsFs()), WithCwd(dir))
	if err != nil {
		t.Fatal(err)
	}
	file, err := core.OpenFile("host.txt", openWrite|openCreate|openTruncate, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("host")); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "host.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "host" {
		t.Fatalf("host file data: %q", data)
	}
}

func TestCoreWriteAllRejectsShortWrite(t *testing.T) {
	core, err := NewCore(
		WithFS(shortWriteFs{Fs: afero.NewMemMapFs()}),
		WithCwd("/workspace"),
	)
	if err != nil {
		t.Fatal(err)
	}
	file, err := core.OpenFile("short.txt", openWrite|openCreate|openTruncate, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.WriteAll([]byte("abc")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("WriteAll error: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

type shortWriteFs struct {
	afero.Fs
}

func (fsys shortWriteFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	file, err := fsys.Fs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return shortWriteFile{File: file}, nil
}

type shortWriteFile struct {
	afero.File
}

func (file shortWriteFile) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}
