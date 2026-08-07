# console 模块

Goja 兼容的 Node `console` 模块。

## 模块名

- `require("console")` / `require("node:console")`
- `console.Enable(rt)` 安装全局 `console`；`eventloop.NewEventLoop` 默认启用。

## 能力

- `log` / `info` / `warn` / `error` / `debug` / `trace` 等基础输出。
- 输出经 `Printer` 接口接入宿主（默认 `fmt.Println`）；可用 `RequireWithPrinter` 自定义。

## Go API

```go
console.Enable(rt)
```
