# Repository-local JavaScript declarations

Add `types/index.d.ts` to the `files` or `include` list of a plugin TypeScript
project. The declarations describe goja_ext's implemented module surface and
assume the TypeScript `DOM` library for Web Fetch, Streams, Abort and WebSocket
types.

Run `npm run test:types` from the repository root to check the declarations.
