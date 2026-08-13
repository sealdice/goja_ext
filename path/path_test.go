package path_test

import (
	"runtime"
	"testing"

	"github.com/dop251/goja"
	rootfs "github.com/sealdice/goja_ext/fs"
	_ "github.com/sealdice/goja_ext/process"
	"github.com/sealdice/goja_ext/require"
	"github.com/spf13/afero"
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

func TestPathWin32Relative(t *testing.T) {
	out := run(t, `
		const w = require("path").win32;
		JSON.stringify([
			w.relative("C:\\a\\b","C:\\a\\x"),
			w.relative("C:\\a","C:\\a"),
			w.relative("C:\\a","D:\\a"),
		]);
	`)
	want := `["..\\x","","D:\\a"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestPathWin32Dirname(t *testing.T) {
	out := run(t, `
		const w = require("path").win32;
		JSON.stringify([
			w.dirname("C:\\a\\b\\c"),
			w.dirname("C:\\a"),
			w.dirname("a"),
		]);
	`)
	want := `["C:\\a\\b","C:\\",""]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestPathWin32Extname(t *testing.T) {
	out := run(t, `
		const w = require("path").win32;
		JSON.stringify([
			w.extname("C:\\a\\b.txt"),
			w.extname(".bashrc"),
			w.extname("a."),
		]);
	`)
	want := `[".txt","",""]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestPathWin32Format(t *testing.T) {
	out := run(t, `
		const w = require("path").win32;
		JSON.stringify([
			w.format({root:"C:\\",base:"a.txt"}),
			w.format({dir:"C:\\a\\b",base:"c.txt"}),
			w.format({dir:"C:\\a\\",base:"c"}),
		]);
	`)
	want := `["C:\\a.txt","C:\\a\\b\\c.txt","C:\\a\\c"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestPathWin32ParseFull(t *testing.T) {
	out := run(t, `
		const w = require("path").win32;
		const parsed = w.parse("C:\\Users\\x\\a.txt");
		JSON.stringify([parsed.root, parsed.dir, parsed.base, parsed.name, parsed.ext]);
	`)
	want := `["C:\\","C:\\Users\\x","a.txt","a",".txt"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestPathWin32BasenameExt(t *testing.T) {
	out := run(t, `
		const w = require("path").win32;
		JSON.stringify([
			w.basename("C:\\a\\b.txt",".txt"),
			w.basename("C:\\a\\b.txt",".js"),
		]);
	`)
	want := `["b","b.txt"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestPathWin32SepDelimiter(t *testing.T) {
	out := run(t, `
		const w = require("path").win32;
		JSON.stringify([w.sep, w.delimiter, typeof w.sep, typeof w.delimiter]);
	`)
	want := `["\\",";","string","string"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestDefaultPathFunctionsMatchHostPlatform(t *testing.T) {
	out := run(t, `
		const p = require("path");
		JSON.stringify([
			p.resolve === p.win32.resolve,
			p.resolve === p.posix.resolve,
			p.sep,
			p.delimiter
		]);
	`)
	want := `[false,true,"/",":"]`
	if runtime.GOOS == "windows" {
		want = `[true,false,"\\",";"]`
	}
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestPathFsAndProcessShareDynamicRuntimeCwd(t *testing.T) {
	rt := goja.New()
	registry := require.NewRegistry()
	rootfs.RegisterWithOptions(registry,
		rootfs.WithFS(afero.NewMemMapFs()),
		rootfs.WithCwd("/workspace"),
	)
	registry.Enable(rt)
	value, err := rt.RunString(`
		const p = require("path");
		const fs = require("fs");
		const process = require("process");
		const before = p.resolve("file.txt");
		fs.mkdirSync("next", { recursive: true });
		fs.chdir("next");
		const afterFs = [p.resolve("file.txt"), process.cwd(), fs.cwd()];
		process.chdir("..");
		JSON.stringify([before, afterFs, p.resolve("file.txt"), fs.cwd()]);
	`)
	if err != nil {
		t.Fatal(err)
	}
	want := `["/workspace/file.txt",["/workspace/next/file.txt","/workspace/next","/workspace/next"],"/workspace/file.txt","/workspace"]`
	if got := value.String(); got != want {
		t.Fatalf("shared cwd result = %s, want %s", got, want)
	}
}

func TestPathWin32IsAbsoluteVariants(t *testing.T) {
	out := run(t, `
		const w = require("path").win32;
		JSON.stringify([
			w.isAbsolute("C:/x"),
			w.isAbsolute("\\\\srv\\share"),
			w.isAbsolute("C|x"),
		]);
	`)
	want := `[true,true,false]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestPathWin32NormalizeEdges(t *testing.T) {
	out := run(t, `
		const w = require("path").win32;
		JSON.stringify([
			w.normalize(""),
			w.normalize("C:/a/b"),
		]);
	`)
	want := `[".","C:\\a\\b"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestPathWin32UNC(t *testing.T) {
	out := run(t, `
		const w = require("path").win32;
		JSON.stringify([
			w.isAbsolute("\\\\srv\\share"),
			w.normalize("\\\\srv\\share\\..\\a"),
		]);
	`)
	want := `[true,"\\\\srv\\share\\a"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}
