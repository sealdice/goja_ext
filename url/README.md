# url 模块

Goja 兼容的 Node `url` 模块。

## 模块名

- `require("url")` / `require("node:url")`
- `url.Enable(rt)` 安装全局 `URL` / `URLSearchParams`。

## 能力

- `URL`：解析、属性访问（`href` / `protocol` / `host` / `pathname` / `search` / `hash` 等）、`toString`。
- `origin` 保留非默认端口；HTTP/WS 系列与 FTP 的默认端口会在解析时规范化掉。
- `URLSearchParams`：`get` / `set` / `append` / `delete` / `has` / `toString` / 迭代。
- `domainToASCII` / `domainToUnicode`。
