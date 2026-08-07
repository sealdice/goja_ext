package fs

import "github.com/dop251/goja"

// nodeConstants builds the fs.constants object (Linux values, matching Node on
// this platform).
func nodeConstants(rt *goja.Runtime) *goja.Object {
	obj := rt.NewObject()
	vals := map[string]int64{
		"F_OK": 0, "R_OK": 4, "W_OK": 2, "X_OK": 1,

		"O_RDONLY": 0, "O_WRONLY": 1, "O_RDWR": 2,
		"O_CREAT": 0x40, "O_EXCL": 0x80, "O_TRUNC": 0x200,
		"O_APPEND": 0x400, "O_SYNC": 0x101000, "O_DIRECTORY": 0x10000,
		"O_NOFOLLOW": 0x20000, "O_NONBLOCK": 0x800,

		"S_IFMT": 0xF000, "S_IFREG": 0x8000, "S_IFDIR": 0x4000,
		"S_IFCHR": 0x2000, "S_IFBLK": 0x6000, "S_IFIFO": 0x1000,
		"S_IFLNK": 0xA000, "S_IFSOCK": 0xC000,

		"COPYFILE_EXCL": 1, "COPYFILE_FICLONE": 2, "COPYFILE_FICLONE_FORCE": 4,

		"UV_FS_SYMLINK_DIR": 1, "UV_FS_SYMLINK_JUNCTION": 2,
	}
	for name, v := range vals {
		_ = obj.Set(name, v)
	}
	return obj
}
