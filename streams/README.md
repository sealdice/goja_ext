# streams 模块

项目统一使用 WHATWG Streams，不提供 Node classic `stream`/`node:stream`。

## 模块名

- `require("streams")`
- `require("stream/web")`
- `require("node:stream/web")`
- `streams.Enable(rt)` 安装全局 Web Streams 构造器。

三个模块入口在同一 runtime 中共享 canonical 构造器。非标准的
`require("node:streams")` 不受支持。

## 能力

- `ReadableStream`（含 byte stream/BYOB、async iterator）、`WritableStream`、
  `TransformStream`、reader/writer/controller、`tee`、`pipeTo`、`pipeThrough`
  和两种 QueuingStrategy。
- `TextEncoder`、`TextDecoder`、`TextEncoderStream`、`TextDecoderStream`。
- Go 集成：`NewReadableStream`、`NewWritableStream`、`IsReadableStream`、
  `ConsumeReadableStream`，供 fs、fetch、Cloudflare KV 等模块复用。

实现基于嵌入的 `web-streams-polyfill@4.3.0`，字节块使用原生 `Uint8Array`，
不依赖 Node `Buffer`。
