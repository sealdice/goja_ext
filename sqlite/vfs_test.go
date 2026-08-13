package sqlite //nolint:testpackage

import (
	"os"
	"sync/atomic"
	"testing"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/vfs"
	"github.com/sealdice/goja_ext/fs"
	"github.com/spf13/afero"
)

type observingFs struct {
	afero.Fs
	openCount atomic.Int32
}

func (f *observingFs) OpenFile(name string, flag int, mode os.FileMode) (afero.File, error) {
	f.openCount.Add(1)
	return f.Fs.OpenFile(name, flag, mode)
}

func TestAferoVFSUsesWrappedOsFs(t *testing.T) {
	directory := t.TempDir()
	backend := &observingFs{Fs: afero.NewOsFs()}
	core, err := fs.NewCore(fs.WithFS(backend), fs.WithCwd(directory))
	if err != nil {
		t.Fatalf("NewCore() error = %v", err)
	}

	name := "goja_ext_observing_vfs"
	vfs.Register(name, newAferoVFS(core))
	defer vfs.Unregister(name)
	db, err := sqlite3.OpenFlags("file:"+directory+"/observed.db?vfs="+name, sqlite3.OPEN_READWRITE|sqlite3.OPEN_CREATE|sqlite3.OPEN_URI)
	if err != nil {
		t.Fatalf("OpenFlags() error = %v", err)
	}
	if err := db.Exec("CREATE TABLE t (value TEXT); INSERT INTO t VALUES ('observed')"); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := backend.openCount.Load(); got == 0 {
		t.Fatal("Afero wrapper did not observe any database opens")
	}
}

func TestAferoVFSOpensSQLite(t *testing.T) {
	backend := afero.NewMemMapFs()
	core, err := fs.NewCore(fs.WithFS(backend), fs.WithCwd("/workspace"))
	if err != nil {
		t.Fatalf("NewCore() error = %v", err)
	}

	name := "goja_ext_test_vfs"
	adapter := newAferoVFS(core)
	vfs.Register(name, adapter)
	defer vfs.Unregister(name)

	db, err := sqlite3.OpenFlags("file:/workspace/test.db?vfs="+name, sqlite3.OPEN_READWRITE|sqlite3.OPEN_CREATE|sqlite3.OPEN_URI)
	if err != nil {
		t.Fatalf("OpenFlags() error = %v", err)
	}
	defer db.Close()
	if err = db.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, value TEXT); INSERT INTO items(value) VALUES ('ok')"); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}

	stmt, _, err := db.Prepare("SELECT value FROM items")
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	defer stmt.Close()
	if !stmt.Step() {
		t.Fatalf("Step() returned false: %v", stmt.Err())
	}
	if got := stmt.ColumnText(0); got != "ok" {
		t.Fatalf("ColumnText() = %q, want %q", got, "ok")
	}
}

func TestAferoVFSPersistsThroughCoreBackend(t *testing.T) {
	backend := afero.NewMemMapFs()
	core, err := fs.NewCore(fs.WithFS(backend), fs.WithCwd("/workspace"))
	if err != nil {
		t.Fatalf("NewCore() error = %v", err)
	}

	adapter := newAferoVFS(core)
	file, _, err := adapter.Open("/workspace/data.db", vfs.OPEN_MAIN_DB|vfs.OPEN_READWRITE|vfs.OPEN_CREATE)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err = file.WriteAt([]byte("sqlite"), 0); err != nil {
		t.Fatalf("WriteAt() error = %v", err)
	}
	if err = file.Sync(vfs.SYNC_NORMAL); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if err = file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	data, err := afero.ReadFile(backend, "/workspace/data.db")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "sqlite" {
		t.Fatalf("backend data = %q, want %q", data, "sqlite")
	}
}
