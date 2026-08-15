# structuredclone

`structuredclone` 为 Goja 提供全局 `structuredClone(value)` 函数，
用于复制 JavaScript 值，并保留支持类型的容器结构和循环引用。

## 支持范围

当前实现覆盖：

- 基本类型
- 数组
- 普通对象
- 循环引用
- `Map`
- `Set`
- `Date`
- `RegExp`
- `ArrayBuffer`、`DataView` 和 TypedArray

函数、Symbol、`WeakMap`、`WeakSet`、Promise 和无法识别的宿主对象会抛出
名称为 `DataCloneError`、代码为 25 的异常。实现不会通过 JSON 降级，也不会
在失败时返回原对象。

## Go API

```go
import (
    "github.com/dop251/goja"
    "github.com/dop251/goja_nodejs/structuredclone"
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
