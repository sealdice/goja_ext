# Fetch ReadableStream EOF 竞态修复设计

## 背景

`fetch()` 将 HTTP 响应体通过 `streamingBody` 桥接为 JavaScript
`ReadableStream`。后台读取协程把字节块写入内部有界队列，JavaScript
`pull()` 从队列取数据；没有数据时，`pull()` 返回 Promise，后台读取到数据后
通过 event loop 解析该 Promise。

当前实现收到 EOF 时只唤醒当时已经登记的 waiter 并执行 cleanup。如果最后一个
数据 waiter 已被取走、而下一个 `pull()` 尚未登记，EOF 通知不会主动关闭
`ReadableStream`。JavaScript 随后的 `reader.read()` 可能一直等待，无法得到
`{ done: true }`。

## 目标

- 正常 EOF 到达后，所有已读取字节仍按顺序交给 JavaScript。
- 最后一个字节块之后，`reader.read()` 必须稳定返回 `{ done: true }`。
- 保留现有 16 块内部队列上限和长连接背压行为。
- cleanup 仍然只执行一次。

## 非目标

- 不重写 Fetch、ReadableStream 或 EventSource 架构。
- 不改变 EventSource 的解析、重连和 `Last-Event-ID` 行为。
- 不在本次修复中重新定义非 EOF 网络错误发生时的缓冲数据语义。

## 方案

`streamingBody` 保存由 `ReadableStream` source 提供的 controller，但只在 event
loop 线程访问它。后台读取协程检测到正常 EOF 后，通过 scheduler 安排一个终止
回调。该回调在 event loop 上：

1. 取出内部队列中尚未交付的字节块；
2. 按原顺序将这些块 enqueue 到 controller；
3. 调用 controller 的 `close()`；
4. 解析仍在等待的 pull Promise；
5. 执行幂等 cleanup。

数据唤醒与 EOF 终止回调均由同一个 scheduler 提交，而 event loop 保证提交顺序，
因此终止回调不会越过更早的数据通知。EOF 后队列不会继续增长，终止时排空至
ReadableStream 自身队列不会破坏长连接期间的有界背压。

非 EOF 错误继续沿用现有路径，避免本修复扩大行为范围。

## 测试

新增一个确定性回归用例，使用受控 `io.ReadCloser` 在单次 `Read()` 中同时返回
最后一段字节和 `io.EOF`。goja 中的 JavaScript 使用 `body.getReader()`：第一次
读取必须得到完整字节，第二次读取必须得到 `done === true`，且不能超时。

验证顺序：

1. 新回归测试在修复前因第二次读取超时而失败；
2. 实现最小修复后，新回归测试通过；
3. 重复运行流式与 EventSource 测试；
4. 运行 `go test -race ./fetch`；
5. 运行 `go test ./...`。
