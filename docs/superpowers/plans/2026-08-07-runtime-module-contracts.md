# Runtime-bound Module Contracts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Repair all audited module completeness and dependency problems, add complete focused and feasible integration coverage, and provide runnable examples.

**Architecture:** Add a runtime-owned host registry for scheduling, environment, and canonical identities. Migrate modules to explicit dependencies in dependency order, split Deno and Node FS installation surfaces, and make optional behavior capability-driven. Preserve existing entry points as checked compatibility wrappers.

**Tech Stack:** Go 1.23+, Goja, Afero, Resty, Gorilla WebSocket, embedded JavaScript polyfills, local `httptest` integration servers.

---

## Phase 1: Runtime ownership and host state

**Files:**
- Create: `runtimehost/host.go`
- Create: `runtimehost/host_test.go`
- Modify: `eventloop/eventloop.go`
- Modify: `eventloop/eventloop_test.go`

1. Add failing tests for canonical state, scheduler ownership, logical cwd, detach, and mismatched scheduler rejection.
2. Implement `runtimehost.Host`, `Scheduler`, canonical value/state access, cwd provider, and cleanup.
3. Make `EventLoop` implement and bind the scheduler contract; expose read-only runtime identity.
4. Verify with `go test ./runtimehost ./eventloop -count=1 -race`.

## Phase 2: Canonical Abort, Fetch, and FS state

**Files:**
- Modify: `abort/module.go`, `abort/abort.go`, `abort/abort_test.go`
- Modify: `fetch/module.go`, `fetch/classes.go`, `fetch/fetch.go`, `fetch/fetch_test.go`
- Modify: `fs/module.go`, `fs/core.go`, `fs/api_test.go`

1. Add identity tests across global and CommonJS installation paths.
2. Cache canonical constructors/exports through `runtimehost`.
3. Construct fetched responses through the canonical Response constructor/prototype.
4. Make all FS installation paths share one core and reject conflicting options.
5. Verify with `go test ./abort ./fetch ./fs -count=1 -race`.

## Phase 3: Runtime-bound asynchronous APIs

**Files:**
- Modify: `fetch/fetch.go`, `fetch/fetch_test.go`
- Modify: `fs/module.go`, `fs/api.go`, `fs/node.go`, `fs/*_test.go`
- Modify: `websocket/websocket.go`, `websocket/websocket_test.go`

1. Add cross-runtime rejection tests for fetch, FS, and WebSocket.
2. Replace raw loop use with the checked scheduler contract.
3. Cover already-aborted fetch signals and listener removal.
4. Verify local HTTP and WebSocket async integration under `-race`.

## Phase 4: Timers, Abort, and Events behavior

**Files:**
- Modify: `abort/abort.go`, `abort/abort_test.go`
- Modify: `timers/module.go`, `timers/promises.js`, `timers/module_test.go`
- Modify: `events/events.js`, `events/module_test.go`

1. Add failure tests for timeout without a scheduler, timer Promise abort/ref behavior, once wrappers, and max listeners.
2. Require a scheduler for timeout signals and implement cleanup.
3. Make timer exports dynamic and complete the Promise option semantics.
4. Correct EventEmitter wrapper and return-value behavior.
5. Verify with `go test ./abort ./timers ./events -count=1 -race`.

## Phase 5: Streams dependency cleanup

**Files:**
- Modify: `streams/node/module.go`, `streams/node/polyfill.go`, `streams/node/*_test.go`
- Modify: `streams/text.go`, `streams/text_streams.js`, `streams/*_test.go`
- Modify: `streams/node/README.md`, `streams/README.md`

1. Add load-order tests proving no global leakage and no eager Web Streams initialization.
2. Inject canonical Events and Abort through private Go bindings consumed by the bundle.
3. Load Web Streams lazily only for `toWeb/fromWeb` adapters.
4. Replace Buffer use in Web text streams with native Uint8Array.
5. Verify both load orders with `go test ./streams/... -count=1 -race`.

## Phase 6: Filesystem facade and capabilities

**Files:**
- Modify: `fs/module.go`, `fs/api.go`, `fs/node.go`, `fs/core.go`
- Implement: `fs/extra/capabilities.go`
- Add: `fs/extra/capabilities_test.go`
- Modify: `fs/node_test.go`, `fs/api_test.go`, `fs/core_test.go`

1. Add tests that Deno installation does not expose Node methods and `WithStreams(false)` removes every stream API.
2. Separate Deno and Node binding functions and remove the compile-time Node streams dependency from Deno installation.
3. Implement concrete link/lstat/realpath capability adapters with `ENOSYS` and symlink-loop coverage.
4. Make `WithExtraCapabilities` validate and install capabilities.
5. Verify MemMap, read-only, base-path, and OS-backed matrices.

## Phase 7: Shared cwd and path/process correctness

**Files:**
- Modify: `path/module.go`, `path/path_test.go`
- Modify: `process/module.go`, `process/module_test.go`
- Modify: `fs/core.go`, `fs/module.go`, `fs/api_test.go`

1. Add shared-cwd integration tests and platform-default path tests.
2. Route path and process cwd through `runtimehost` and bind the FS provider.
3. Expose string `sep` and `delimiter`; retain explicit `posix` and `win32` objects.
4. Verify with `go test ./path ./process ./fs -count=1 -race`.

## Phase 8: WebSocket host dependencies and TLS

**Files:**
- Modify: `websocket/websocket.go`, `websocket/websocket_test.go`, `websocket/README.md`

1. Add TLS verification failure/success tests using `httptest.NewTLSServer`.
2. Add injectable dialer/TLS/logger/manager options scoped per module instance.
3. Remove insecure defaults and global mutable operational state from new instances; retain deprecated compatibility shims.
4. Verify connection lifecycle, normal/error close, cleanup, and races.

## Phase 9: Module-local completeness fixes

**Files:**
- Modify: `console/console.go`, `console/console_test.go`
- Modify: `buffer/buffer.go`, `buffer/buffer_test.go`
- Modify: `string_decoder/*`, `string_decoder/*_test.go`
- Modify: `url/url.go`, `url/url_test.go`
- Modify: `structuredclone/structuredclone.go`, `structuredclone/structuredclone_test.go`

1. Add focused failing tests for every audited gap.
2. Implement console trace, numeric Buffer construction, decoder carry state, URL origin ports, and strict structured-clone errors.
3. Verify focused packages under `-race`.

## Phase 10: Examples, documentation, and quality gate

**Files:**
- Create: `examples/composed_runtime/main.go`
- Create: `examples/fetch_abort/main.go`
- Create: `examples/fs_runtime/main.go`
- Create: `examples/websocket_tls/main.go`
- Create: `examples/streams/main.go`
- Modify: affected module `README.md` files and root `README.md`
- Fix: all issues reported by `.golangci.yml`

1. Add executable examples backed by local resources and test them with `go run`.
2. Update module contracts, dependency requirements, and supported subset documentation.
3. Resolve all lint findings without suppressing real errors.
4. Run the full verification matrix from the design document and record fresh output.
5. Audit every item in the design against source and test evidence before declaring completion.
