# Embedded bare-fetch facade

This directory contains the generated JavaScript API facade used by the Go
`fetch` package.

- `bare-fetch`: 3.2.0
- `bare-form-data`: 1.2.2
- `bare-mime`: 1.0.0 (resolved by the lockfile)
- build entry: `facade.js`
- generated output: `bundle.js`

`bundle.js` includes the bare-fetch Headers, Body, Request, Response and error
classes, plus bare-form-data and bare-mime. It deliberately excludes
bare-http1, bare-https, bare-performance and bare-zlib. Network I/O is supplied
by the Go `dispatch()` host function.

The build aliases `bare-stream/web`, `bare-url` and `bare-buffer` to host shims.
At runtime those shims use goja_ext's canonical Web Streams, URL and Buffer
constructors, so the facade does not install duplicate implementations.

Regenerate from the repository root with:

```sh
npm run build:fetch
```

The build prints the output SHA-256. The upstream Apache-2.0 license texts are
stored next to the bundle.
