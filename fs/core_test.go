package fs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestWithExtraCapabilitiesRejectsUnknownCapability(t *testing.T) {
	_, err := NewCore(WithExtraCapabilities(struct{ Name string }{Name: "unknown"}))
	if err == nil || !strings.Contains(err.Error(), "unsupported capability") {
		t.Fatalf("unknown capability error = %v", err)
	}
}

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

func TestOpenFlagCreateNew(t *testing.T) {
	core, err := NewCore(WithFS(afero.NewMemMapFs()), WithCwd("/"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := core.OpenFile("x", openWrite|openCreate|openTruncate, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("data")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = core.OpenFile("x", openWrite|openCreateNew, 0o644)
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected os.ErrExist, got %v", err)
	}
}

func TestOpenFlagAppend(t *testing.T) {
	core, err := NewCore(WithFS(afero.NewMemMapFs()), WithCwd("/"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := core.OpenFile("f", openWrite|openCreate|openTruncate, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("AB")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	a, err := core.OpenFile("f", openWrite|openAppend, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Write([]byte("C")); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := core.OpenFile("f", openRead, 0)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if string(data) != "ABC" {
		t.Fatalf("expected appended content ABC, got %q", data)
	}
}

func TestClosedHandleOperations(t *testing.T) {
	core, err := NewCore(WithFS(afero.NewMemMapFs()), WithCwd("/"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := core.OpenFile("f", openWrite|openCreate|openTruncate, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("data")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 4)
	if _, err := f.Read(buf); !errors.Is(err, ErrClosedHandle) {
		t.Fatalf("Read after close: expected ErrClosedHandle, got %v", err)
	}
	if _, err := f.Seek(0, 0); !errors.Is(err, ErrClosedHandle) {
		t.Fatalf("Seek after close: expected ErrClosedHandle, got %v", err)
	}
	if _, err := f.Stat(); !errors.Is(err, ErrClosedHandle) {
		t.Fatalf("Stat after close: expected ErrClosedHandle, got %v", err)
	}
	if err := f.Truncate(0); !errors.Is(err, ErrClosedHandle) {
		t.Fatalf("Truncate after close: expected ErrClosedHandle, got %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("second Close: expected nil, got %v", err)
	}
}
