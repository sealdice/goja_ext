# string_decoder 模块

Goja 兼容的 Node `string_decoder` 模块。

## 模块名

- `require("string_decoder")` / `require("node:string_decoder")`

## 实现

- Go 原生实现，使用 `buffer` 模块接收字节输入，并在 decoder 内维护编码状态。
- `StringDecoder` 支持 `write` / `end` / `text` / `fillLast` 与 `.encoding`。
- 默认 `utf8`，跨 chunk 保留不完整多字节序列；`end()`/`fillLast()` 时以单个 U+FFFD 替换尾部不完整序列。
- `base64` 跨 chunk 保留不足 3 字节的尾部；`ucs2`/`utf16le` 保留奇数字节和末尾高代理项。
- 其他编码：`ascii`、`latin1`/`binary`、`hex`。
- 本模块供用户代码显式使用，不接入 Web Streams 内部。
