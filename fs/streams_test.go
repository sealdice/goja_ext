package fs_test

import (
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/fs"
	"github.com/dop251/goja_nodejs/require"
	"github.com/spf13/afero"
)

func TestFsFileReadableStreamReadsFile(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		fs.writeTextFileSync("stream.txt", "hello");
		const file = fs.openSync("stream.txt", { read: true });
		const reader = file.readable.getReader();
		reader.read()
			.then((first) => reader.read().then((second) => {
				file.close();
				globalThis.__result = [
					Array.prototype.slice.call(first.value).join(","),
					first.done,
					String(second.value),
					second.done,
				].join("|");
			}));
	`)
	if result != "104,101,108,108,111|false|undefined|true" {
		t.Fatalf("unexpected readable stream result: %s", result)
	}
}

func TestFsFileWritableStreamWritesFile(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		const file = fs.openSync("writable.txt", {
			write: true,
			create: true,
			truncate: true,
		});
		const writer = file.writable.getWriter();
		writer.write(new Uint8Array([65]))
			.then(() => writer.write(new Uint8Array([66])))
			.then(() => writer.close())
			.then(() => {
				file.close();
				globalThis.__result = fs.readTextFileSync("writable.txt");
			});
	`)
	if result != "AB" {
		t.Fatalf("unexpected writable stream result: %s", result)
	}
}

func TestWriteFileConsumesReadableStream(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		const { ReadableStream } = require("streams");
		const source = new ReadableStream({
			start(controller) {
				controller.enqueue(new Uint8Array([65]));
				controller.enqueue(new Uint8Array([66]));
				controller.close();
			},
		});
		fs.writeFile("from-stream.txt", source).then(() => {
			globalThis.__result = fs.readTextFileSync("from-stream.txt");
		});
	`)
	if result != "AB" {
		t.Fatalf("unexpected writeFile stream result: %s", result)
	}
}

func TestWriteTextFileConsumesReadableStream(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		const { ReadableStream } = require("streams");
		const source = new ReadableStream({
			start(controller) {
				controller.enqueue("你");
				controller.enqueue("好");
				controller.close();
			},
		});
		fs.writeTextFile("text-stream.txt", source).then(() => {
			globalThis.__result = fs.readTextFileSync("text-stream.txt");
		});
	`)
	if result != "你好" {
		t.Fatalf("unexpected writeTextFile stream result: %s", result)
	}
}

func TestFsFileReadableClosesOnEOF(t *testing.T) {
	base := afero.NewMemMapFs()
	if err := afero.WriteFile(base, "/stream.txt", []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &closeTrackingFS{Fs: base}
	result := runFSAPIScriptWithBackend(t, backend, "/", `
		const fs = require("fs");
		const reader = fs.openSync("stream.txt").readable.getReader();
		function pump() {
			return reader.read().then((item) => item.done ? undefined : pump());
		}
		pump().then(() => { globalThis.__result = "done"; });
	`)
	if result != "done" {
		t.Fatalf("readable result = %q", result)
	}
	if got := backend.closeCalls(); got != 1 {
		t.Fatalf("readable EOF close calls = %d, want 1", got)
	}
}

func TestFsFileReadableClosesOnCancel(t *testing.T) {
	base := afero.NewMemMapFs()
	if err := afero.WriteFile(base, "/stream.txt", []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &closeTrackingFS{Fs: base}
	result := runFSAPIScriptWithBackend(t, backend, "/", `
		const fs = require("fs");
		const reader = fs.openSync("stream.txt").readable.getReader();
		reader.cancel("stop").then(() => { globalThis.__result = "cancelled"; });
	`)
	if result != "cancelled" {
		t.Fatalf("readable cancel result = %q", result)
	}
	if got := backend.closeCalls(); got != 1 {
		t.Fatalf("readable cancel close calls = %d, want 1", got)
	}
}

func TestFsFileWritableClosesOnClose(t *testing.T) {
	backend := &closeTrackingFS{Fs: afero.NewMemMapFs()}
	result := runFSAPIScriptWithBackend(t, backend, "/", `
		const fs = require("fs");
		const file = fs.createSync("stream.txt");
		const writer = file.writable.getWriter();
		writer.write(new Uint8Array([65]))
			.then(() => writer.close())
			.then(() => { globalThis.__result = "closed"; });
	`)
	if result != "closed" {
		t.Fatalf("writable close result = %q", result)
	}
	if got := backend.closeCalls(); got != 1 {
		t.Fatalf("writable close calls = %d, want 1", got)
	}
}

func TestFsFileWritableClosesOnAbort(t *testing.T) {
	backend := &closeTrackingFS{Fs: afero.NewMemMapFs()}
	result := runFSAPIScriptWithBackend(t, backend, "/", `
		const fs = require("fs");
		const writer = fs.createSync("stream.txt").writable.getWriter();
		writer.abort("stop").then(() => { globalThis.__result = "aborted"; });
	`)
	if result != "aborted" {
		t.Fatalf("writable abort result = %q", result)
	}
	if got := backend.closeCalls(); got != 1 {
		t.Fatalf("writable abort close calls = %d, want 1", got)
	}
}

func TestFsFileWritableClosesOnInvalidChunk(t *testing.T) {
	backend := &closeTrackingFS{Fs: afero.NewMemMapFs()}
	result := runFSAPIScriptWithBackend(t, backend, "/", `
		const writer = require("fs").createSync("stream.txt").writable.getWriter();
		writer.write("not bytes").then(
			() => { globalThis.__result = "resolved"; },
			(error) => { globalThis.__result = String(error instanceof TypeError); },
		);
	`)
	if result != "true" {
		t.Fatalf("invalid writable chunk result = %q", result)
	}
	if got := backend.closeCalls(); got != 1 {
		t.Fatalf("invalid writable chunk close calls = %d, want 1", got)
	}
}

func TestFsFileCloseDoesNotBlockOnInFlightRead(t *testing.T) {
	base := afero.NewMemMapFs()
	if err := afero.WriteFile(base, "/stream.txt", []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &blockingReadFS{
		closeTrackingFS: closeTrackingFS{Fs: base},
		started:         make(chan struct{}),
		release:         make(chan struct{}),
	}
	defer backend.releaseRead()

	registry := require.NewRegistry()
	loop := eventloop.NewEventLoop(
		eventloop.EnableConsole(false),
		eventloop.WithRegistry(registry),
	)
	fs.RegisterWithLoop(registry, loop,
		fs.WithFS(backend),
		fs.WithCwd("/"),
		fs.WithStreams(true),
	)
	go loop.StartInForeground()
	t.Cleanup(func() { loop.Stop() })

	setup := make(chan error, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		_, err := vm.RunString(`
			const file = require("fs").openSync("stream.txt");
			file.readable.getReader().read().catch(() => undefined);
			globalThis.__closeFile = () => file.close();
		`)
		setup <- err
	})
	if err := <-setup; err != nil {
		t.Fatal(err)
	}
	select {
	case <-backend.started:
	case <-time.After(time.Second):
		t.Fatal("stream read did not start")
	}

	closeReturned := make(chan error, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		_, err := vm.RunString(`__closeFile()`)
		closeReturned <- err
	})
	select {
	case err := <-closeReturned:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("FsFile.close blocked the event loop behind an in-flight read")
	}

	backend.releaseRead()
	deadline := time.Now().Add(time.Second)
	for backend.closeCalls() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := backend.closeCalls(); got != 1 {
		t.Fatalf("deferred physical close calls = %d, want 1", got)
	}
}

func TestReadFileAbortRejectsBeforeBlockingReadReturns(t *testing.T) {
	base := afero.NewMemMapFs()
	if err := afero.WriteFile(base, "/stream.txt", []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &blockingReadFS{
		closeTrackingFS: closeTrackingFS{Fs: base},
		started:         make(chan struct{}),
		release:         make(chan struct{}),
	}
	defer backend.releaseRead()

	registry := require.NewRegistry()
	loop := eventloop.NewEventLoop(
		eventloop.EnableConsole(false),
		eventloop.WithRegistry(registry),
	)
	fs.RegisterWithLoop(registry, loop, fs.WithFS(backend), fs.WithCwd("/"))
	go loop.StartInForeground()
	t.Cleanup(func() { loop.Stop() })

	setup := make(chan error, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		_, err := vm.RunString(`
			const reason = { marker: "stop" };
			let listener;
			let removals = 0;
			const signal = {
				aborted: false,
				reason: undefined,
				addEventListener(type, fn) { if (type === "abort") listener = fn; },
				removeEventListener(type, fn) {
					if (type === "abort" && fn === listener) removals++;
				},
			};
			globalThis.__abortRead = () => {
				signal.aborted = true;
				signal.reason = reason;
				listener();
			};
			require("fs").readFile("stream.txt", { signal }).then(
				() => { globalThis.__result = "resolved"; },
				(error) => {
					globalThis.__result = String(error === reason) + "|" + removals;
				},
			);
		`)
		setup <- err
	})
	if err := <-setup; err != nil {
		t.Fatal(err)
	}
	select {
	case <-backend.started:
	case <-time.After(time.Second):
		t.Fatal("readFile did not start")
	}

	aborted := make(chan string, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		if _, err := vm.RunString(`__abortRead()`); err != nil {
			aborted <- "error:" + err.Error()
			return
		}
		loop.SetTimeout(func(vm *goja.Runtime) {
			aborted <- vm.Get("__result").String()
		}, time.Millisecond)
	})
	select {
	case result := <-aborted:
		if result != "true|1" {
			t.Fatalf("abort result = %q", result)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("abort waited for the blocking read")
	}

	backend.releaseRead()
	deadline := time.Now().Add(time.Second)
	for backend.closeCalls() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := backend.closeCalls(); got != 1 {
		t.Fatalf("readFile close calls after abort = %d, want 1", got)
	}
}

func TestWriteFileStreamDoesNotBlockEventLoop(t *testing.T) {
	backend := &blockingWriteFS{
		closeTrackingFS: closeTrackingFS{Fs: afero.NewMemMapFs()},
		started:         make(chan struct{}),
		release:         make(chan struct{}),
	}
	defer backend.releaseWrite()

	registry := require.NewRegistry()
	loop := eventloop.NewEventLoop(
		eventloop.EnableConsole(false),
		eventloop.WithRegistry(registry),
	)
	fs.RegisterWithLoop(registry, loop,
		fs.WithFS(backend),
		fs.WithCwd("/"),
		fs.WithStreams(true),
	)
	go loop.StartInForeground()
	t.Cleanup(func() { loop.Stop() })

	setup := make(chan error, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		_, err := vm.RunString(`
			globalThis.__startWrite = () => {
				const { ReadableStream } = require("streams");
				const source = new ReadableStream({
					start(controller) {
						controller.enqueue(new Uint8Array([65]));
						controller.close();
					},
				});
				require("fs").writeFile("stream.txt", source).then(
					() => { globalThis.__result = "done"; },
					(error) => { globalThis.__result = "error:" + error; },
				);
			};
		`)
		setup <- err
	})
	if err := <-setup; err != nil {
		t.Fatal(err)
	}
	loop.RunOnLoop(func(vm *goja.Runtime) {
		_, _ = vm.RunString(`__startWrite()`)
	})
	select {
	case <-backend.started:
	case <-time.After(time.Second):
		backend.releaseWrite()
		t.Fatal("stream write did not start")
	}

	loopResponsive := make(chan struct{}, 1)
	loop.RunOnLoop(func(*goja.Runtime) { loopResponsive <- struct{}{} })
	responsive := false
	select {
	case <-loopResponsive:
		responsive = true
	case <-time.After(200 * time.Millisecond):
	}

	backend.releaseWrite()
	if !responsive {
		t.Fatal("stream write blocked the event loop")
	}
	done := make(chan string, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		loop.SetTimeout(func(vm *goja.Runtime) {
			done <- vm.Get("__result").String()
		}, 10*time.Millisecond)
	})
	select {
	case result := <-done:
		if result != "done" {
			t.Fatalf("stream write result = %q", result)
		}
	case <-time.After(time.Second):
		t.Fatal("stream write did not finish")
	}
	if got := backend.closeCalls(); got != 1 {
		t.Fatalf("stream write close calls = %d, want 1", got)
	}
}

func TestReadDirDoesNotBlockEventLoop(t *testing.T) {
	base := afero.NewMemMapFs()
	if err := base.MkdirAll("/dir", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(base, "/dir/value.txt", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &blockingReaddirFS{
		Fs:      base,
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	defer backend.releaseReadDir()

	registry := require.NewRegistry()
	loop := eventloop.NewEventLoop(
		eventloop.EnableConsole(false),
		eventloop.WithRegistry(registry),
	)
	fs.RegisterWithLoop(registry, loop, fs.WithFS(backend), fs.WithCwd("/"))
	go loop.StartInForeground()
	t.Cleanup(func() { loop.Stop() })

	setup := make(chan error, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		_, err := vm.RunString(`
			globalThis.__startReadDir = () => {
				const iterator = require("fs").readDir("dir");
				iterator.next().then((item) => {
					globalThis.__result = item.value.name;
				});
			};
		`)
		setup <- err
	})
	if err := <-setup; err != nil {
		t.Fatal(err)
	}
	loop.RunOnLoop(func(vm *goja.Runtime) {
		_, _ = vm.RunString(`__startReadDir()`)
	})
	select {
	case <-backend.started:
	case <-time.After(time.Second):
		backend.releaseReadDir()
		t.Fatal("readDir did not start")
	}

	loopResponsive := make(chan struct{}, 1)
	loop.RunOnLoop(func(*goja.Runtime) { loopResponsive <- struct{}{} })
	responsive := false
	select {
	case <-loopResponsive:
		responsive = true
	case <-time.After(200 * time.Millisecond):
	}
	backend.releaseReadDir()
	if !responsive {
		t.Fatal("readDir blocked the event loop")
	}

	done := make(chan string, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		loop.SetTimeout(func(vm *goja.Runtime) {
			done <- vm.Get("__result").String()
		}, 10*time.Millisecond)
	})
	if result := <-done; result != "value.txt" {
		t.Fatalf("readDir result = %q", result)
	}
}

func TestWriteFileStreamAbortRejectsBeforeBlockingWriteReturns(t *testing.T) {
	backend := &blockingWriteFS{
		closeTrackingFS: closeTrackingFS{Fs: afero.NewMemMapFs()},
		started:         make(chan struct{}),
		release:         make(chan struct{}),
	}
	defer backend.releaseWrite()

	registry := require.NewRegistry()
	loop := eventloop.NewEventLoop(
		eventloop.EnableConsole(false),
		eventloop.WithRegistry(registry),
	)
	fs.RegisterWithLoop(registry, loop,
		fs.WithFS(backend),
		fs.WithCwd("/"),
		fs.WithStreams(true),
	)
	go loop.StartInForeground()
	t.Cleanup(func() { loop.Stop() })

	setup := make(chan error, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		_, err := vm.RunString(`
			const reason = { marker: "stop-stream" };
			const listeners = [];
			let removals = 0;
			const signal = {
				aborted: false,
				reason: undefined,
				addEventListener(type, fn) { if (type === "abort") listeners.push(fn); },
				removeEventListener(type, fn) {
					if (type !== "abort") return;
					const index = listeners.indexOf(fn);
					if (index >= 0) listeners.splice(index, 1);
					removals++;
				},
			};
			const source = new (require("streams").ReadableStream)({
				start(controller) { controller.enqueue(new Uint8Array([65])); },
				cancel(value) { globalThis.__sourceCancelled = value === reason; },
			});
			globalThis.__abortWrite = () => {
				signal.aborted = true;
				signal.reason = reason;
				for (const listener of listeners.slice()) listener();
			};
			require("fs").writeFile("stream.txt", source, { signal }).then(
				() => { globalThis.__abortResult = "resolved"; },
				(error) => {
					globalThis.__abortResult = String(error === reason) + "|" + String(removals > 0);
				},
			);
		`)
		setup <- err
	})
	if err := <-setup; err != nil {
		t.Fatal(err)
	}
	select {
	case <-backend.started:
	case <-time.After(time.Second):
		t.Fatal("stream write did not start")
	}

	aborted := make(chan string, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		if _, err := vm.RunString(`__abortWrite()`); err != nil {
			aborted <- "error:" + err.Error()
			return
		}
		loop.SetTimeout(func(vm *goja.Runtime) {
			aborted <- vm.Get("__abortResult").String()
		}, time.Millisecond)
	})
	select {
	case result := <-aborted:
		if result != "true|true" {
			t.Fatalf("stream abort result = %q", result)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("stream abort waited for the blocking write")
	}

	backend.releaseWrite()
	cleanup := make(chan bool, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		loop.SetTimeout(func(vm *goja.Runtime) {
			cleanup <- vm.Get("__sourceCancelled").ToBoolean()
		}, 20*time.Millisecond)
	})
	select {
	case cancelled := <-cleanup:
		if !cancelled {
			t.Fatal("stream source was not cancelled with the abort reason")
		}
	case <-time.After(time.Second):
		t.Fatal("stream abort cleanup did not finish")
	}
	if got := backend.closeCalls(); got != 1 {
		t.Fatalf("stream abort close calls = %d, want 1", got)
	}
}

func TestWriteFileKeepsPartialDataAndClosesOnError(t *testing.T) {
	backend := &partialErrorWriteFS{closeTrackingFS: closeTrackingFS{Fs: afero.NewMemMapFs()}}
	result := runFSAPIScriptWithBackend(t, backend, "/", `
		const fs = require("fs");
		fs.writeFile("partial.txt", new Uint8Array([65, 66])).then(
			() => { globalThis.__result = "resolved"; },
			() => { globalThis.__result = "error|" + fs.readTextFileSync("partial.txt"); },
		);
	`)
	if result != "error|A" {
		t.Fatalf("partial write result = %q", result)
	}
	if got := backend.closeCalls(); got != 1 {
		t.Fatalf("partial write close calls = %d, want 1", got)
	}
}

func TestFsFileReadableUsesConfiguredChunkSize(t *testing.T) {
	base := afero.NewMemMapFs()
	if err := afero.WriteFile(base, "/chunks.txt", []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &closeTrackingFS{Fs: base}
	result := runFSAPIScriptWithBackend(t, backend, "/", `
		const reader = require("fs").openSync("chunks.txt").readable.getReader();
		const lengths = [];
		function pump() {
			return reader.read().then((item) => {
				if (item.done) return;
				lengths.push(item.value.byteLength);
				return pump();
			});
		}
		pump().then(() => { globalThis.__result = lengths.join(","); });
	`, fs.WithStreamChunkSize(2))
	if result != "2,2,1" {
		t.Fatalf("stream chunk lengths = %q", result)
	}
	if got := backend.closeCalls(); got != 1 {
		t.Fatalf("chunked readable close calls = %d, want 1", got)
	}
}

func TestWebStreamsRoundTripOnOsFs(t *testing.T) {
	directory := t.TempDir()
	result := runFSAPIScriptWithBackend(t, afero.NewOsFs(), directory, `
		const fs = require("fs");
		const { ReadableStream } = require("streams");
		const source = new ReadableStream({
			start(controller) {
				controller.enqueue(new Uint8Array([65, 66]));
				controller.enqueue(new Uint8Array([67]));
				controller.close();
			},
		});
		fs.writeFile("host.bin", source).then(() => {
			const reader = fs.openSync("host.bin").readable.getReader();
			const bytes = [];
			function pump() {
				return reader.read().then((item) => {
					if (item.done) return;
					bytes.push(...item.value);
					return pump();
				});
			}
			return pump().then(() => { globalThis.__result = bytes.join(","); });
		});
	`)
	if result != "65,66,67" {
		t.Fatalf("OsFs stream round trip = %q", result)
	}
}

type closeTrackingFS struct {
	afero.Fs
	mu     sync.Mutex
	closes int
}

func (fsys *closeTrackingFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	file, err := fsys.Fs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return &closeTrackingFile{File: file, owner: fsys}, nil
}

func (fsys *closeTrackingFS) closeCalls() int {
	fsys.mu.Lock()
	defer fsys.mu.Unlock()
	return fsys.closes
}

type closeTrackingFile struct {
	afero.File
	owner *closeTrackingFS
}

func (file *closeTrackingFile) Close() error {
	file.owner.mu.Lock()
	file.owner.closes++
	file.owner.mu.Unlock()
	return file.File.Close()
}

type blockingReadFS struct {
	closeTrackingFS
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func (fsys *blockingReadFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	file, err := fsys.closeTrackingFS.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return &blockingReadFile{File: file, owner: fsys}, nil
}

func (fsys *blockingReadFS) Open(name string) (afero.File, error) {
	file, err := fsys.closeTrackingFS.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return &blockingReadFile{File: file, owner: fsys}, nil
}

func (fsys *blockingReadFS) releaseRead() {
	fsys.releaseOnce.Do(func() { close(fsys.release) })
}

type blockingReadFile struct {
	afero.File
	owner *blockingReadFS
}

func (file *blockingReadFile) Read(p []byte) (int, error) {
	file.owner.startedOnce.Do(func() { close(file.owner.started) })
	<-file.owner.release
	return file.File.Read(p)
}

type blockingWriteFS struct {
	closeTrackingFS
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func (fsys *blockingWriteFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	file, err := fsys.closeTrackingFS.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return &blockingWriteFile{File: file, owner: fsys}, nil
}

func (fsys *blockingWriteFS) releaseWrite() {
	fsys.releaseOnce.Do(func() { close(fsys.release) })
}

type blockingWriteFile struct {
	afero.File
	owner *blockingWriteFS
}

func (file *blockingWriteFile) Write(p []byte) (int, error) {
	file.owner.startedOnce.Do(func() { close(file.owner.started) })
	<-file.owner.release
	return file.File.Write(p)
}

type blockingReaddirFS struct {
	afero.Fs
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func (fsys *blockingReaddirFS) Open(name string) (afero.File, error) {
	file, err := fsys.Fs.Open(name)
	if err != nil {
		return nil, err
	}
	return &blockingReaddirFile{File: file, owner: fsys}, nil
}

func (fsys *blockingReaddirFS) releaseReadDir() {
	fsys.releaseOnce.Do(func() { close(fsys.release) })
}

type blockingReaddirFile struct {
	afero.File
	owner *blockingReaddirFS
}

func (file *blockingReaddirFile) Readdir(count int) ([]os.FileInfo, error) {
	file.owner.startedOnce.Do(func() { close(file.owner.started) })
	<-file.owner.release
	return file.File.Readdir(count)
}

type partialErrorWriteFS struct {
	closeTrackingFS
}

func (fsys *partialErrorWriteFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	file, err := fsys.closeTrackingFS.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return &partialErrorWriteFile{File: file}, nil
}

type partialErrorWriteFile struct {
	afero.File
	wrote bool
}

func (file *partialErrorWriteFile) Write(p []byte) (int, error) {
	if file.wrote || len(p) == 0 {
		return 0, errors.New("forced write failure")
	}
	file.wrote = true
	n, err := file.File.Write(p[:1])
	if err != nil {
		return n, err
	}
	return n, errors.New("forced write failure")
}
