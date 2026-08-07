package path

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
)

func run(t *testing.T, script string) string {
	t.Helper()
	rt := goja.New()
	registry := new(require.Registry)
	registry.Enable(rt)
	v, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return v.String()
}

func TestPathPosixFunctions(t *testing.T) {
	out := run(t, `
		const p = require("path");
		JSON.stringify([
			p.join("/a", "/b", "c"),
			p.resolve("/a", "b", "..", "c"),
			p.normalize("/a//b/./c/../d"),
			p.basename("/a/b/c.txt"),
			p.basename("/a/b/c.txt", ".txt"),
			p.dirname("/a/b/c.txt"),
			p.extname("/a/b/c.txt"),
			p.isAbsolute("/a"),
			p.relative("/a/b/c", "/a/x/y"),
			p.sep, p.delimiter,
			p.posix.join("a", "b"),
			p.win32.join("C:\\a", "b"),
		]);
	`)
	want := `["/a/b/c","/a/c","/a/b/d","c.txt","c","/a/b",".txt",true,"../../x/y","/",":","a/b","C:\\a\\b"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestPathParseFormat(t *testing.T) {
	out := run(t, `
		const p = require("path");
		const parsed = p.parse("/a/b/c.txt");
		const formatted = p.format(parsed);
		JSON.stringify([parsed.root, parsed.dir, parsed.base, parsed.name, parsed.ext, formatted]);
	`)
	want := `["/","/a/b","c.txt","c",".txt","/a/b/c.txt"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestPathWin32(t *testing.T) {
	out := run(t, `
		const w = require("path").win32;
		JSON.stringify([
			w.parse("C:\\Users\\x\\a.txt").name,
			w.isAbsolute("C:\\x"),
			w.isAbsolute("\\x"),
			w.isAbsolute("C:x"),
			w.resolve("C:\\a", "..\\b"),
			w.normalize("C:\\a\\..\\b\\"),
			w.basename("C:\\a\\b.txt"),
		]);
	`)
	want := `["a",true,true,false,"C:\\b","C:\\b","b.txt"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestPathNodeAlias(t *testing.T) {
	out := run(t, `
		const p = require("path");
		const np = require("node:path");
		JSON.stringify(p.resolve === np.resolve && p.posix === np.posix);
	`)
	if out != "true" {
		t.Fatalf("got %s", out)
	}
}
