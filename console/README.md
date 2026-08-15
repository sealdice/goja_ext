# console 模块

Goja 兼容的 Node `console` 模块。

## 模块名

- `require("console")` / `require("node:console")`
- `console.Enable(rt)` 安装全局 `console`；`eventloop.NewEventLoop` 默认启用。

## 能力

- `log` / `info` / `warn` / `error` / `debug` / `trace` 等基础输出。
- `assert`、`dir`、`dirxml`、`count` / `countReset`。
- `time` / `timeLog` / `timeEnd` 和 `group` / `groupCollapsed` / `groupEnd`。
- `clear` 可调用；文本输出后端不维护终端画面，因此它是无操作。
- `trace` 使用 `util.format` 格式化参数，并把 JavaScript 调用栈写到错误输出。
- 普通对象按 Node 文本控制台习惯通过 `util.inspect` 输出，而不是
  `[object Object]`。这是文本结果，不提供浏览器开发者工具的可展开对象视图。
- 输出经 `Printer` 接口接入宿主（默认直接写 stdout/stderr，不附加时间戳）；可用
  `RequireWithPrinter` 自定义。

当前不提供 Node 的 `Console` 流构造器、`table`、profile 和 timeline 系列 API。

## Go API

```go
console.Enable(rt)
```
