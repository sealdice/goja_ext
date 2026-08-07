# timers 模块

Goja 兼容的 Node `timers` 与 `timers/promises` 模块。

## 模块名

- `require("timers")` / `require("node:timers")`
- `require("timers/promises")` / `require("node:timers/promises")`

## 实现

- `timers` 直接复用 eventloop 安装的全局 `setTimeout`/`setInterval`/`setImmediate`/`clearX`；runtime 无 eventloop 时导出函数调用即抛 `"timers require an event loop"`。
- `timers/promises` 用 Promise 包装：
  - `setTimeout([delay, value, options])` → Promise
  - `setImmediate([value, options])` → Promise
  - `setInterval([delay, value, options])` → async iterator（`return()` 停止并结算 pending `next()`）
  - `scheduler.wait`
- 已知边界：顶层脚本（`RunString`）若用 `const { setTimeout } = require("timers/promises")` 会与 eventloop 安装的全局 `setTimeout` 冲突（goja 的 TDZ 报错）；模块内的 `const` 不受影响。
