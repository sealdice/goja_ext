# buffer 模块

Goja 兼容的 Node `buffer` 模块。

## 模块名

- `require("buffer")` / `require("node:buffer")`
- `buffer.Enable(rt)` 会安装全局 `Buffer`。

## 能力

- `Buffer` 构造器与 `Buffer.from` / `Buffer.alloc` / `Buffer.concat` 等静态方法。
- 编码/解码：utf8、hex、base64、latin1、ucs2 等（`buffer.StringCodecByName`）。
- 其他包通过 `buffer.Bytes` / `buffer.WrapBytes` / `buffer.DecodeBytes` / `buffer.EncodeBytes` 复用字节桥。

## Go API

```go
buffer.Enable(rt) // 安装全局 Buffer
```
