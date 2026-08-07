# structuredclone

`structuredclone` 为 Goja 提供全局 `structuredClone(value)` 函数，
用于复制 JavaScript 值，并尽量保留容器结构和循环引用。

## 支持范围

当前实现覆盖：

- 基本类型
- 数组
- 普通对象
- 循环引用
- `Map`
- `Set`
- `Date`

对于未特别处理的值，当前实现会回退到 JSON 序列化和反序列化。
因此函数、符号以及其他复杂对象不应被视为完整浏览器
Structured Clone Algorithm 的等价实现。

## Go API

```go
import (
    "github.com/dop251/goja"
    "github.com/sealdice/goja_ext/structuredclone"
)

rt := goja.New()
structuredclone.Enable(rt)
```

`Enable` 会注册全局 `structuredClone` 函数。模块也会以
`"structuredclone"` 的名称注册到 `require` 的核心模块表中。

## JavaScript 示例

```javascript
const original = {
  name: "demo",
  values: new Set([1, 2, 3])
};
original.self = original;

const cloned = structuredClone(original);

cloned !== original;                // true
cloned.values instanceof Set;       // true
cloned.self === cloned;             // true
```

调用时必须传入待复制的值；缺少参数会抛出 TypeError。
