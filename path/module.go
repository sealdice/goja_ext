package path

import (
	"os"
	"path"
	"strings"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
)

const ModuleName = "path"

const (
	posixSep       = "/"
	posixDelimiter = ":"
	winSep         = "\\"
	winDelimiter   = ";"
)

func Require(rt *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)
	hostCwd, err := os.Getwd()
	if err != nil {
		hostCwd = ""
	}
	buildPathModule(rt, exports, hostCwd)
}

func buildPathModule(rt *goja.Runtime, exports *goja.Object, hostCwd string) {
	posixImpl := posixImpl{hostCwd: strings.ReplaceAll(hostCwd, "\\", "/")}
	winImpl := win32Impl{hostCwd: hostCwd}
	posix := newPathObject(rt, posixImpl)
	win32 := newPathObject(rt, winImpl)

	platform := pathImpl(posixImpl)
	if os.PathSeparator == '\\' {
		platform = winImpl
	}

	if err := exports.Set("sep", platform.sep()); err != nil {
		panic(err)
	}
	if err := exports.Set("delimiter", platform.delimiter()); err != nil {
		panic(err)
	}
	for _, name := range []string{
		"join", "resolve", "normalize", "relative", "isAbsolute",
		"basename", "dirname", "extname", "parse", "format", "toNamespacedPath",
	} {
		if err := exports.Set(name, posix.Get(name)); err != nil {
			panic(err)
		}
	}
	if err := exports.Set("posix", posix); err != nil {
		panic(err)
	}
	if err := exports.Set("win32", win32); err != nil {
		panic(err)
	}
}

// pathImpl abstracts the posix vs win32 path semantics.
type pathImpl interface {
	sep() string
	delimiter() string
	join(parts []string) string
	resolve(parts []string) string
	normalize(p string) string
	relative(from, to string) string
	isAbsolute(p string) bool
	basename(p string) string
	basenameExt(p, ext string) string
	dirname(p string) string
	extname(p string) string
	parse(p string) (root, dir, base, name, ext string)
	format(root, dir, base string) string
	toNamespacedPath(p string) string
}

func newPathObject(rt *goja.Runtime, impl pathImpl) *goja.Object {
	obj := rt.NewObject()
	set := func(name string, fn func(call goja.FunctionCall) goja.Value) {
		if err := obj.Set(name, fn); err != nil {
			panic(err)
		}
	}
	set("sep", func(goja.FunctionCall) goja.Value { return rt.ToValue(impl.sep()) })
	set("delimiter", func(goja.FunctionCall) goja.Value { return rt.ToValue(impl.delimiter()) })
	set("join", func(call goja.FunctionCall) goja.Value {
		return rt.ToValue(impl.join(stringValues(call.Arguments)))
	})
	set("resolve", func(call goja.FunctionCall) goja.Value {
		return rt.ToValue(impl.resolve(stringValues(call.Arguments)))
	})
	set("normalize", func(call goja.FunctionCall) goja.Value {
		return rt.ToValue(impl.normalize(str(call.Argument(0))))
	})
	set("relative", func(call goja.FunctionCall) goja.Value {
		return rt.ToValue(impl.relative(str(call.Argument(0)), str(call.Argument(1))))
	})
	set("isAbsolute", func(call goja.FunctionCall) goja.Value {
		return rt.ToValue(impl.isAbsolute(str(call.Argument(0))))
	})
	set("basename", func(call goja.FunctionCall) goja.Value {
		p := str(call.Argument(0))
		ext := call.Argument(1)
		if ext == nil || goja.IsUndefined(ext) {
			return rt.ToValue(impl.basename(p))
		}
		return rt.ToValue(impl.basenameExt(p, ext.String()))
	})
	set("dirname", func(call goja.FunctionCall) goja.Value {
		return rt.ToValue(impl.dirname(str(call.Argument(0))))
	})
	set("extname", func(call goja.FunctionCall) goja.Value {
		return rt.ToValue(impl.extname(str(call.Argument(0))))
	})
	set("parse", func(call goja.FunctionCall) goja.Value {
		root, dir, base, name, ext := impl.parse(str(call.Argument(0)))
		obj := rt.NewObject()
		_ = obj.Set("root", root)
		_ = obj.Set("dir", dir)
		_ = obj.Set("base", base)
		_ = obj.Set("name", name)
		_ = obj.Set("ext", ext)
		return obj
	})
	set("format", func(call goja.FunctionCall) goja.Value {
		obj := call.Argument(0).ToObject(rt)
		root := property(obj, "root")
		dir := property(obj, "dir")
		base := property(obj, "base")
		return rt.ToValue(impl.format(root, dir, base))
	})
	set("toNamespacedPath", func(call goja.FunctionCall) goja.Value {
		return rt.ToValue(impl.toNamespacedPath(str(call.Argument(0))))
	})
	return obj
}

func stringValues(args []goja.Value) []string {
	out := make([]string, len(args))
	for i, v := range args {
		out[i] = str(v)
	}
	return out
}

func str(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	return v.String()
}

func property(obj *goja.Object, name string) string {
	v := obj.Get(name)
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	return v.String()
}

// --- posix ---

type posixImpl struct {
	hostCwd string
}

func (posixImpl) sep() string       { return posixSep }
func (posixImpl) delimiter() string { return posixDelimiter }

func (p posixImpl) join(parts []string) string {
	return path.Join(parts...)
}

func (p posixImpl) resolve(parts []string) string {
	var resolved string
	absolute := false
	for i := len(parts) - 1; i >= -1 && !absolute; i-- {
		part := p.hostCwd
		if i >= 0 {
			part = parts[i]
		}
		if part == "" {
			continue
		}
		resolved = part + posixSep + resolved
		absolute = path.IsAbs(part)
	}
	return path.Clean(resolved)
}

func (posixImpl) normalize(p string) string { return path.Clean(p) }

func (posixImpl) relative(from, to string) string {
	if from == to {
		return ""
	}
	from = path.Clean(from)
	to = path.Clean(to)
	fromParts := strings.Split(from, "/")
	toParts := strings.Split(to, "/")
	i := 0
	for i < len(fromParts) && i < len(toParts) && fromParts[i] == toParts[i] {
		i++
	}
	rel := make([]string, 0, len(fromParts)-i+len(toParts)-i)
	for j := i; j < len(fromParts); j++ {
		if fromParts[j] != "" {
			rel = append(rel, "..")
		}
	}
	for j := i; j < len(toParts); j++ {
		if toParts[j] != "" {
			rel = append(rel, toParts[j])
		}
	}
	if len(rel) == 0 {
		return "."
	}
	return strings.Join(rel, "/")
}

func (posixImpl) isAbsolute(p string) bool { return path.IsAbs(p) }

func (posixImpl) basename(p string) string {
	if p == "" {
		return ""
	}
	return path.Base(p)
}

func (posixImpl) basenameExt(p, ext string) string {
	base := path.Base(p)
	if ext != "" && strings.HasSuffix(base, ext) && base != ext {
		return base[:len(base)-len(ext)]
	}
	return base
}

func (posixImpl) dirname(p string) string { return path.Dir(p) }

func (posixImpl) extname(p string) string { return path.Ext(p) }

func (posixImpl) parse(p string) (root, dir, base, name, ext string) {
	if path.IsAbs(p) {
		root = posixSep
	} else {
		root = ""
	}
	base = path.Base(p)
	if base == posixSep {
		dir = posixSep
		name = posixSep
		return
	}
	ext = path.Ext(base)
	if ext != "" {
		name = base[:len(base)-len(ext)]
	} else {
		name = base
	}
	dir = path.Dir(p)
	if dir == "." {
		dir = ""
	}
	if dir == posixSep {
		dir = root
	}
	return
}

func (posixImpl) format(root, dir, base string) string {
	if dir == "" {
		return root + base
	}
	if base == posixSep && dir == root {
		return root
	}
	if dir == root {
		return root + base
	}
	if strings.HasSuffix(dir, posixSep) {
		return dir + base
	}
	return dir + posixSep + base
}

func (posixImpl) toNamespacedPath(p string) string { return p }

func init() {
	require.RegisterCoreModule(ModuleName, Require)
}
