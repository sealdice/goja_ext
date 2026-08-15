# abort

`abort` 为 Goja 提供基于 JavaScript 的取消控制能力，包含
`AbortController` 和 `AbortSignal`。

## 功能

- `AbortController`
  - `signal`
  - `abort(reason)`
- `AbortSignal`
  - `aborted`
  - `reason`
  - `addEventListener("abort", listener)`
  - `removeEventListener("abort", listener)`
  - `throwIfAborted()`
  - `AbortSignal.abort(reason)`
  - `AbortSignal.timeout(milliseconds)`

这是当前项目中的轻量实现，覆盖项目内 `fetch` 等模块所需的取消流程，
不承诺覆盖浏览器完整的 `AbortSignal` API。

## Go API

```go
import (
    "github.com/dop251/goja"
    "github.com/dop251/goja_nodejs/abort"
)

rt := goja.New()
abort.Enable(rt)
```

`Enable` 会把构造函数注册为 Goja 全局对象。模块也会以
`"abort"` 的名称注册到 `require` 的核心模块表中。

## JavaScript 示例

```javascript
const controller = new AbortController();
const signal = controller.signal;

signal.addEventListener("abort", (event) => {
  console.log(event.reason);
});

controller.abort("user cancelled");
signal.throwIfAborted();
```

`abort(reason)` 只会生效一次。没有传入 reason 时，当前实现使用
`"Aborted"` 作为默认原因。
