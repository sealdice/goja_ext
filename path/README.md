# path 模块

Goja 兼容的 Node `path` 模块。

## 模块名

- `require("path")` / `require("node:path")`

## 实现

- Go 原生实现，无第三方依赖。
- `posix` 基于标准库 `path`；`win32` 单独实现（盘符、UNC、`\`/`/` 分隔符）。
- 默认导出按宿主平台选择 `posix` 或 `win32`，并与对应子对象共享函数身份。
- `resolve` 每次调用都从 `runtimehost` 读取 runtime-local cwd；`fs.chdir`
  与 `process.chdir` 的更新会立即反映到已经加载的 path 对象。
- `posix.sep/delimiter` 与 `win32.sep/delimiter` 是字符串属性，不是函数。

## 导出

`join` / `resolve` / `normalize` / `relative` / `isAbsolute` / `basename` / `dirname` / `extname` / `parse` / `format` / `toNamespacedPath` / `sep` / `delimiter` / `posix` / `win32`。
