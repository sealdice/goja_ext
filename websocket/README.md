# websocket

`websocket` 为 Goja 提供事件驱动的 WebSocket 客户端，底层使用
`github.com/gorilla/websocket`。

该模块依赖项目的 `eventloop.EventLoop`，网络读写在 Go 协程中执行，
JavaScript 事件回调通过事件循环线程派发。

## Go API

```go
import (
    "github.com/dop251/goja"
    "github.com/sealdice/goja_ext/eventloop"
    "github.com/sealdice/goja_ext/websocket"
)

rt := goja.New()
loop := eventloop.NewEventLoop(eventloop.EnableConsole(false))
go loop.StartInForeground()
defer loop.Stop()

ready := make(chan struct{})
loop.RunOnLoop(func(rt *goja.Runtime) {
    websocket.Enable(rt, loop)
    close(ready)
})
<-ready
```

`Enable` 会注册全局 `WebSocket` 构造函数。当前模块不通过
`require` 注册，而是直接绑定到指定 Goja runtime。

## JavaScript API

```javascript
const ws = new WebSocket("ws://127.0.0.1:8080/echo");

ws.onopen = () => {
  ws.send("hello");
};

ws.onmessage = (event) => {
  console.log(event.data);
  ws.close();
};

ws.onerror = (event) => {
  console.error(event.error);
};
```

连接对象提供：

- `url`
- `protocol`
- `readyState`
- `CONNECTING`、`OPEN`、`CLOSING`、`CLOSED`
- `onopen`、`onmessage`、`onclose`、`onerror`
- `send(message)`
- `close(code, reason)`
- `addEventListener(type, listener)`
- `removeEventListener(type, listener)`

## 连接管理和日志

`GlobalConnManager` 会跟踪已创建的连接，可以通过
`GlobalConnManager.CloseAll()` 统一关闭连接。

日志接口复用 `eventloop.Logger`，可以通过 `SetLogger` 注入自定义实现。
未注入日志器时，普通日志级别丢弃，错误日志写入标准错误输出。

## 注意事项

- 当前实现的 TLS 配置启用了 `InsecureSkipVerify`，不会校验证书。
  使用 `wss://` 连接不应默认视为安全连接。
- 当前实现主要覆盖文本消息和基础 WebSocket 生命周期。
- `Enable` 必须传入非 nil 的事件循环，否则创建连接时会抛出 TypeError。
