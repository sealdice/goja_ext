# util

`util` 是项目内置的 Goja 工具模块，当前提供 `format` 和 `inspect`
两个函数。模块名称为 `"util"`，通过 `require("util")` 获取。

## `format`

`util.format(format, ...args)` 提供常用格式化占位符：

- `%s`：字符串
- `%d`：数字
- `%j`：通过当前 runtime 的 `JSON.stringify` 序列化
- `%%`：百分号

未被占位符消费的参数会以空格分隔追加到结果末尾。该函数是面向项目
需求的轻量实现，不覆盖 Node.js `util.format` 的全部规则。

## `inspect`

`util.inspect(value[, options])` 将 JavaScript 值转换为便于日志查看的
字符串，支持：

- 字符串、布尔值、数字、`null`、`undefined`
- 普通对象
- 数组
- 函数名称
- 循环引用检测，输出 `[Circular]`
- `options.depth` 深度控制，默认值为 `2`
- `depth: -1` 表示不限制嵌套深度

示例：

```javascript
const util = require("util");

util.inspect({
  name: "demo",
  items: [1, 2, 3]
});
// { 'name': 'demo', 'items': [ 1, 2, 3 ] }
```

输出格式以当前实现为准，不保证与 Node.js `util.inspect` 完全一致。
