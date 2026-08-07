# goutil 包

Goja 参数校验与类型转换辅助（内部包，不是 JS 模块）。

## 能力

- `RequiredIntegerArgument` / `RequiredStrictIntegerArgument` / `RequiredStringArgument` 等：从 `goja.FunctionCall` 读取并校验参数，出错时抛 Node 风格错误（配合 `errors` 包）。
- `GetUint32` / `GetInt64` / `GetString` 等 goja `Value` 到 Go 类型的转换。

## 依赖

- 依赖 `errors` 包生成带 `code` 的 JS 错误。
