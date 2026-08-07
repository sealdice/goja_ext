# path 模块

Goja 兼容的 Node `path` 模块。

## 模块名

- `require("path")` / `require("node:path")`

## 实现

- Go 原生实现，无第三方依赖。
- `posix` 基于标准库 `path`；`win32` 单独实现（盘符、UNC、`\`/`/` 分隔符）。
- 平台默认在 Linux 下为 `posix`（与 Node 一致）。
- `resolve` 以宿主进程 `os.Getwd()` 为基准；若宿主用 `fs` 模块设置了逻辑 cwd，两者可能不一致（已知边界）。

## 导出

`join` / `resolve` / `normalize` / `relative` / `isAbsolute` / `basename` / `dirname` / `extname` / `parse` / `format` / `toNamespacedPath` / `sep` / `delimiter` / `posix` / `win32`。
