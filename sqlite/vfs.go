package sqlite

import (
	"errors"
	"io"
	"os"
	"sync"

	hostfs "github.com/dop251/goja_nodejs/fs"
	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/util/vfsutil"
	"github.com/ncruces/go-sqlite3/vfs"
	"github.com/spf13/afero"
)

const sqliteFileMode = 0o666

type aferoVFS struct {
	core  *hostfs.Core
	locks lockManager
}

func newAferoVFS(core *hostfs.Core) *aferoVFS {
	return &aferoVFS{core: core}
}

func (v *aferoVFS) Open(name string, flags vfs.OpenFlag) (vfs.File, vfs.OpenFlag, error) {
	return v.open(v.core.ResolvePath(name), flags)
}

func (v *aferoVFS) OpenFilename(name *vfs.Filename, flags vfs.OpenFlag) (vfs.File, vfs.OpenFlag, error) {
	if name == nil {
		return v.open("", flags)
	}
	return v.open(v.core.ResolvePath(name.String()), flags)
}

func (v *aferoVFS) open(name string, flags vfs.OpenFlag) (vfs.File, vfs.OpenFlag, error) {
	if flags&vfs.OPEN_WAL != 0 {
		return nil, flags, sqlite3.CANTOPEN
	}

	if flags&(vfs.OPEN_TEMP_DB|vfs.OPEN_TRANSIENT_DB|vfs.OPEN_TEMP_JOURNAL|vfs.OPEN_SUBJOURNAL|vfs.OPEN_SUPER_JOURNAL) != 0 || name == "" {
		return &memoryFile{manager: &v.locks, name: name}, flags | vfs.OPEN_MEMORY, nil
	}

	backend := v.core.Backend()
	mode := os.FileMode(sqliteFileMode)
	openFlags := os.O_RDONLY
	if flags&vfs.OPEN_READONLY == 0 {
		openFlags = os.O_RDWR
	}
	if flags&vfs.OPEN_CREATE != 0 {
		openFlags |= os.O_CREATE
	}
	if flags&vfs.OPEN_EXCLUSIVE != 0 {
		openFlags |= os.O_EXCL
	}
	file, err := backend.OpenFile(name, openFlags, mode)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && flags&vfs.OPEN_CREATE == 0 {
			return nil, flags, sqlite3.CANTOPEN
		}
		if errors.Is(err, os.ErrPermission) {
			return nil, flags, sqlite3.READONLY
		}
		return nil, flags, sqlite3.CANTOPEN
	}

	return &aferoFile{
		file:          file,
		backend:       backend,
		manager:       &v.locks,
		name:          name,
		readOnly:      flags&vfs.OPEN_READONLY != 0,
		deleteOnClose: flags&vfs.OPEN_DELETEONCLOSE != 0,
	}, flags, nil
}

func (v *aferoVFS) Delete(name string, _ bool) error {
	err := v.core.Backend().Remove(v.core.ResolvePath(name))
	if errors.Is(err, os.ErrNotExist) {
		return sqlite3.IOERR_DELETE_NOENT
	}
	if err != nil {
		return sqlite3.IOERR_DELETE
	}
	return nil
}

func (v *aferoVFS) Access(name string, flags vfs.AccessFlag) (bool, error) {
	name = v.core.ResolvePath(name)
	info, err := v.core.Backend().Stat(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, sqlite3.IOERR_ACCESS
	}
	if flags == vfs.ACCESS_READWRITE && info.Mode().Perm()&0o222 == 0 {
		return false, nil
	}
	return true, nil
}

func (v *aferoVFS) FullPathname(name string) (string, error) {
	return v.core.ResolvePath(name), nil
}

type aferoFile struct {
	file          afero.File
	backend       afero.Fs
	manager       *lockManager
	name          string
	readOnly      bool
	deleteOnClose bool
	lock          vfs.LockLevel
}

var _ vfs.FileLockState = (*aferoFile)(nil)

func (f *aferoFile) ReadAt(p []byte, off int64) (int, error) {
	n, err := f.file.ReadAt(p, off)
	if n < len(p) && (err == nil || errors.Is(err, io.ErrUnexpectedEOF)) {
		err = io.EOF
	}
	return n, err
}

func (f *aferoFile) WriteAt(p []byte, off int64) (int, error) {
	if f.readOnly {
		return 0, sqlite3.READONLY
	}
	n, err := f.file.WriteAt(p, off)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	return n, err
}

func (f *aferoFile) Truncate(size int64) error {
	if f.readOnly {
		return sqlite3.READONLY
	}
	return f.file.Truncate(size)
}

func (f *aferoFile) Sync(vfs.SyncFlag) error {
	if err := f.file.Sync(); err != nil {
		return sqlite3.IOERR_FSYNC
	}
	return nil
}

func (f *aferoFile) Size() (int64, error) {
	info, err := f.file.Stat()
	if err != nil {
		return 0, sqlite3.IOERR_FSTAT
	}
	return info.Size(), nil
}

func (f *aferoFile) Lock(level vfs.LockLevel) error {
	if err := f.manager.lock(f, level); err != nil {
		return err
	}
	f.lock = level
	return nil
}

func (f *aferoFile) Unlock(level vfs.LockLevel) error {
	if err := f.manager.unlock(f, level); err != nil {
		return err
	}
	f.lock = level
	return nil
}

func (f *aferoFile) CheckReservedLock() (bool, error) {
	return f.manager.reserved(f.name), nil
}

func (f *aferoFile) LockState() vfs.LockLevel {
	return f.lock
}

func (f *aferoFile) SectorSize() int {
	return 4096
}

func (f *aferoFile) DeviceCharacteristics() vfs.DeviceCharacteristic {
	return vfs.IOCAP_SUBPAGE_READ | vfs.IOCAP_POWERSAFE_OVERWRITE
}

func (f *aferoFile) Close() error {
	_ = f.Unlock(vfs.LOCK_NONE)
	err := f.file.Close()
	if f.deleteOnClose {
		if removeErr := f.backend.Remove(f.name); err == nil {
			err = removeErr
		}
	}
	return err
}

type memoryFile struct {
	data    vfsutil.SliceFile
	manager *lockManager
	name    string
	lock    vfs.LockLevel
}

var _ vfs.FileLockState = (*memoryFile)(nil)

func (f *memoryFile) ReadAt(p []byte, off int64) (int, error) { return f.data.ReadAt(p, off) }

func (f *memoryFile) WriteAt(p []byte, off int64) (int, error) { return f.data.WriteAt(p, off) }

func (f *memoryFile) Truncate(size int64) error { return f.data.Truncate(size) }

func (f *memoryFile) Sync(vfs.SyncFlag) error { return nil }

func (f *memoryFile) Size() (int64, error) { return f.data.Size() }

func (f *memoryFile) Lock(level vfs.LockLevel) error {
	if err := f.manager.lock(f, level); err != nil {
		return err
	}
	f.lock = level
	return nil
}

func (f *memoryFile) Unlock(level vfs.LockLevel) error {
	if err := f.manager.unlock(f, level); err != nil {
		return err
	}
	f.lock = level
	return nil
}

func (f *memoryFile) CheckReservedLock() (bool, error) { return f.manager.reserved(f.name), nil }

func (f *memoryFile) LockState() vfs.LockLevel { return f.lock }

func (f *memoryFile) SectorSize() int { return 4096 }

func (f *memoryFile) DeviceCharacteristics() vfs.DeviceCharacteristic {
	return vfs.IOCAP_SUBPAGE_READ | vfs.IOCAP_POWERSAFE_OVERWRITE
}

func (f *memoryFile) Close() error {
	return f.Unlock(vfs.LOCK_NONE)
}

type lockManager struct {
	mu     sync.Mutex
	states map[string]*lockState
}

type lockState struct {
	shared    map[vfs.File]bool
	reserved  vfs.File
	pending   vfs.File
	exclusive vfs.File
}

func (m *lockManager) state(name string) *lockState {
	if m.states == nil {
		m.states = make(map[string]*lockState)
	}
	state := m.states[name]
	if state == nil {
		state = &lockState{shared: make(map[vfs.File]bool)}
		m.states[name] = state
	}
	return state
}

func (m *lockManager) lock(file vfs.File, level vfs.LockLevel) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.state(fileName(file))
	if level <= lockStateOf(file) {
		return nil
	}
	switch level {
	case vfs.LOCK_SHARED:
		if (state.exclusive != nil && state.exclusive != file) || (state.pending != nil && state.pending != file) {
			return sqlite3.BUSY
		}
		state.shared[file] = true
	case vfs.LOCK_RESERVED:
		if (state.exclusive != nil && state.exclusive != file) || (state.reserved != nil && state.reserved != file) {
			return sqlite3.BUSY
		}
		state.shared[file] = true
		state.reserved = file
	case vfs.LOCK_PENDING:
		if (state.exclusive != nil && state.exclusive != file) || (state.pending != nil && state.pending != file) {
			return sqlite3.BUSY
		}
		state.shared[file] = true
		state.pending = file
	case vfs.LOCK_EXCLUSIVE:
		if state.exclusive != nil && state.exclusive != file {
			return sqlite3.BUSY
		}
		for owner := range state.shared {
			if owner != file {
				return sqlite3.BUSY
			}
		}
		if (state.reserved != nil && state.reserved != file) || (state.pending != nil && state.pending != file) {
			return sqlite3.BUSY
		}
		state.exclusive = file
		delete(state.shared, file)
	default:
		return sqlite3.IOERR_LOCK
	}
	return nil
}

func (m *lockManager) unlock(file vfs.File, level vfs.LockLevel) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.state(fileName(file))
	if level >= lockStateOf(file) {
		return nil
	}
	if level < vfs.LOCK_EXCLUSIVE && state.exclusive == file {
		state.exclusive = nil
	}
	if level < vfs.LOCK_PENDING && state.pending == file {
		state.pending = nil
	}
	if level < vfs.LOCK_RESERVED && state.reserved == file {
		state.reserved = nil
	}
	if level < vfs.LOCK_SHARED {
		delete(state.shared, file)
	} else {
		state.shared[file] = true
	}
	if state.exclusive == nil && state.pending == nil && state.reserved == nil && len(state.shared) == 0 {
		delete(m.states, fileName(file))
	}
	return nil
}

func (m *lockManager) reserved(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.states[name] != nil && m.states[name].reserved != nil
}

func fileName(file vfs.File) string {
	switch f := file.(type) {
	case *aferoFile:
		return f.name
	case *memoryFile:
		return f.name
	default:
		return ""
	}
}

func lockStateOf(file vfs.File) vfs.LockLevel {
	switch f := file.(type) {
	case *aferoFile:
		return f.lock
	case *memoryFile:
		return f.lock
	default:
		return vfs.LOCK_NONE
	}
}
