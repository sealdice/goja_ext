# fetch

`fetch` 为 Goja 提供 Fetch API。Headers、Body、Request、Response 和 FormData
行为来自固定版本的 `bare-fetch` / `bare-form-data` API 层；普通 HTTP 请求由
Go 层的 `go-resty/resty/v2` 执行。`ReadableStream` 上传使用 Resty 配置的底层
`http.Client` 直接发送，以避免 Resty 为重放请求而预读完整 `io.Reader`。项目
没有嵌入 `bare-http1`，Resty 类型和配置也不会暴露给 JavaScript。

## 提供的对象

`Enable` 注册以下全局构造函数：

- `Headers`
- `Request`
- `Response`
- `FormData`

`require("fetch")` 返回包含上述构造函数的对象，并额外导出 `Blob` 和 `File`。
模块对象本身不可调用；`Blob`、`File` 默认也不会写入全局对象。

`EnableFetch` 另外注册全局 `fetch(url, init)` 函数。`fetch` 返回 JavaScript
`Promise<Response>`，因此必须提供项目的 `eventloop.EventLoop`。SSE 使用内置
`require("@microsoft/fetch-event-source")`。

## 支持范围

- 常用请求方法和请求头
- 字符串、ArrayBuffer、TypedArray、URLSearchParams、Blob 和 FormData 请求体：
  发送前转换为静态字节，保留准确的 `Content-Length` 和可重放重定向语义
- canonical `ReadableStream` 请求体：逐块上传，使用有界桥接和逐块确认提供
  背压，不预读完整请求体；chunk 必须是 ArrayBuffer 或 TypedArray 字节块，
  `duplex` 可省略或设为唯一支持值 `"half"`
- `Response.body`：canonical `ReadableStream`（`stream/web`），逐块流式交付
  并使用有界队列提供背压；`text()`、`json()`、`bytes()`、`arrayBuffer()`、
  `buffer()` 和 `formData()` 都消费该流，body 只能消费一次
- Request/Response `clone()`、Response `error()` / `redirect()` / `json()`、
  Headers 标准迭代器与 `getSetCookie()`
- **SSE**：内置 `@microsoft/fetch-event-source@2.0.1`，支持 POST、请求头、请求体、自动重试、`Last-Event-ID` 与 AbortSignal
- `Headers`、`Request`、`Response`、`FormData` 的基础操作
- 通过 `AbortSignal` 取消正在执行的请求
- 超时、代理、自定义 HTTP Client 和 Transport

API 范围不是完整的浏览器 Fetch 标准实现。当前不支持 `mode`、`credentials`、
`cache`、JavaScript 侧可配置的 redirect policy、`integrity`、`keepalive`
和 Bare `agent`。传入非空 `agent` 会拒绝请求；`duplex` 除 `"half"` 外的值
也会拒绝。上传和下载均保持流式，并支持 cancel 和 AbortSignal。

流式请求体不可重放：`307`/`308` 且带 `Location` 的响应会拒绝为
`FetchError`（`code === "NETWORK_ERROR"`），不会向重定向目标重复发送；
`303` 仍按 HTTP 语义改为 GET 后跟随。静态请求体继续支持 `307`/`308` 重放。

流式上传沿用 Resty 底层 `http.Client` 的 Transport、代理、总超时和重定向策略，
但不会执行 Resty 的请求/响应 middleware、hook、重试或其他 `Request.Execute`
专属处理。依赖这些扩展点的集成方应使用自定义 `http.RoundTripper`，或改用静态
请求体。

## Go API

```go
import (
    "time"

    "github.com/dop251/goja"
    "github.com/dop251/goja_nodejs/eventloop"
    "github.com/dop251/goja_nodejs/fetch"
)

rt := goja.New()
loop := eventloop.NewEventLoop(eventloop.EnableConsole(false))
go loop.StartInForeground()
defer loop.Stop()

ready := make(chan struct{})
loop.RunOnLoop(func(rt *goja.Runtime) {
    fetch.Enable(rt)
    if err := fetch.EnableFetch(rt, loop, fetch.WithTimeout(10*time.Second)); err != nil {
        panic(err)
    }
    close(ready)
})
<-ready
```

可用的 Go 侧选项包括：

- `WithTimeout`
- `WithProxy`
- `WithHTTPClient`
- `WithTransport`
- `WithRestyClient`

这些选项只影响 Go 层执行器，不会成为 JavaScript 全局对象或模块导出。
默认 client 最多跟随 20 次重定向；通过 `WithHTTPClient` 或
`WithRestyClient` 传入的 client 保留调用方自己的重定向策略。
`WithTimeout` 是包含上传和响应体读取全程的总超时，不是 chunk 空闲超时；SSE
等长连接通常不应设置总超时，而应由插件使用 `AbortSignal` 控制生命周期。

## JavaScript 示例

```javascript
(async () => {
  const response = await fetch("https://example.com/data", {
    headers: {
      "accept": "application/json"
    }
  });

  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`);
  }

  const data = await response.json();
  console.log(data);
})();
```

如果需要取消请求，应先启用 `abort` 模块，并在 `fetch` 的 `init` 中传入
`signal`：

```javascript
const controller = new AbortController();
const request = fetch("https://example.com/data", {
  signal: controller.signal
});

controller.abort("request cancelled");
```

### SSE 示例

```javascript
const { fetchEventSource } = require("@microsoft/fetch-event-source");

await fetchEventSource("https://example.com/events", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ stream: true }),
  onmessage(event) {
    console.log("data:", event.data);
  }
});
```

### 流式读取响应体

```javascript
const response = await fetch("https://example.com/data");
const reader = response.body.getReader();
while (true) {
  const { done, value } = await reader.read();
  if (done) break;
  // value 为 Uint8Array
  console.log(value);
}
```

### 流式上传请求体

```javascript
const body = new ReadableStream({
  pull(controller) {
    controller.enqueue(new Uint8Array([1, 2, 3, 4]));
    controller.close();
  }
});

const response = await fetch("https://example.com/upload", {
  method: "POST",
  body,
  duplex: "half"
});
```

## 更新嵌入 bundle

安装根目录开发依赖后运行：

```bash
npm run build:fetch
npm run build:fetch-event-source
```

生成物、依赖版本锁和第三方许可证均提交到仓库，运行 fetch 不需要 Node.js 或
`node_modules`。
