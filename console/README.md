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
- 支持按来源打标与过滤：在 `Config` 里提供 `Tag`（常用 `ModuleTag`，取最内层
  脚本文件名）和可选的 `Filter` 回调，每条日志会带 `[来源]` 前缀并按需被丢弃。
  用 `RequireWithConfig` 注册为原生模块即可启用；同一 runtime 里多个 require 插件
  的日志因此可以区分，并能按来源/方法静音。默认 `Require`/`RequireWithPrinter`
  不抓调用栈、零开销。

当前不提供 Node 的 `Console` 流构造器、`table`、profile 和 timeline 系列 API。

## Go API

```go
console.Enable(rt)
```

```go
registry.RegisterNativeModule(console.ModuleName, console.RequireWithConfig(console.Config{
    Printer: printer,
    Tag:     console.ModuleTag, // 自动以"实际打印文件"打标;nil 则不打标
    Filter:  func(e console.Entry) bool { return e.Method != "debug" },
}))
console.Enable(rt)
```
