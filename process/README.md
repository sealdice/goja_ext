# process 模块

Goja 兼容的 Node `process` 模块（当前为最小实现）。

## 模块名

- `require("process")` / `require("node:process")`
- `process.Enable(rt)` 安装全局 `process`。

## 能力

- `process.env`：宿主环境变量映射（`os.Environ()` 快照）。
- `process.cwd()` / `process.chdir(path)`：读写 runtime-local cwd。若 FS 已
  安装，两者复用同一个 FS provider 并验证目录是否存在；不会调用
  `os.Chdir`，因此不同 runtime 和宿主进程互不影响。
- 全局 `process` 与 `require("process")` 返回同一个 canonical 对象。

## 说明

当前 portable subset 不含 `process.nextTick`、`argv`、`platform` 等进程
控制信息。异步模块通过 runtime scheduler 调度，不依赖 process。
