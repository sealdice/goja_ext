package fs

import (
	"os"
	"syscall"
	"time"

	"github.com/dop251/goja"
)

// nodeMode converts os.FileMode to a POSIX st_mode value (permission bits plus
// the S_IF* file-type bits in the top nibble), matching Node.js stats.mode.
func nodeMode(m os.FileMode) uint32 {
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

// nodeStatsValue builds a Node.js-shaped Stats object from an os.FileInfo.
// uid/gid/dev/ino/atime/ctime are populated when the backend provides them
// (e.g. afero.OsFs); otherwise they degrade to 0 / null.
func nodeStatsValue(rt *goja.Runtime, info os.FileInfo) *goja.Object {
	object := rt.NewObject()

	var stat *syscall.Stat_t
	if s, ok := info.Sys().(*syscall.Stat_t); ok {
		stat = s
	}

	_ = object.Set("name", info.Name())
	_ = object.Set("size", info.Size())
	_ = object.Set("mode", nodeMode(info.Mode()))
	_ = object.Set("mtime", dateValue(rt, info.ModTime()))

	atime := time.Time{}
	ctime := time.Time{}
	dev, ino, nlink, uid, gid, rdev, blksize, blocks := int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0)
	if stat != nil {
		atime = time.Unix(stat.Atim.Sec, stat.Atim.Nsec)
		ctime = time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec)
		dev, ino, nlink = int64(stat.Dev), int64(stat.Ino), int64(stat.Nlink)
		uid, gid, rdev = int64(stat.Uid), int64(stat.Gid), int64(stat.Rdev)
		blksize, blocks = stat.Blksize, stat.Blocks
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
	_ = object.Set("isFile", func(goja.FunctionCall) goja.Value { return rt.ToValue(mode.IsRegular()) })
	_ = object.Set("isDirectory", func(goja.FunctionCall) goja.Value { return rt.ToValue(mode.IsDir()) })
	_ = object.Set("isSymbolicLink", func(goja.FunctionCall) goja.Value { return rt.ToValue(mode&os.ModeSymlink != 0) })
	_ = object.Set("isSymlink", func(goja.FunctionCall) goja.Value { return rt.ToValue(mode&os.ModeSymlink != 0) })
	_ = object.Set("isBlockDevice", func(goja.FunctionCall) goja.Value {
		return rt.ToValue(mode&os.ModeDevice != 0 && mode&os.ModeCharDevice == 0)
	})
	_ = object.Set("isCharDevice", func(goja.FunctionCall) goja.Value {
		return rt.ToValue(mode&os.ModeCharDevice != 0)
	})
	_ = object.Set("isFIFO", func(goja.FunctionCall) goja.Value { return rt.ToValue(mode&os.ModeNamedPipe != 0) })
	_ = object.Set("isSocket", func(goja.FunctionCall) goja.Value { return rt.ToValue(mode&os.ModeSocket != 0) })
	return object
}

func dateOrNull(rt *goja.Runtime, t time.Time) goja.Value {
	if t.IsZero() {
		return goja.Null()
	}
	return dateValue(rt, t)
}
