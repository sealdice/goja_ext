# util

`util` 是项目内置的 Goja 工具模块，当前提供 `format` 和 `inspect`
两个函数。模块名称为 `"util"`，通过 `require("util")` 获取。

## `format`

`util.format(format, ...args)` 提供常用格式化占位符：

- `%s`：字符串
- `%d`：数字
- `%i`：整数
- `%f`：浮点数
- `%j`：通过当前 runtime 的 `JSON.stringify` 序列化
- `%o` / `%O`：使用 `util.inspect` 格式化对象；`%o` 展开层级更深
- `%%`：百分号

未被占位符消费的字符串参数原样追加，其他参数使用 `util.inspect`，因此
`console.log(record.value, record.metadata)` 会输出可读对象，而不是
`[object Object]`。循环对象用于 `%j` 时输出 `[Circular]`。

## `inspect`

`util.inspect(value[, options])` 将 JavaScript 值转换为便于日志查看的
字符串，支持：

- 字符串、布尔值、数字、`null`、`undefined`
- 普通对象
- 数组
- 函数名称
- Date、RegExp、Map、Set、BigInt、Symbol 和 Error
- 循环引用检测，输出 `[Circular]`
- `options.depth` 深度控制，默认值为 `2`
- `depth: -1` 只显示对象类型；`depth: null` 不限制嵌套深度

示例：

```javascript
const util = require("util");

util.inspect({
  name: "demo",
  items: [1, 2, 3]
});
// { name: 'demo', items: [ 1, 2, 3 ] }
```

输出面向 Node.js 常见日志行为，但不支持颜色、自定义 inspect hook、getter 控制、
排序等完整 `util.inspect` 选项，也不保证复杂对象与 Node.js 逐字符一致。
