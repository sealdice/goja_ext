# eventloop 模块

Goja 的高性能事件循环。fork 自 goja_nodejs eventloop，支持生命周期管理、panic 统计与自定义 logger。

## 能力

- `Run(fn)` / `Start()` / `StartInForeground()` / `Stop()` / `Terminate()`。
- `RunOnLoop(fn)`：线程安全地调度任务到循环线程（goroutine-safe）。
- `SetTimeout` / `SetInterval` / `SetImmediate` / `ClearTimeout` / `ClearInterval` / `ClearImmediate`（Go 侧）。
- 安装 JS 全局：`setTimeout`、`setInterval`、`setImmediate`、`clearTimeout`、`clearInterval`、`clearImmediate`、`queueMicrotask`。
- `queueMicrotask` 通过 `Promise.resolve().then(fn)` 走 goja 原生微任务队列（与 promise reaction 同队列、FIFO、`leave()` 时排空），语义与 Node 一致。

## Go API

```go
loop := eventloop.NewEventLoop(eventloop.EnableConsole(false))
go loop.StartInForeground()
defer loop.Stop()

loop.RunOnLoop(func(vm *goja.Runtime) {
    // 所有 JS 执行收敛在此
})
```

## 说明

- goja 非 goroutine-safe：所有引擎调用必须发生在循环线程。
- 一个 loop 只拥有它创建的 runtime；将该 loop 传给其他 runtime 的异步模块会
  返回 ownership 错误。
- eventloop 有意静态依赖 `console`，因为 `NewEventLoop()` 默认安装 console。
  `EnableConsole(false)` 关闭运行时安装，但不会改变 Go 的编译依赖。只需要调度
  抽象时可实现 `runtimehost.Scheduler`。
- 若 runtime 无 eventloop，`timers` 模块导出的函数调用会抛 `"timers require an event loop"`。
