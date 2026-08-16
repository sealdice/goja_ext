// Package extra adapts optional host filesystem operations to fs.Capability.
package extra

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	rootfs "github.com/dop251/goja_nodejs/fs"
	"github.com/spf13/afero"
)

type aferoLinks struct {
	backend afero.Fs
}

// FromAfero returns link-related capabilities advertised by backend. Afero's
// Lstater boolean is preserved: a fallback to Stat is reported as ENOSYS.
func FromAfero(backend afero.Fs) []rootfs.Capability {
	if backend == nil {
		return nil
	}
	_, hasLstat := backend.(afero.Lstater)
	_, hasReadlink := backend.(afero.LinkReader)
	_, hasSymlink := backend.(afero.Linker)
	if !hasLstat && !hasReadlink && !hasSymlink {
		return nil
	}
	return []rootfs.Capability{&aferoLinks{backend: backend}}
}

func (a *aferoLinks) CapabilityIdentity() any { return a.backend }

func (a *aferoLinks) Lstat(name string) (os.FileInfo, error) {
	lstater, ok := a.backend.(afero.Lstater)
	if !ok {
		return nil, syscall.ENOSYS
	}
	info, usedLstat, err := lstater.LstatIfPossible(name)
	if err != nil {
		return nil, err
	}
	if !usedLstat {
		return nil, syscall.ENOSYS
	}
	return info, nil
}

func (a *aferoLinks) Readlink(name string) (string, error) {
	reader, ok := a.backend.(afero.LinkReader)
	if !ok {
		return "", syscall.ENOSYS
	}
	target, err := reader.ReadlinkIfPossible(name)
	if errors.Is(err, afero.ErrNoReadlink) {
		return "", fmt.Errorf("%w: %w", syscall.ENOSYS, err)
	}
	return target, err
}

func (a *aferoLinks) Symlink(target, name string) error {
	linker, ok := a.backend.(afero.Linker)
	if !ok {
		return syscall.ENOSYS
	}
	err := linker.SymlinkIfPossible(target, name)
	if errors.Is(err, afero.ErrNoSymlink) {
		return fmt.Errorf("%w: %w", syscall.ENOSYS, err)
	}
	return err
}
