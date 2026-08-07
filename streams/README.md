# streams 模块

WHATWG Streams 与 Node classic streams 的 Goja 实现。

## 模块名

- `require("streams")` / `require("stream/web")` / `require("node:stream/web")` —— WHATWG Web Streams（`web-streams-polyfill@4.3.0`）
- `require("stream")` / `require("node:stream")` —— Node classic streams（**streamx** 引擎，见 `streams/node`）
- `streams.Enable(rt)` 安装全局 Web Streams 构造器。

## WHATWG 能力

- `ReadableStream`（含 byte stream/BYOB、`from`、`values`、async iterator）、`WritableStream`、`TransformStream`、reader/writer/controller、`tee`/`pipeTo`/`pipeThrough`、两种 QueuingStrategy。
- `TextEncoderStream` / `TextDecoderStream`（UTF-8）。
- Go 侧集成：`streams.NewReadableStream` / `NewWritableStream` / `IsReadableStream` / `ConsumeReadableStream`（`streams/integration.go`），供 fs、fetch 等复用 canonical 流。

## Node classic（streamx 引擎）

- 构造器：`Readable` / `Writable` / `Duplex` / `Transform` / `PassThrough`。
- 辅助：`pipeline`、`finished`、`addAbortSignal`、`Readable.from`、谓词、`duplexPair`。
- Web ↔ classic 适配：`Readable.toWeb/fromWeb`、`Writable.toWeb/fromWeb`、`Duplex.toWeb/fromWeb`。
- 事件基座复用 canonical `events` 模块（`require("events").EventEmitter` 与 stream 对象构造器身份一致）。
- bundle 来源：`streams/internal/streamx`（streamx@2.28.0，esbuild，SHA-256 见其 README）；构建脚本 `scripts/build-node-streams.mjs`。

## 已知差异（vs Node classic）

- 字符串 chunk 在写/推时被转换为 `Uint8Array`（需 `Buffer.from(chunk).toString(encoding)` 解码）。
- `read(0)` 不支持；默认 destroy 错误为 `STREAM_DESTROYED`；状态存于 `_duplexState` 位掩码。

## Go API

```go
// 读取 canonical ReadableStream
consumed, err := streams.ConsumeReadableStream(rt, stream, func(chunk goja.Value) goja.Value {
    return goja.Undefined() // 返回 Promise 可施加背压
})
```

## 说明

- goja 未定义 `Symbol.asyncIterator`；`streams`/`events` 装配时将其设为 `Symbol.for("Symbol.asyncIterator")`，保证 polyfill 的 `ReadableStream.from` 与 streamx async iterator 一致。
