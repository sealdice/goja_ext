# errors 包

Node 风格错误构造辅助（内部包，不是 JS 模块）。

## 能力

- `NewTypeError` / `NewRangeError` / `NewError`：构造带 `code`（如 `ERR_INVALID_ARG_TYPE`）的 JS 错误对象。
- `NewArgumentNotNumberTypeError` 等便捷类型错误。
- 供 `goutil`、`buffer` 等 Go 侧模块在参数校验时复用。

## Go API

```go
panic(errors.NewTypeError(rt, errors.ErrCodeInvalidArgType, "The %q argument must be a string.", name))
```
