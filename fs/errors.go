package fs

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

type pathError struct {
	op   string
	path string
	dest string
	err  error
}

func (e *pathError) Error() string {
	if e.dest != "" {
		return fmt.Sprintf("%s %s -> %s: %v", e.op, e.path, e.dest, e.err)
	}
	return fmt.Sprintf("%s %s: %v", e.op, e.path, e.err)
}

func (e *pathError) Unwrap() error {
	return e.err
}

func wrapPathError(op, path, dest string, err error) error {
	if err == nil {
		return nil
	}
	return &pathError{op: op, path: path, dest: dest, err: err}
}

func errorCode(err error) string {
	if errors.Is(err, ErrClosedHandle) {
		return "EBADF"
	}
	if errors.Is(err, os.ErrNotExist) {
		return "ENOENT"
	}
	if errors.Is(err, os.ErrExist) {
		return "EEXIST"
	}
	if errors.Is(err, os.ErrPermission) {
		return "EACCES"
	}
	if errors.Is(err, syscall.ENOTDIR) {
		return "ENOTDIR"
	}
	if errors.Is(err, syscall.EISDIR) {
		return "EISDIR"
	}
	if errors.Is(err, syscall.ENOSPC) {
		return "ENOSPC"
	}
	if errors.Is(err, syscall.EINVAL) {
		return "EINVAL"
	}
	if errors.Is(err, syscall.ENOSYS) {
		return "ENOSYS"
	}
	return "EIO"
}

func errorDetails(err error) (op, path, dest string) {
	var pe *pathError
	if errors.As(err, &pe) {
		return pe.op, pe.path, pe.dest
	}
	var osPathErr *os.PathError
	if errors.As(err, &osPathErr) {
		return osPathErr.Op, osPathErr.Path, ""
	}
	return "", "", ""
}
