# Embedded fetch-event-source

- Upstream: `@microsoft/fetch-event-source@2.0.1`
- Bundle SHA-256: `acf32e1a5ee13bfb2899b50e2f73c4286e2ddcf2746503de6163e6ee92fcd3c3`
- Rebuild: `npm run build:fetch-event-source`

The esbuild inject shim supplies private headless `window` and `document` objects plus the runtime's canonical `AbortController` and `TextDecoder`. The pinned parser is patched to ignore comment-only blocks while preserving explicit empty `data:` events.
