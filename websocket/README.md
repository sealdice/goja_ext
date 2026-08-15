# websocket

`websocket` 为 Goja 提供事件驱动的 WebSocket 客户端，底层使用
`github.com/gorilla/websocket`。

该模块依赖项目的 `eventloop.EventLoop`，网络读写在 Go 协程中执行，
JavaScript 事件回调通过事件循环线程派发。

## Go API

```go
import (
    "github.com/dop251/goja"
    "github.com/dop251/goja_nodejs/eventloop"
    "github.com/dop251/goja_nodejs/websocket"
)

rt := goja.New()
loop := eventloop.NewEventLoop(eventloop.EnableConsole(false))
go loop.StartInForeground()
defer loop.Stop()

ready := make(chan struct{})
loop.RunOnLoop(func(rt *goja.Runtime) {
    if err := websocket.Enable(rt, loop); err != nil {
        panic(err)
    }
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

## 依赖与连接管理

`Enable` 是兼容入口：它把当前的全局日志器和 `GlobalConnManager` 注入
模块，因此可以通过 `GlobalConnManager.CloseAll()` 统一关闭这条入口创建的连接。

日志接口复用 `eventloop.Logger`，可以通过 `SetLogger` 注入自定义实现。
未注入日志器时，普通日志级别丢弃，错误日志写入标准错误输出。

需要隔离不同 runtime 时，应为每个模块提供自己的依赖：

```go
manager := &websocket.WebSocketManager{}
module := websocket.New(
    websocket.WithConnectionManager(manager),
)
defer module.CloseAll()

instance, err := module.NewInstanceChecked(rt, loop)
if err != nil {
    return err
}
if err := rt.Set("WebSocket", instance.Exports()); err != nil {
    return err
}
```

`New()` 默认创建独立的管理器、日志器和拨号器。也可以通过
`WithDialer` 注入完整的拨号策略，便于代理、测试或宿主网络策略接管。

只需要安装构造函数时，可使用 `EnableWithOptions`；若要主动清理连接，调用方
应注入并保留自己的 `WebSocketManager`。

## TLS

默认拨号器使用 Go 的系统根证书和主机名校验，不会跳过证书验证。
`WithTLSConfig` 会克隆调用方提供的配置，可用于添加私有 CA、客户端证书等。
若调用方显式设置 `InsecureSkipVerify`，不安全行为由该宿主配置自行承担。

## 注意事项

- 当前实现主要覆盖文本消息和基础 WebSocket 生命周期。
- `Enable` 必须传入非 nil 的事件循环，否则创建连接时会抛出 TypeError。
