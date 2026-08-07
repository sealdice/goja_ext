# buffer 模块

Goja 兼容的 Node `buffer` 模块。

## 模块名

- `require("buffer")` / `require("node:buffer")`
- `buffer.Enable(rt)` 会安装全局 `Buffer`。

## 能力

- `Buffer` 构造器与 `Buffer.from` / `Buffer.alloc` / `Buffer.concat` 等静态方法。
- 历史数值构造形式 `new Buffer(size)` / `Buffer(size)` 已实现；新代码仍应优先使用明确的 `Buffer.alloc(size)`。
- 当前公共 codec 表支持 utf8、hex、base64 与 base64Url。
- 其他包通过 `buffer.Bytes` / `buffer.WrapBytes` / `buffer.DecodeBytes` / `buffer.EncodeBytes` 复用字节桥。

## Go API

```go
buffer.Enable(rt) // 安装全局 Buffer
```
