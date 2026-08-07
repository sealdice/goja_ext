package fs

import (
	"errors"
	"io"
	"os"
	"path"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/spf13/afero"
)

const (
	openRead = 1 << iota
	openWrite
	openAppend
	openTruncate
	openCreate
	openCreateNew
)

const defaultStreamChunkSize = 64 * 1024

type config struct {
	backend    afero.Fs
	backendSet bool
	cwd        string
	cwdSet     bool
	chunkSize  int
	withStream bool
}

// Option configures an Afero-backed FS instance.
type Option func(*config) error

// Capability is an optional host-provided filesystem capability.
// Concrete capability interfaces live in fs/extra.
type Capability interface{}

func WithFS(backend afero.Fs) Option {
	return func(cfg *config) error {
		if backend == nil {
			return errors.New("fs: backend must not be nil")
		}
		cfg.backend = backend
		cfg.backendSet = true
		return nil
	}
}

func WithCwd(cwd string) Option {
	return func(cfg *config) error {
		if cwd == "" {
			return errors.New("fs: cwd must not be empty")
		}
		cfg.cwd = normalizeCwd(cwd)
		cfg.cwdSet = true
		return nil
	}
}

func WithStreams(enabled bool) Option {
	return func(cfg *config) error {
		cfg.withStream = enabled
		return nil
	}
}

func WithStreamChunkSize(size int) Option {
	return func(cfg *config) error {
		if size <= 0 {
			return errors.New("fs: stream chunk size must be positive")
		}
		cfg.chunkSize = size
		return nil
	}
}

// WithExtraCapabilities is reserved for fs/extra capability registration.
// The core accepts it as a no-op so callers can share option lists between
// the core and optional extension packages.
func WithExtraCapabilities(_ ...Capability) Option {
	return func(*config) error { return nil }
}

// Core owns the backend, logical cwd, and backend-independent file policy.
type Core struct {
	backend   afero.Fs
	cwdMu     sync.RWMutex
	cwd       string
	chunkSize int
}

// NewCore constructs a runtime-independent Afero FS core.
func NewCore(opts ...Option) (*Core, error) {
	cfg, err := configFromOptions(opts)
	if err != nil {
		return nil, err
	}
	return newCoreFromConfig(cfg)
}

func configFromOptions(opts []Option) (config, error) {
	cfg := config{backend: afero.NewOsFs(), chunkSize: defaultStreamChunkSize}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&cfg); err != nil {
			return config{}, err
		}
	}
	if cfg.backend == nil {
		cfg.backend = afero.NewOsFs()
	}
	return cfg, nil
}

func newCoreFromConfig(cfg config) (*Core, error) {
	if !cfg.cwdSet {
		if _, ok := cfg.backend.(*afero.OsFs); ok {
			wd, err := os.Getwd()
			if err != nil {
				return nil, err
			}
			cfg.cwd = normalizeCwd(wd)
		} else {
			cfg.cwd = "/"
		}
	}
	return &Core{
		backend:   cfg.backend,
		cwd:       cfg.cwd,
		chunkSize: cfg.chunkSize,
	}, nil
}

func coreConfigConflict(existing, incoming config) string {
	if existing.backendSet != incoming.backendSet {
		return "backend"
	}
	if existing.backendSet && !sameBackend(existing.backend, incoming.backend) {
		return "backend"
	}
	if existing.cwdSet != incoming.cwdSet || existing.cwd != incoming.cwd {
		return "cwd"
	}
	if existing.chunkSize != incoming.chunkSize {
		return "chunk size"
	}
	return ""
}

func sameBackend(left, right afero.Fs) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	lv := reflect.ValueOf(left)
	rv := reflect.ValueOf(right)
	return lv.Type() == rv.Type() && lv.Comparable() && rv.Comparable() && lv.Interface() == rv.Interface()
}

func (c *Core) Backend() afero.Fs {
	return c.backend
}

func (c *Core) Cwd() string {
	c.cwdMu.RLock()
	defer c.cwdMu.RUnlock()
	return c.cwd
}

func (c *Core) Chdir(directory string) error {
	resolved := c.ResolvePath(directory)
	info, err := c.backend.Stat(resolved)
	if err != nil {
		return wrapPathError("chdir", resolved, "", err)
	}
	if !info.IsDir() {
		return wrapPathError("chdir", resolved, "", errors.New("not a directory"))
	}
	c.cwdMu.Lock()
	c.cwd = resolved
	c.cwdMu.Unlock()
	return nil
}

func (c *Core) ResolvePath(input string) string {
	if input == "" {
		return c.Cwd()
	}
	if strings.HasPrefix(input, "file://") {
		input = strings.TrimPrefix(input, "file://")
	}
	if path.IsAbs(input) {
		return path.Clean(input)
	}
	return path.Join(c.Cwd(), input)
}

func (c *Core) ChunkSize() int {
	return c.chunkSize
}

func (c *Core) Mkdir(name string, perm os.FileMode, recursive bool) error {
	resolved := c.ResolvePath(name)
	var err error
	if recursive {
		err = c.backend.MkdirAll(resolved, perm)
	} else {
		err = c.backend.Mkdir(resolved, perm)
	}
	if err != nil {
		return wrapPathError("mkdir", resolved, "", err)
	}
	return nil
}

func (c *Core) MkdirAll(name string, perm os.FileMode) error {
	return c.Mkdir(name, perm, true)
}

func (c *Core) Remove(name string, recursive bool) error {
	resolved := c.ResolvePath(name)
	var err error
	if recursive {
		err = c.backend.RemoveAll(resolved)
	} else {
		err = c.backend.Remove(resolved)
	}
	if err != nil {
		return wrapPathError("remove", resolved, "", err)
	}
	return nil
}

func (c *Core) Rename(oldname, newname string) error {
	oldpath := c.ResolvePath(oldname)
	newpath := c.ResolvePath(newname)
	if err := c.backend.Rename(oldpath, newpath); err != nil {
		return wrapPathError("rename", oldpath, newpath, err)
	}
	return nil
}

func (c *Core) Stat(name string) (os.FileInfo, error) {
	resolved := c.ResolvePath(name)
	info, err := c.backend.Stat(resolved)
	if err != nil {
		return nil, wrapPathError("stat", resolved, "", err)
	}
	return info, nil
}

func (c *Core) Chmod(name string, mode os.FileMode) error {
	resolved := c.ResolvePath(name)
	if err := c.backend.Chmod(resolved, mode); err != nil {
		return wrapPathError("chmod", resolved, "", err)
	}
	return nil
}

func (c *Core) Chown(name string, uid, gid int) error {
	resolved := c.ResolvePath(name)
	if err := c.backend.Chown(resolved, uid, gid); err != nil {
		return wrapPathError("chown", resolved, "", err)
	}
	return nil
}

func (c *Core) Chtimes(name string, atime, mtime time.Time) error {
	resolved := c.ResolvePath(name)
	if err := c.backend.Chtimes(resolved, atime, mtime); err != nil {
		return wrapPathError("utime", resolved, "", err)
	}
	return nil
}

type FileHandle struct {
	core   *Core
	file   afero.File
	mu     sync.Mutex
	closed bool
}

func (c *Core) OpenFile(name string, flags int, perm os.FileMode) (*FileHandle, error) {
	resolved := c.ResolvePath(name)
	goFlags := os.O_RDONLY
	switch {
	case flags&openRead != 0 && flags&openWrite != 0:
		goFlags = os.O_RDWR
	case flags&openWrite != 0:
		goFlags = os.O_WRONLY
	}
	if flags&openAppend != 0 {
		goFlags |= os.O_APPEND
	}
	if flags&openTruncate != 0 {
		goFlags |= os.O_TRUNC
	}
	if flags&openCreate != 0 {
		goFlags |= os.O_CREATE
	}
	if flags&openCreateNew != 0 {
		goFlags |= os.O_CREATE | os.O_EXCL
	}
	file, err := c.backend.OpenFile(resolved, goFlags, perm)
	if err != nil {
		return nil, wrapPathError("open", resolved, "", err)
	}
	return &FileHandle{core: c, file: file}, nil
}

func (f *FileHandle) withOpen(fn func(afero.File) error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return ErrClosedHandle
	}
	return fn(f.file)
}

func (f *FileHandle) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	return f.file.Close()
}

func (f *FileHandle) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, ErrClosedHandle
	}
	return f.file.Read(p)
}

func (f *FileHandle) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, ErrClosedHandle
	}
	return f.file.Write(p)
}

func (f *FileHandle) WriteAll(p []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return ErrClosedHandle
	}
	for len(p) > 0 {
		n, err := f.file.Write(p)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		p = p[n:]
	}
	return nil
}

func (f *FileHandle) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, ErrClosedHandle
	}
	return f.file.Seek(offset, whence)
}

func (f *FileHandle) Truncate(size int64) error {
	return f.withOpen(func(file afero.File) error {
		return file.Truncate(size)
	})
}

func (f *FileHandle) Stat() (os.FileInfo, error) {
	var info os.FileInfo
	err := f.withOpen(func(file afero.File) error {
		var err error
		info, err = file.Stat()
		return err
	})
	return info, err
}

func (f *FileHandle) Sync() error {
	return f.withOpen(func(file afero.File) error {
		return file.Sync()
	})
}

func (f *FileHandle) Chtimes(atime, mtime time.Time) error {
	var name string
	if err := f.withOpen(func(file afero.File) error {
		name = file.Name()
		return nil
	}); err != nil {
		return err
	}
	return f.core.Chtimes(name, atime, mtime)
}

func (f *FileHandle) Readdir() ([]os.FileInfo, error) {
	var entries []os.FileInfo
	err := f.withOpen(func(file afero.File) error {
		var err error
		entries, err = file.Readdir(-1)
		return err
	})
	return entries, err
}

var ErrClosedHandle = errors.New("file handle is closed")
