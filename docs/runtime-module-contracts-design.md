# Runtime-bound module contracts

**Date:** 2026-08-07

## Context

The repository currently has useful implementations for most modules, but several
cross-module contracts are implicit. In particular, asynchronous modules accept an
arbitrary `*goja.Runtime` and `*eventloop.EventLoop`, constructors are recreated by
different installation paths, filesystem configuration is cached with first-call-wins
semantics, and some optional capabilities are emulated incorrectly.

This design makes those contracts explicit while preserving the existing public entry
points where they can be made safe.

## Compatibility policy

Existing Go and JavaScript entry points remain available. New runtime-bound APIs are
the preferred path. Existing APIs either delegate to the new implementation or return
an explicit error when their old argument combination cannot be correct. No API may
silently settle a Promise on a different runtime, silently ignore a conflicting module
configuration, or claim an unavailable capability.

## Runtime host

A small `runtimehost` package owns per-runtime host state:

- the runtime identity;
- an optional `Scheduler` whose `Runtime()` is the same runtime;
- a logical working-directory provider;
- canonical module values and private Go state keyed by stable package-owned keys.

`eventloop.EventLoop` implements `runtimehost.Scheduler`, exposes its runtime for
identity checks, and binds itself when constructed. Runtime operations remain confined
to the event-loop callback. Modules validate ownership at installation time and never
accept values created by another runtime.

The canonical registry is not a JavaScript global property. It replaces observable
magic globals and duplicated private symbols. A runtime can be detached explicitly so
long-lived embedders can release registry state.

## Canonical module values

Every installation path for a module returns the same constructor or exports object in
one runtime:

- `abort.Enable()` and `require("abort")` share `AbortController` and `AbortSignal`;
- `fetch.Enable()` and `require("fetch")` share `Headers`, `Request`, `Response`, and
  `FormData`;
- network responses use the canonical `Response.prototype` and pass `instanceof`;
- Web Streams, Node streams, and Events obtain dependencies from the runtime registry,
  without writing temporary global variables;
- all FS loaders and the Deno global facade share one configured `Core` per runtime.

Reconfiguration is idempotent only when it is equivalent. A conflicting backend,
working directory, scheduler, stream mode, or chunk size returns an error that names
the conflicting setting.

## Scheduling, timers, and abort

`AbortSignal.timeout()` requires a runtime scheduler. Without one it throws a clear
configuration error instead of returning a signal that can never abort. Timer module
exports resolve their target dynamically and `timers/promises` implements `signal`,
`ref`, and iterator cancellation semantics. Fetch observes already-aborted signals,
removes listeners at completion, and propagates the original abort reason.

## Filesystem boundaries

The neutral Afero-backed `Core` remains reusable. Installation is separated into:

- a Deno-style facade used by `fs.Enable*` and containing only Deno operations;
- a Node facade used by `require("fs")` and `require("fs/promises")`;
- optional capabilities for links, `lstat`, and `realpath`.

The Deno facade does not initialize or import Node classic streams. Node stream methods
are installed only when Node streams are enabled and available. `WithStreams(false)`
removes every stream surface, including `createReadStream` and `createWriteStream`.

`lstat` uses an actual lstat capability. `realpath` follows links with loop detection
and is exposed only when the backend supplies the required link interfaces. Unsupported
capabilities return `ENOSYS`; they never delegate to `stat` or return a merely cleaned
path. `WithExtraCapabilities` installs concrete capabilities and is not a no-op.

The FS core may act as the runtime working-directory provider. Consequently `fs.chdir`,
`process.cwd/process.chdir`, and relative `path.resolve` observe the same logical state
without mutating the host process working directory.

## Streams and networking

Node streams initialize from explicit canonical Events and optional Web Streams values.
`addAbortSignal` consumes the supplied signal structurally, so the package does not
import or initialize Abort. Requiring Node streams does not initialize Web Streams
unless a Web adapter is actually requested. The Web text stream layer emits and
consumes native `Uint8Array` values and does not import Node Buffer.

Fetch and WebSocket use the runtime-bound scheduler. WebSocket dialer, TLS settings,
logger, and connection manager are instance-scoped and injectable. TLS verification is
enabled by default. A caller must explicitly provide an insecure TLS configuration to
disable it.

## Local compatibility fixes

- Console exports `trace` with a stack-bearing message.
- Numeric Buffer construction is implemented with validated size semantics.
- Events preserves once wrappers in `rawListeners`, honors per-instance max listeners,
  and returns the emitter from `setMaxListeners`.
- Path exposes string `sep`/`delimiter`, uses the host platform for the default export,
  and reads the runtime cwd dynamically.
- StringDecoder retains incomplete UTF-8, UTF-16LE/UCS-2, and base64 units across writes.
- URL origin includes a non-default port.
- Structured clone either creates an independent supported value or throws a
  `DataCloneError`; it never returns the original object as a fallback.

## Verification

Each behavior is introduced by a failing focused test. Completion requires:

```text
go test -count=1 ./...
go test -count=1 -race ./...
go vet ./...
go build ./...
golangci-lint run ./...
git diff --check
```

Integration tests use local HTTP, TLS, WebSocket, and temporary filesystem servers only.
Runnable examples cover a composed runtime, fetch cancellation, injected WebSocket TLS,
shared cwd, FS capabilities, and streams. Documentation must state supported subsets and
configuration errors precisely.
