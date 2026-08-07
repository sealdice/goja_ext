# string_decoder 模块

Goja 兼容的 Node `string_decoder` 模块。

## 模块名

- `require("string_decoder")` / `require("node:string_decoder")`

## 实现

- Go 原生实现，基于 `buffer` 模块的编解码能力。
- `StringDecoder` 支持 `write` / `end` / `text` / `fillLast` 与 `.encoding`。
- 默认 `utf8`，跨 chunk 保留不完整多字节序列；`end()`/`fillLast()` 时以单个 U+FFFD 替换尾部不完整序列。
- 其他编码：`ascii`、`latin1`/`binary`、`base64`、`hex`、`ucs2`/`utf16le`。
- 注意：streamx 栈使用 `text-decoder`，本模块仅供用户代码，不接入经典流内部。
