# Node Classic Streams Facade

This package registers optional `stream` / `node:stream` modules backed by a
bundled **streamx** engine (see `streams/internal/streamx`).

- The facade reuses the canonical `events` module as the EventEmitter base, so
  `require("events").EventEmitter` and stream objects share constructor
  identity.
- Web `<->` classic adapters (`Readable.toWeb/fromWeb`, `Writable.toWeb/fromWeb`,
  `Duplex.toWeb/fromWeb`) are implemented against the canonical
  `web-streams-polyfill` (`stream/web`). Web Streams are initialized lazily on
  the first adapter call; loading `stream` alone does not initialize them.
- Events, the private microtask function, and the lazy Web Streams provider are
  passed directly to the embedded bundle. Initialization does not write
  `self`, `queueMicrotask`, magic string globals, or private Symbols to the JS
  global object.
- It is separate from `streams`, `stream/web`, and the `fs` module. The Deno
  style FS implementation uses WHATWG Streams directly and does not import this
  package.
- The package does not import `abort` or `buffer`. Applications that use those
  modules must register/import them explicitly.
- Semantic differences vs Node's readable-stream are documented in
  `streams/internal/streamx/README.md` (string chunks arrive as `Uint8Array`;
  decode with `Buffer.from(chunk).toString(encoding)`).
