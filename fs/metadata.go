package fs

import (
	"os"
	"syscall"
	"time"

	"github.com/dop251/goja"
)

// posixMode converts os.FileMode to permission and S_IF* bits.
func posixMode(m os.FileMode) uint32 {
	mode := uint32(m.Perm())
	switch {
	case m.IsRegular():
		mode |= 0x8000 // S_IFREG
	case m.IsDir():
		mode |= 0x4000 // S_IFDIR
	case m&os.ModeSymlink != 0:
		mode |= 0xA000 // S_IFLNK
	case m&os.ModeDevice != 0:
		if m&os.ModeCharDevice != 0 {
			mode |= 0x2000 // S_IFCHR
		} else {
			mode |= 0x6000 // S_IFBLK
		}
	case m&os.ModeNamedPipe != 0:
		mode |= 0x1000 // S_IFIFO
	case m&os.ModeSocket != 0:
		mode |= 0xC000 // S_IFSOCK
	}
	return mode
}

// fileInfoValue builds the Deno FileInfo property shape. Platform metadata is
// null when the backend does not expose an os-level stat structure.
func fileInfoValue(rt *goja.Runtime, info os.FileInfo) *goja.Object {
	object := rt.NewObject()

	var stat *syscall.Stat_t
	if s, ok := info.Sys().(*syscall.Stat_t); ok {
		stat = s
	}

	_ = object.Set("size", info.Size())
	_ = object.Set("mode", posixMode(info.Mode()))
	_ = object.Set("mtime", dateOrNull(rt, info.ModTime()))

	atime := time.Time{}
	ctime := time.Time{}
	var dev, ino, nlink, uid, gid, rdev, blksize, blocks goja.Value
	dev, ino, nlink, uid, gid, rdev, blksize, blocks = goja.Null(), goja.Null(), goja.Null(), goja.Null(), goja.Null(), goja.Null(), goja.Null(), goja.Null()
	if stat != nil {
		atime = time.Unix(stat.Atim.Sec, stat.Atim.Nsec)
		ctime = time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec)
		dev, ino, nlink = rt.ToValue(int64(stat.Dev)), rt.ToValue(int64(stat.Ino)), rt.ToValue(int64(stat.Nlink))
		uid, gid, rdev = rt.ToValue(int64(stat.Uid)), rt.ToValue(int64(stat.Gid)), rt.ToValue(int64(stat.Rdev))
		blksize, blocks = rt.ToValue(stat.Blksize), rt.ToValue(stat.Blocks)
	}
	_ = object.Set("atime", dateOrNull(rt, atime))
	_ = object.Set("ctime", dateOrNull(rt, ctime))
	_ = object.Set("birthtime", goja.Null())
	_ = object.Set("dev", dev)
	_ = object.Set("ino", ino)
	_ = object.Set("nlink", nlink)
	_ = object.Set("uid", uid)
	_ = object.Set("gid", gid)
	_ = object.Set("rdev", rdev)
	_ = object.Set("blksize", blksize)
	_ = object.Set("blocks", blocks)

	mode := info.Mode()
	_ = object.Set("isFile", mode.IsRegular())
	_ = object.Set("isDirectory", mode.IsDir())
	_ = object.Set("isSymlink", mode&os.ModeSymlink != 0)
	_ = object.Set("isBlockDevice", mode&os.ModeDevice != 0 && mode&os.ModeCharDevice == 0)
	_ = object.Set("isCharDevice", mode&os.ModeCharDevice != 0)
	_ = object.Set("isFifo", mode&os.ModeNamedPipe != 0)
	_ = object.Set("isSocket", mode&os.ModeSocket != 0)
	return object
}

func dateOrNull(rt *goja.Runtime, t time.Time) goja.Value {
	if t.IsZero() {
		return goja.Null()
	}
	return dateValue(rt, t)
}
