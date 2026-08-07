# fetch

`fetch` 为 Goja 提供一组面向 JavaScript 的 Fetch API 基础能力。
HTTP 请求由 Go 层的 `go-resty/resty/v2` 执行，Resty 类型和配置不会暴露给
JavaScript。

## 提供的对象

`Enable` 注册以下全局构造函数：

- `Headers`
- `Request`
- `Response`
- `FormData`

`EnableFetch` 另外注册全局 `fetch(url, init)` 函数与 `EventSource`（SSE）。
`fetch` 返回 JavaScript `Promise<Response>`，因此必须提供项目的
`eventloop.EventLoop`。

## 支持范围

- 常用请求方法和请求头
- 字符串、ArrayBuffer、URLSearchParams、FormData 请求体
- `Response.body`：**canonical `ReadableStream`**（`stream/web`），逐块流式交付，天然支持背压；`text()` / `json()` / `arrayBuffer()` 消费该流（浏览器语义：body 只能消费一次）
- **SSE**：`new EventSource(url)`，支持 `message`/`open`/`error` 事件、自定义事件名、自动重连（`retry` 字段）、`Last-Event-ID`、`close()`
- `Headers`、`Request`、`Response`、`FormData` 的基础操作
- 通过 `AbortSignal` 取消正在执行的请求
- 超时、代理、自定义 HTTP Client 和 Transport

API 范围不是完整的浏览器 Fetch 标准实现（例如 `Response.body` 的 cancel 语义、`mode`/`credentials` 的完整约束未实现）。

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
