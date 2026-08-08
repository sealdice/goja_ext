# fetch

`fetch` 为 Goja 提供 Fetch API。Headers、Body、Request、Response 和 FormData
行为来自固定版本的 `bare-fetch` / `bare-form-data` API 层；HTTP 请求仍由 Go
层的 `go-resty/resty/v2` 执行。项目没有嵌入 `bare-http1`，Resty 类型和配置也
不会暴露给 JavaScript。

## 提供的对象

`Enable` 注册以下全局构造函数：

- `Headers`
- `Request`
- `Response`
- `FormData`

`require("fetch")` 返回包含上述构造函数的对象，并额外导出 `Blob` 和 `File`。
模块对象本身不可调用；`Blob`、`File` 默认也不会写入全局对象。

`EnableFetch` 另外注册全局 `fetch(url, init)` 函数与 `EventSource`（SSE）。
`fetch` 返回 JavaScript `Promise<Response>`，因此必须提供项目的
`eventloop.EventLoop`。

## 支持范围

- 常用请求方法和请求头
- 字符串、ArrayBuffer、TypedArray、URLSearchParams、Blob、FormData 和
  `ReadableStream` 请求体；发起网络请求前会完整缓冲上传体
- `Response.body`：canonical `ReadableStream`（`stream/web`），逐块流式交付
  并使用有界队列提供背压；`text()`、`json()`、`bytes()`、`arrayBuffer()`、
  `buffer()` 和 `formData()` 都消费该流，body 只能消费一次
- Request/Response `clone()`、Response `error()` / `redirect()` / `json()`、
  Headers 标准迭代器与 `getSetCookie()`
- **SSE**：`new EventSource(url)`，支持 `message`/`open`/`error` 事件、自定义事件名、自动重连（`retry` 字段）、`Last-Event-ID`、`close()`
- `Headers`、`Request`、`Response`、`FormData` 的基础操作
- 通过 `AbortSignal` 取消正在执行的请求
- 超时、代理、自定义 HTTP Client 和 Transport

API 范围不是完整的浏览器 Fetch 标准实现。当前不支持 `mode`、`credentials`、
`cache`、JavaScript 侧可配置的 redirect policy、`integrity`、`keepalive`、
`duplex` 和 Bare `agent`。传入非空 `agent` 会拒绝请求。真正的流式上传暂未
实现；下载响应保持流式，并支持 cancel 和 AbortSignal。

## Go API

```go
import (
    "time"

    "github.com/dop251/goja"
    "github.com/sealdice/goja_ext/eventloop"
    "github.com/sealdice/goja_ext/fetch"
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
const es = new EventSource("https://example.com/events");

es.onopen = () => console.log("connected");
es.onmessage = (event) => console.log("data:", event.data);
es.addEventListener("custom", (event) => console.log("custom:", event.data));

// 取消订阅 / 关闭
// es.close();
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

## 更新嵌入 bundle

安装根目录开发依赖后运行：

```bash
npm run build:fetch
```

生成物、依赖版本锁和第三方许可证均提交到仓库，运行 fetch 不需要 Node.js 或
`node_modules`。
