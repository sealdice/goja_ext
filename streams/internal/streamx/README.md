# Node Classic Streams Polyfill (streamx)

This directory vendors an esbuild bundle of the streamx-based Node classic
streams facade for the Goja `stream` module.

## Sources

- Engine: `streamx@2.28.0` (MIT)
- Buffer abstraction: `b4a` (Apache-2.0)
- FIFO queue: `fast-fifo` (MIT)
- Text decoding: `text-decoder` (Apache-2.0)
- Facade/bridge: adapted from `bare-stream@2.13.3` (Apache-2.0) — see
  `facade.js` / `bridge.js` headers for the adaptation notes
- `events-universal` (streamx dependency) is aliased to `events` and provided
  by the Go layer as the canonical `events` module

## Bundle

- Bundler: `esbuild`
- Target: ES2015, CJS, browser platform
- External: `events` (canonical module injected by Go)
- Build: `npm run build:node-streams` (see `scripts/build-node-streams.mjs`)
- SHA-256 (bundle.js): `2e4288765b44147b52632fb636f7e9e32f5b2e7ec6beee8884a454c343afc358`

## Semantic differences vs readable-stream (Node)

- String chunks are converted to `Uint8Array` on write/push (bare-stream
  behavior). Node code that writes strings must decode via
  `Buffer.from(chunk).toString(encoding)`.
- `read(0)` is not supported (returns `null`).
- Default destroy error is `STREAM_DESTROYED` rather than
  `ERR_STREAM_DESTROYED`.
- Stream state lives in the `_duplexState` bitmask (streamx layout), not
  `_readableState`/`_writableState`.
