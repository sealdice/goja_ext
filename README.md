# goja_ext

`goja_ext` 为 [Goja](https://github.com/dop251/goja) 提供可组合的 Node.js、
Web Platform 和宿主能力模块。模块可以按需安装；带异步行为的模块通过同一个
runtime-bound event loop 调度，不跨 runtime 操作 Goja 值。

## 模块边界

| 表面 | 安装入口 | 依赖与约束 |
| --- | --- | --- |
| Event loop | `eventloop.NewEventLoop` | 一个 loop 只拥有它创建的 runtime；默认安装 console |
| Abort | `abort.Enable` / `require("abort")` | `AbortSignal.timeout()` 需要当前 runtime 的 scheduler |
| Fetch | `fetch.EnableFetch(rt, loop)` | 复用 canonical Abort 与 Web Streams；异步回调只回到所属 loop |
| Deno FS | `fs.EnableWithLoop` | 只安装 Deno 风格表面，不加载 Node classic streams |
| Node FS | `require("fs")` / `require("fs/promises")` | Node 文件流需要显式导入 `streams/node` 并启用 `WithStreams(true)` |
| Web Streams | `streams.Enable` / `require("stream/web")` | 使用原生 `Uint8Array`，不依赖 Node Buffer |
| Node Streams | blank-import `streams/node` 后 `require("stream")` | 复用 canonical Events；Web adapter 首次使用时才加载 Web Streams |
| WebSocket | `websocket.EnableWithOptions(rt, loop, ...)` | dialer、TLS、logger、manager 均按实例注入；TLS 默认校验 |

`Enable` 与 `require(...)` 在同一个 runtime 内复用 canonical 构造器和导出对象。
同一 runtime 的 FS backend、cwd、scheduler 等配置只能等价地重复安装；冲突会
直接返回错误，不采用 first-call-wins。

eventloop 对 `console` 的 Go import 是有意保留的兼容依赖：
`NewEventLoop()` 默认启用 console。`EnableConsole(false)` 仅关闭该 runtime 的
console 安装，不能在 Go 编译期动态移除依赖。只需要调度接口的宿主可以直接实现
`runtimehost.Scheduler`，无需依赖 eventloop。

## 组合 runtime

```go
registry := require.NewRegistry()
loop := eventloop.NewEventLoop(
	eventloop.EnableConsole(false),
	eventloop.WithRegistry(registry),
)

loop.Run(func(rt *goja.Runtime) {
	abort.Enable(rt)
	fetch.Enable(rt)
	if err := fetch.EnableFetch(rt, loop); err != nil {
		panic(err)
	}
})
```

Goja 不是 goroutine-safe。宿主应在 `loop.Run` 或 `loop.RunOnLoop` 回调中安装模块
和执行 JavaScript；网络、文件系统及 WebSocket 的完成回调也会返回同一个 loop。

## 可运行示例

```text
go run ./examples/composed_runtime
go run ./examples/fetch_abort
go run ./examples/fs_runtime
go run ./examples/websocket_tls
go run ./examples/streams
```

这些示例分别验证 canonical 模块身份、Fetch 取消、FS/process/path 共享逻辑 cwd、
自签名证书的显式 TLS trust 注入，以及 Node/Web Streams adapter。

## 验证

```text
go test -count=1 ./...
go test -count=1 -race ./...
go vet ./...
go build ./...
golangci-lint run ./...
git diff --check
```

更详细的依赖与运行时约束见
[`docs/runtime-module-contracts-design.md`](docs/runtime-module-contracts-design.md)。

## Type definitions

已实现模块的类型定义发布在 npm 的 `@dop251/types-goja_nodejs-MODULE` 包中。
使用时安装相应模块，并把 `node_modules/@dop251` 加入 `tsconfig.json` 的
`typeRoots`。类型包按模块拆分，以便与 Go 侧的按需启用模型保持一致。
