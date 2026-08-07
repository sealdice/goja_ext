# Afero-backed Deno FS and Streams Extension Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the existing Goja Web Streams embedding and implement a Deno-style `30_fs.js` API backed by Afero, while keeping Node classic streams and optional filesystem capabilities removable.

**Architecture:** Afero is wrapped by a Go FS Core that owns paths, logical cwd, handles, locking, errors, and asynchronous execution. The Deno facade is the first JS API and uses canonical WHATWG Streams. Text streams and Node classic streams live in independent stream layers; link/watch/lock/terminal capabilities live under `fs/extra`.

**Tech Stack:** Go 1.23+, Goja, goja_nodejs require/eventloop, Afero, embedded ES2015 JavaScript, `web-streams-polyfill@4.3.0`, optional `readable-stream@4.7.0`.

---

## Task 1: Lock the revised design and dependency boundaries

**Files:**
- Modify: `docs/afero-backed-fs-design.md`
- Modify: `功能规格说明书.md`
- Create: `docs/superpowers/plans/2026-08-07-afero-backed-fs-and-streams.md`

- [x] **Step 1: Record the Deno-first layering**

  Keep `fs/core`, `fs`, and `fs/extra` as separate responsibilities. Node `fs` is a later facade over `fs/core`; it must not call the Deno JS facade.

- [x] **Step 2: Record stream expansion**

  Add `TextEncoderStream` and `TextDecoderStream` to the Web Streams surface. Keep Node classic streams under `streams/node` and permit that module to remain disabled if the vendored polyfill cannot run in Goja.

- [x] **Step 3: Record optional Afero capabilities**

  Keep links, hard links, watcher, locking, and terminal methods out of the core package. Detect them from explicit interfaces/capabilities and install only the selected extension.

- [x] **Step 4: Verify the documents**

  Run:

  ```bash
  git diff --check
  rg -n "Deno|extra|TextEncoderStream|node:stream" docs/afero-backed-fs-design.md 功能规格说明书.md
  ```

  Expected: no whitespace errors and all four boundaries are present.

## Task 2: Add UTF-8 text conversion streams

**Files:**
- Create: `streams/text_streams.js`
- Modify: `streams/module.go`
- Create: `streams/text.go`
- Modify: `streams/module_test.go`

- [x] **Step 1: Write failing constructor/export tests**

  Add tests that require `streams`, check `TextEncoderStream` and `TextDecoderStream` exports, and verify that each instance exposes `.readable`, `.writable`, `.encoding`, `.fatal`, and `.ignoreBOM` with the expected UTF-8 defaults.

- [x] **Step 2: Run the focused tests and verify the expected failure**

  ```bash
  go test ./streams -run 'TestText'
  ```

  Expected: failure because the two constructors are not exported yet.

- [x] **Step 3: Implement the Go UTF-8 bridge**

  Use `buffer.DecodeBytes` for input byte extraction and `buffer.WrapBytes` for encoded output. Add a Go callback that accepts a byte chunk and pending bytes, decodes complete UTF-8 sequences, returns `{ text, pending }`, and flushes incomplete sequences with U+FFFD. Keep the callback on the runtime goroutine.

- [x] **Step 4: Implement the JS TransformStream wrappers**

  Build `TextEncoderStream` and `TextDecoderStream` on the existing canonical `TransformStream`. `TextDecoderStream` must retain incomplete UTF-8 suffixes between transform calls and emit them with replacement semantics on flush. Reject labels other than `utf-8`/`utf8`; do not pretend to implement other encodings.

- [x] **Step 5: Add the constructors to runtime exports and globals**

  Cache the text exports per runtime under a private symbol, merge them with the polyfill exports, and install both constructors from `Enable`. Preserve the existing 13 constructor identities.

- [x] **Step 6: Add behavior tests**

  Cover ASCII, multibyte UTF-8, split multibyte sequences, flush replacement, invalid labels, close/error propagation, and backpressure through `TransformStream`.

- [x] **Step 7: Run the focused tests**

  ```bash
  go test ./streams -run 'TestText'
  ```

  Expected: PASS.

## Task 3: Attempt an independent Node classic streams facade

**Files:**
- Create: `streams/node/README.md`
- Create: `streams/node/module.go`
- Create: `streams/node/module_test.go`
- Create: `streams/internal/nodepolyfill/README.md`
- Create: `streams/internal/nodepolyfill/LICENSE`
- Create: `streams/internal/nodepolyfill/readable-stream.js`
- Modify: `go.mod`
- Modify: `go.sum`

- [x] **Step 1: Add module surface tests**

  Test `require("stream")` and `require("node:stream")`, constructor identity, `Readable.from`, `Transform`, `PassThrough`, `pipeline`, `finished`, and EventEmitter events. Keep tests in `streams/node` so they can be skipped without weakening `streams` Web Streams tests.

- [x] **Step 2: Fetch and inspect the mature implementation**

  Vendor `readable-stream@4.7.0` plus its runtime dependencies as one Goja-parseable bundle. Record the exact version, source package, license, integrity hash, and bundling command in `streams/internal/nodepolyfill/README.md`.

- [x] **Step 3: Compile the bundle with Goja**

  Compile once and cache exports per runtime, following `streams/module.go`. Expose only the Node classic stream exports that the bundle provides; do not install them globally.

- [x] **Step 4: Run the focused Node tests**

  ```bash
  go test ./streams/node
  ```

  Expected: PASS if the bundle parses and its event/backpressure behavior works.

- [x] **Step 5: Isolate incompatibility**

  If the bundle requires unsupported syntax or host globals, leave `streams/node` unregistered by default, document the exact failing dependency in `streams/node/README.md`, and keep the Web Streams build independent. The Deno FS implementation must not import this package.

## Task 4: Introduce Afero and implement FS Core

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `fs/core.go`
- Create: `fs/paths.go`
- Create: `fs/errors.go`
- Create: `fs/core_test.go`

- [x] **Step 1: Write failing Afero core tests**

  Use `afero.NewMemMapFs()` to test logical cwd, path joining, create/open/read/write, directory operations, rename/remove, stat, truncation, and closed-handle errors.

- [x] **Step 2: Run the focused tests and verify the expected failure**

  ```bash
  go test ./fs -run 'TestCore'
  ```

  Expected: package or symbol failures because `fs` does not exist.

- [x] **Step 3: Add a direct Afero dependency**

  Add the selected stable Afero version to `go.mod`, run `go mod tidy`, and verify that no unrelated module versions change.

- [x] **Step 4: Implement FS Core**

  Define an internal service containing:

  ```go
  type Core struct {
      backend afero.Fs
      cwd     string
      chunkSize int
      handles *HandleTable
  }
  ```

  Add path normalization, logical cwd, `afero.File` handle records with mutex and closed state, open flag translation, `os.FileInfo` conversion, and error-to-code mapping. Keep all runtime-independent operations in this layer.

- [x] **Step 5: Run core tests**

  ```bash
  go test ./fs -run 'TestCore'
  ```

  Expected: PASS.

## Task 5: Implement the Deno path API

**Files:**
- Create: `fs/module.go`
- Create: `fs/api.go`
- Create: `fs/api_test.go`

- [x] **Step 1: Write failing Deno API tests**

  Test synchronous and Promise variants of `cwd`, `chdir`, `mkdir`, `readFile`, `readTextFile`, `writeFile`, `writeTextFile`, `copyFile`, `rename`, `remove`, `stat`, `truncate`, `chmod`, `chown`, `utime`, `makeTempFile`, `makeTempDir`, `readDir`, and `open`.

- [x] **Step 2: Run the focused tests and verify the expected failure**

  ```bash
  go test ./fs -run 'TestDeno'
  ```

  Expected: failure because the Deno facade is not registered.

- [x] **Step 3: Implement module registration and options**

  Add `Enable`, `EnableWithLoop`, `RequireWithOptions`, `RequirePromisesWithOptions`, `RequireWithLoop`, `RequirePromisesWithLoop`, `RegisterWithOptions`, `RegisterWithLoop`, `WithFS`, `WithCwd`, `WithStreams`, `WithStreamChunkSize`, and `WithExtraCapabilities`. Register `fs`, `node:fs`, `fs/promises`, and `node:fs/promises` through registry-local helpers when options are supplied.

- [x] **Step 4: Implement synchronous path methods**

  Convert path/URL-like inputs to logical backend paths. Return byte data as `Uint8Array`/Buffer-compatible values, return text as UTF-8 strings, and expose portable stat/read-dir objects with stable methods.

- [x] **Step 5: Implement Promise methods**

  Run only pure Go requests in worker goroutines. Settle promises through `EventLoop.RunOnLoop`, preserve JS error values, and close all temporary files on success, rejection, or stream cancellation.

- [x] **Step 6: Run Deno API tests**

  ```bash
  go test ./fs -run 'TestDeno'
  ```

  Expected: PASS.

## Task 6: Implement `FsFile` and Web Streams file bridge

**Files:**
- Create: `fs/handles.go`
- Create: `fs/streams.go`
- Create: `fs/streams_test.go`
- Modify: `streams/integration.go`

- [x] **Step 1: Write failing handle/stream tests**

  Test `FsFile.read`, `write`, `seek`, `truncate`, `stat`, `sync`, `syncData`, `close`, `.readable`, `.writable`, EOF close, read errors, write errors, cancellation, and shared offset serialization.

- [x] **Step 2: Run the focused tests and verify the expected failure**

  ```bash
  go test ./fs -run 'TestFsFile|TestFileStream'
  ```

  Expected: failure because handle and stream adapters are not implemented.

- [x] **Step 3: Implement the handle wrapper**

  Wrap a core handle in a JS object with Deno method names and stable closed-handle checks. Use `streams.NewReadableStream`, `streams.NewWritableStream`, and `streams.ConsumeReadableStream`; never duplicate the Web Streams state machine.

- [x] **Step 4: Implement `writeFile` stream input**

  Detect canonical `ReadableStream`, open the target with the requested options, consume chunks sequentially, convert strings/typed arrays/ArrayBuffers to bytes, and close the owned file in every terminal path.

- [x] **Step 5: Route `writeTextFile` through `TextEncoderStream`**

  Use the new text stream for stream inputs. For non-stream input, encode UTF-8 directly in Go. Preserve abort errors and backpressure.

- [x] **Step 6: Run stream integration tests**

  ```bash
  go test ./fs -run 'TestFsFile|TestFileStream'
  ```

  Expected: PASS.

## Task 7: Defer optional filesystem capabilities under `fs/extra`

**Files:**
- Create: `fs/extra/capabilities.go`
- Future: `fs/extra/links.go`
- Future: `fs/extra/locking.go`
- Future: `fs/extra/terminal.go`
- Future: `fs/extra/watch.go`
- Future: `fs/extra/extra_test.go`

- [x] **Step 1: Create removable extension placeholder**

  Add `fs/extra/capabilities.go` as a package-level TODO. The core `fs` package does not import `fs/extra`, so embedders can delete the directory without changing core Deno FS behavior.

- [x] **Step 2: Keep symlink/hardlink/watch/lock/terminal out of core**

  Core exports omit `lstat`, `readlink`, `symlink`, `link`, `realPath`, `umask`, file locks, watch, raw mode, and terminal handling. These remain explicitly optional rather than partially emulated through `Stat`.

- [ ] **Future Step 3: Write capability detection tests**

  Use `MemMapFs`, `OsFs`, and small stubs implementing `afero.Lstater`, `afero.LinkReader`, `afero.Linker`, or watcher interfaces. Verify that unsupported methods are omitted or return `ENOSYS` according to the selected option.

- [ ] **Future Step 4: Implement capability interfaces**

  Detect each optional interface independently. Do not infer symlink support from `Stat`; do not expose `realpath` unless the backend explicitly provides resolution semantics.

- [ ] **Future Step 5: Implement and test selected extensions**

  Add link, lock, terminal, watcher, and realpath files only for capabilities the project decides to keep.

## Task 8: Verification and regression pass

**Files:**
- Modify: `功能规格说明书.md`
- Modify: `docs/afero-backed-fs-design.md`
- Modify: `streams/internal/nodepolyfill/README.md` if classic streams are deferred

- [x] **Step 1: Run formatting and static checks**

  ```bash
  gofmt -w streams fs
  go test ./...
  go test -race ./...
  go vet ./...
  go build ./...
  git diff --check
  ```

- [x] **Step 2: Run focused backend matrix**

  Verify core Deno tests against `MemMapFs`, `ReadOnlyFs`, `BasePathFs`, and `OsFs` where the behavior is portable. Keep chmod/chown/sync/link/watch assertions platform-aware.

- [x] **Step 3: Update completion checklists**

  Mark only verified capabilities as complete. Keep WPT full-runner and unsupported optional capabilities explicitly listed as pending or backend-dependent.
