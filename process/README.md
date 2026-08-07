# process 模块

Goja 兼容的 Node `process` 模块（当前为最小实现）。

## 模块名

- `require("process")` / `require("node:process")`
- `process.Enable(rt)` 安装全局 `process`。

## 能力

- `process.env`：宿主环境变量映射（`os.Environ()` 快照）。

## 说明

当前仅提供 `env`。`process.nextTick`、`process.cwd`、`process.argv`、`process.platform` 等尚未实现；streamx 依赖的 `nextTick` 由 eventloop 安装的全局 `queueMicrotask` 取代。
