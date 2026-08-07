package fs

import (
	"os"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/eventloop"
	"github.com/sealdice/goja_ext/require"
	"github.com/spf13/afero"
)

func runFSAPIScript(t *testing.T, script string) string {
	t.Helper()

	registry := require.NewRegistry()
	backend := afero.NewMemMapFs()
	loader := RequireWithOptions(
		WithFS(backend),
		WithCwd("/workspace"),
		WithStreams(true),
	)
	promiseLoader := RequirePromisesWithOptions(
		WithFS(backend),
		WithCwd("/workspace"),
	)
	registry.RegisterNativeModule("fs", loader)
	registry.RegisterNativeModule("fs/promises", promiseLoader)

	loop := eventloop.NewEventLoop(
		eventloop.EnableConsole(false),
		eventloop.WithRegistry(registry),
	)
	go loop.StartInForeground()
	t.Cleanup(func() { loop.Stop() })

	done := make(chan struct{})
	var result string
	var runErr error
	var rec any
	loop.RunOnLoop(func(vm *goja.Runtime) {
		defer func() {
			rec = recover()
			loop.SetTimeout(func(vm *goja.Runtime) {
				if value := vm.Get("__result"); value != nil && !goja.IsUndefined(value) {
					result = value.String()
				}
				close(done)
			}, 10*time.Millisecond)
		}()
		_, runErr = vm.RunString(script)
	})

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for fs script")
	}
	if rec != nil {
		t.Fatalf("panic: %v", rec)
	}
	if runErr != nil {
		t.Fatalf("script failed: %v", runErr)
	}
	return result
}

func TestDenoPathAPISyncAndPromises(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		const promises = require("fs/promises");
		fs.mkdirSync("docs", { recursive: true });
		fs.writeTextFileSync("docs/readme.txt", "hello");
		const bytes = fs.readFileSync("docs/readme.txt");
		const stat = fs.statSync("docs/readme.txt");
		const sync = [
			fs.cwd(),
			Array.prototype.slice.call(bytes).join(","),
			fs.readTextFileSync("docs/readme.txt"),
			stat.name,
			stat.size,
			stat.isFile(),
			fs.readDirSync("docs")[0].name,
		].join("|");

		Promise.all([
			promises.writeTextFile("docs/async.txt", "async"),
			promises.readTextFile("docs/readme.txt"),
		]).then(function (items) {
			globalThis.__result = sync + "|" + items[1];
		});
	`)
	if result != "/workspace|104,101,108,108,111|hello|readme.txt|5|true|readme.txt|hello" {
		t.Fatalf("unexpected Deno path API result: %s", result)
	}
}

func TestDenoFsAndPromisesShareCwd(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		const promises = require("fs/promises");
		fs.mkdirSync("shared", { recursive: true });
		fs.chdir("shared");
		promises.writeTextFile("inside.txt", "x").then(function () {
			globalThis.__result = [
				fs.cwd(),
				fs.readTextFileSync("inside.txt"),
				fs.statSync("../shared/inside.txt").isFile(),
			].join("|");
		});
	`)
	if result != "/workspace/shared|x|true" {
		t.Fatalf("fs and fs/promises did not share cwd: %s", result)
	}
}

func TestDenoRequireWithLoopRunsAsyncOperationsOffLoop(t *testing.T) {
	registry := require.NewRegistry()
	backend := &blockingOpenFs{
		Fs:      afero.NewMemMapFs(),
		release: make(chan struct{}),
	}
	if err := backend.Fs.MkdirAll("/workspace", 0o755); err != nil {
		t.Fatal(err)
	}
	loop := eventloop.NewEventLoop(
		eventloop.EnableConsole(false),
		eventloop.WithRegistry(registry),
	)
	registry.RegisterNativeModule("fs", RequireWithLoop(
		loop,
		WithFS(backend),
		WithCwd("/workspace"),
	))
	registry.RegisterNativeModule("fs/promises", RequirePromisesWithLoop(
		loop,
		WithFS(backend),
		WithCwd("/workspace"),
	))

	go loop.StartInForeground()
	t.Cleanup(func() { loop.Stop() })

	scriptReturned := make(chan struct{})
	loop.RunOnLoop(func(vm *goja.Runtime) {
		_, err := vm.RunString(`
			const fs = require("fs");
			fs.writeTextFile("blocked.txt", "x").then(function () {
				globalThis.__done = true;
			});
			globalThis.__scheduled = true;
		`)
		if err != nil {
			panic(err)
		}
		close(scriptReturned)
	})

	select {
	case <-scriptReturned:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("async write blocked the event loop")
	}
	close(backend.release)

	done := make(chan struct{})
	loop.RunOnLoop(func(vm *goja.Runtime) {
		loop.SetTimeout(func(vm *goja.Runtime) {
			if !vm.Get("__done").ToBoolean() {
				t.Error("async write promise did not settle")
			}
			close(done)
		}, 20*time.Millisecond)
	})
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for async write")
	}
}

func TestDenoLoaderDoesNotShareCwdAcrossRuntimes(t *testing.T) {
	backend := afero.NewMemMapFs()
	if err := backend.MkdirAll("/workspace/a", 0o755); err != nil {
		t.Fatal(err)
	}
	loader := RequireWithOptions(
		WithFS(backend),
		WithCwd("/workspace"),
	)

	registry1 := require.NewRegistry()
	registry1.RegisterNativeModule("fs", loader)
	loop1 := eventloop.NewEventLoop(
		eventloop.EnableConsole(false),
		eventloop.WithRegistry(registry1),
	)
	go loop1.StartInForeground()
	t.Cleanup(func() { loop1.Stop() })

	registry2 := require.NewRegistry()
	registry2.RegisterNativeModule("fs", loader)
	loop2 := eventloop.NewEventLoop(
		eventloop.EnableConsole(false),
		eventloop.WithRegistry(registry2),
	)
	go loop2.StartInForeground()
	t.Cleanup(func() { loop2.Stop() })

	done1 := make(chan struct{})
	loop1.RunOnLoop(func(vm *goja.Runtime) {
		_, err := vm.RunString(`
			const fs = require("fs");
			fs.chdir("a");
		`)
		if err != nil {
			panic(err)
		}
		close(done1)
	})
	select {
	case <-done1:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for first runtime")
	}

	done2 := make(chan string, 1)
	loop2.RunOnLoop(func(vm *goja.Runtime) {
		value, err := vm.RunString(`
			const fs = require("fs");
			fs.cwd();
		`)
		if err != nil {
			panic(err)
		}
		done2 <- value.String()
	})
	select {
	case cwd := <-done2:
		if cwd != "/workspace" {
			t.Fatalf("second runtime cwd leaked from first runtime: %s", cwd)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for second runtime")
	}
}

func TestDenoRegisterWithOptionsInstallsNodeAliases(t *testing.T) {
	registry := require.NewRegistry()
	RegisterWithOptions(
		registry,
		WithFS(afero.NewMemMapFs()),
		WithCwd("/workspace"),
	)
	loop := eventloop.NewEventLoop(
		eventloop.EnableConsole(false),
		eventloop.WithRegistry(registry),
	)
	go loop.StartInForeground()
	t.Cleanup(func() { loop.Stop() })

	done := make(chan string, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		value, err := vm.RunString(`
			const fs = require("fs");
			const nodeFs = require("node:fs");
			const promises = require("fs/promises");
			const nodePromises = require("node:fs/promises");
			String(
				fs.cwd() === nodeFs.cwd() &&
				typeof promises.readFile === "function" &&
				promises.readFile === nodePromises.readFile
			);
		`)
		if err != nil {
			panic(err)
		}
		done <- value.String()
	})
	select {
	case result := <-done:
		if result != "true" {
			t.Fatalf("node aliases were not installed: %s", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for node alias test")
	}
}

func TestDenoRegisterWithLoopInstallsAsyncNodeAliases(t *testing.T) {
	registry := require.NewRegistry()
	loop := eventloop.NewEventLoop(
		eventloop.EnableConsole(false),
		eventloop.WithRegistry(registry),
	)
	RegisterWithLoop(
		registry,
		loop,
		WithFS(afero.NewMemMapFs()),
		WithCwd("/workspace"),
	)
	go loop.StartInForeground()
	t.Cleanup(func() { loop.Stop() })

	done := make(chan string, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		value, err := vm.RunString(`
			const fs = require("node:fs");
			const promises = require("node:fs/promises");
			fs.mkdirSync("alias", { recursive: true });
			promises.writeTextFile("alias/file.txt", "x").then(function () {
				globalThis.__result = fs.readTextFileSync("alias/file.txt");
			});
			"scheduled";
		`)
		if err != nil {
			panic(err)
		}
		if value.String() != "scheduled" {
			panic("script did not schedule promise")
		}
		loop.SetTimeout(func(vm *goja.Runtime) {
			done <- vm.Get("__result").String()
		}, 20*time.Millisecond)
	})
	select {
	case result := <-done:
		if result != "x" {
			t.Fatalf("async node alias did not complete: %s", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for async node alias test")
	}
}

func TestDenoOpenAndFsFile(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		const file = fs.openSync("data.bin", {
			read: true,
			write: true,
			create: true,
			truncate: true,
		});
		file.writeSync(new Uint8Array([65, 66]));
		file.seekSync(0, 0);
		const buf = new Uint8Array(2);
		const count = file.readSync(buf);
		const stat = file.statSync();
		file.close();
		globalThis.__result = [
			count,
			Array.prototype.slice.call(buf).join(","),
			stat.size,
		].join("|");
	`)
	if result != "2|65,66|2" {
		t.Fatalf("unexpected FsFile result: %s", result)
	}
}

func TestDenoUtimeAndFsFileUtime(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		fs.writeTextFileSync("time.txt", "x");
		fs.utimeSync("time.txt", 1, 2);
		const first = fs.statSync("time.txt").mtime.getTime();

		const file = fs.openSync("time.txt", { read: true });
		file.utimeSync(new Date(3000), new Date(4000));
		const second = file.statSync().mtime.getTime();
		file.close();

		fs.writeTextFileSync("async-time.txt", "x");
		fs.utime("async-time.txt", new Date(5000), new Date(6000))
			.then(() => {
				const asyncFile = fs.openSync("async-time.txt", { read: true });
				return asyncFile.utime(7, 8).then(() => {
					const third = fs.statSync("async-time.txt").mtime.getTime();
					asyncFile.close();
					globalThis.__result = [first, second, third].join("|");
				});
			});
	`)
	if result != "2000|4000|8000" {
		t.Fatalf("unexpected utime result: %s", result)
	}
}

func TestDenoPathMutationTempAndReadDirAPIs(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		fs.mkdirSync("ops", { recursive: true });
		fs.writeTextFileSync("ops/source.txt", "abcdef");
		fs.copyFileSync("ops/source.txt", "ops/copy.txt");
		fs.renameSync("ops/copy.txt", "ops/moved.txt");
		fs.truncateSync("ops/moved.txt", 3);
		fs.chmodSync("ops/moved.txt", 0o600);
		fs.chownSync("ops/moved.txt", 1, 2);

		const created = fs.createSync("ops/created.txt");
		created.writeSync("x");
		created.close();

		const tempFile = fs.makeTempFileSync({
			dir: "ops",
			prefix: "p-",
			suffix: ".tmp",
		});
		const tempDir = fs.makeTempDirSync({
			dir: "ops",
			prefix: "d-",
			suffix: ".dir",
		});

		fs.copyFile("ops/source.txt", "ops/async-copy.txt")
			.then(() => fs.rename("ops/async-copy.txt", "ops/async-moved.txt"))
			.then(() => fs.truncate("ops/async-moved.txt", 2))
			.then(() => fs.remove("ops/source.txt"))
			.then(() => fs.makeTempFile({
				dir: "ops",
				prefix: "a-",
				suffix: ".dat",
			}))
			.then((asyncTemp) => {
				const iterator = fs.readDir("ops");
				const names = [];
				function pump() {
					return iterator.next().then((item) => {
						if (item.done) return names;
						names.push(item.value.name);
						return pump();
					});
				}
				return pump().then((names) => {
					globalThis.__result = [
						fs.readTextFileSync("ops/moved.txt"),
						String(tempFile.startsWith("/workspace/ops/p-") && tempFile.endsWith(".tmp")),
						String(tempDir.startsWith("/workspace/ops/d-") && tempDir.endsWith(".dir")),
						String(fs.statSync(tempDir).isDirectory()),
						fs.readTextFileSync("ops/async-moved.txt"),
						String(asyncTemp.endsWith(".dat")),
						String(names.indexOf("source.txt") === -1),
						String(names.indexOf("moved.txt") >= 0 &&
							names.indexOf("async-moved.txt") >= 0 &&
							names.indexOf("created.txt") >= 0),
					].join("|");
				});
			});
	`)
	if result != "abc|true|true|true|ab|true|true|true" {
		t.Fatalf("unexpected path mutation/temp/readDir result: %s", result)
	}
}

type blockingOpenFs struct {
	afero.Fs
	release chan struct{}
}

func (fsys *blockingOpenFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	<-fsys.release
	return fsys.Fs.OpenFile(name, flag, perm)
}
