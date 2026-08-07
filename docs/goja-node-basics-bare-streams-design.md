# Goja Node 基础模块与 bare-stack classic streams 功能设计

> **范围（2026-08-07）**：新增 `events`、`path`、`string_decoder`、`timers`(+`timers/promises`)四个 Node 兼容模块，并将 `stream`/`node:stream` facade 的底层引擎从 `readable-stream@4.7.0` 替换为 **streamx + canonical events（bare-events 改编）**。`stream/web` 与 fs 模块保持基于 `web-streams-polyfill@4.3.0` 的实现，不做改动。
> **实现说明**：本文档描述外部可观察行为与 Goja 集成契约；经典流状态机由 vendored `streamx` 提供，事件基座由改编版 `bare-events` 提供，Go 层只负责模块装配、canonical 身份、宿主集成与 Web↔classic 桥接。

---

## 0. 背景与动机

当前仓库是 `github.com/sealdice/goja_ext`，已提供 `require`、`buffer`、`eventloop`、`streams`（WHATWG Streams + classic facade）、`fs`（Afero-backed）、`fetch`、`websocket`、`abort`、`url`、`util` 等模块。`stream`/`node:stream` facade 目前由 vendored `readable-stream@4.7.0` bundle（`streams/internal/nodepolyfill/readable-stream.js`）驱动。

本设计要解决三类问题：

1. **基础模块缺失**：`events`（EventEmitter）、`path`、`string_decoder`、`timers` 是 Node 生态的基座，用户代码与后续模块都需要，目前仓库没有。
2. **性能**：readable-stream 是 Node 原版实现，行为保真但性能平庸；`streamx` 的设计目标就是替代 Node streams 并更快，是本项目"性能更高"诉求的正解。
3. **canonical 身份**：readable-stream bundle 内部自带一份 EventEmitter（`require_events`），与任何独立的 `events` 模块是**两个不同构造器**，导致 `instanceof`/身份不一致。本设计让经典流内部与公开 `events` 模块共享同一个 canonical 实例。

### 关键取舍

- **引擎选 streamx，不整包使用 bare-stream**：`bare-stream/index.js` 强制 `require('./web')`，会带进第二套 WHATWG Streams 实现（`bare-stream/web`）。这会破坏 `streams.IsReadableStream`（Go 侧基于 canonical `web-streams-polyfill` 的识别）与 fs 的流输入。因此经典流 facade = **streamx 引擎 + 自建薄 facade**（API 面按 `功能规格说明书.md` §3.0.2），Web 互操作走已有 canonical streams 桥。
- **canonical events 用 JS（bare-events 改编），不用 Go 原生**：streamx 内部是 `class Stream extends EventEmitter`，JS class 构造器在 goja 中可被 `class extends` 安全继承、也可 `EE.call(this)`；Go 构造器继承行为未验证且 emit 走 FFI 无性能收益。性能故事在 streamx 引擎，不在 emitter。
- **`queueMicrotask` 由 eventloop 提供**：goja 无此全局，也没有从 Go 直接入队微任务的公开 API（`enqueuePromiseJob`/`leave` 均未导出）。`Promise.resolve().then(fn)` 走的就是 goja 原生微任务队列（与 promise reaction 同队列、FIFO、`leave()` 时排空），语义与 Node 一致；**不能用 `RunOnLoop`**（那是 task 级 auxJobs，顺序会错且 `Run()` 短生命周期下可能不执行）。由 `eventloop.NewEventLoop` 安装全局 `queueMicrotask`。

---

## 1. 项目概述

### 1.1 技术栈

| 项 | 值 |
|---|---|
| 语言 | Go |
| JS 引擎 | `github.com/dop251/goja` |
| 模块系统 | `github.com/sealdice/goja_ext/require`（goja_nodejs） |
| 事件循环 | `github.com/sealdice/goja_ext/eventloop` |
| 经典流引擎 | `streamx@2.28.0`（esbuild bundle，Apache/MIT） |
| 事件基座 | `bare-events`（改编，Apache-2.0） |
| Web 流 | `web-streams-polyfill@4.3.0`（不变） |
| 打包 | `esbuild`（devDependency） |

### 1.2 模块注册契约

- 用 `require.RegisterCoreModule("<name>", loader)` 或 `Registry.RegisterNativeModule` 注册；`node:` 前缀别名由 `require` 自动提供。
- 每个模块按 runtime 缓存 canonical 实例（global symbol，同 `streams` 现有模式），保证同一 runtime 内构造器身份一致。

---

## 2. `queueMicrotask`（eventloop 提供）

### 2.1 契约

`eventloop.NewEventLoop` 在安装 `setTimeout` 等全局的同时安装 `queueMicrotask`：

```js
queueMicrotask(fn)
```

- `fn` 必须是函数，否则抛 TypeError。
- `fn` 入队 goja 原生微任务队列，在控制离开 runtime（`leave()`）时按 FIFO 排空。
- 与 promise reaction 交错顺序与 Node 一致（同队列 FIFO）。

### 2.2 实现

```go
_ = vm.Set("queueMicrotask", func(call goja.FunctionCall) goja.Value {
    fn := call.Argument(0)
    if fn == nil || goja.IsUndefined(fn) || goja.IsNull(fn) || !goja.IsFunction(fn) {
        panic(vm.NewTypeError("queueMicrotask expects a function"))
    }
    // Promise.resolve().then(fn) 走 goja 原生微任务队列
    // （用中间函数包装，使返回的 promise 不参与用户可见链）
    ... 
})
```

包装写法（避免 fn 返回值泄漏到未处理的派生 promise 链，同时让 fn 抛错表现为未处理的 promise rejection）：

```js
function queueMicrotask(fn) {
  Promise.resolve().then(function () { fn(); })
}
```

> **已知差异**：`fn` 抛错在 Node 为 `'uncaughtException'`，此处为未处理 promise rejection，宿主可用 `SetPromiseRejectionTracker` 兜底。streamx 内部 `qmt` 回调不抛错，实际无影响。

---

## 3. `events` 模块（canonical EventEmitter）

### 3.1 模块表面

- 模块名：`events`、`node:events`。
- 导出：
  - `EventEmitter`（构造器，可 `new`、可被 `class extends`、可 `EE.call(this)`）
  - `EventEmitter.once(emitter, name[, options])`、`EventEmitter.listenerCount`、`EventEmitter.defaultMaxListeners`、`EventEmitter.captureRejections`
  - `events.once(emitter, name[, options])` → Promise / async iterator
  - `events.on`（async iterator）
  - `events.getEventListeners(emitter, name)`
  - `events.setMaxListeners(n[, ...emitters])`
  - `events.addAbortListener(signal, listener)`
  - `events.emit(emitter, ...args)`（Node 24+）
  - `Symbol.for('nodejs.rejection')`（captureRejections 支持）

### 3.2 实现

- Go 包 `events`，嵌入改编版 `bare-events`（`events/events.js`，Apache-2.0，LICENSE 与 SHA-256 记录在 README）。
- 改编点：
  1. `.listeners(name)` / `.rawListeners(name)` 返回**纯函数数组**（bare-events 原版 `listeners()` 返回 `[fn, once]` 元组，与 Node 不符）。
  2. 无监听者的 `error` 事件在 `emit` 内**同步抛出**（Node 语义；原版走 `queueMicrotask` 抛）。
  3. 补 `events.getEventListeners`、`events.setMaxListeners`、`events.addAbortListener`、`events.emit`。
  4. `throwUnhandledError` 的 `queueMicrotask` 使用全局（由 eventloop 提供，Phase 0）。
  5. `captureRejections` 通过 `Symbol.for('nodejs.rejection')` 支持（宿主可用 `SetPromiseRejectionTracker` 消费）。
- `Require(rt, module)` 注册并按 runtime 缓存；`Exports(rt) *goja.Object` 暴露 canonical 实例，供 `streams/node` 复用（通过 bundle external + require shim 注入）。

### 3.3 行为契约（测试锁定）

- `new EventEmitter()` 无状态污染；实例方法返回 `this`（链式）。
- `once` 监听在触发后自动移除；`emit` 返回是否有监听者。
- `newListener` 在**加入前**触发、`removeListener` 在**移除后**触发（bare-events 原语义，与 Node 一致）。
- `events.once(emitter, 'foo')`：触发解析为参数数组；`signal` abort 时 reject `AbortError`。
- `class MyEmitter extends EventEmitter` 可正常 `super()` 并继承原型方法。
- `EventEmitter.call(plainObj)` 可初始化对象。

---

## 4. `path` 模块

### 4.1 模块表面

- 模块名：`path`、`node:path`。
- 三套命名空间：平台默认（Linux 下 = posix）、`path.posix`、`path.win32`。
- 函数：`join`、`resolve`、`normalize`、`relative`、`isAbsolute`、`basename`、`dirname`、`extname`、`parse`、`format`、`sep`、`delimiter`、`toNamespacedPath`、`posix`、`win32`。

### 4.2 实现

- Go 原生实现。posix 用标准库 `path`，win32 单独实现（驱动器盘符、反斜杠、UNC）。
- 行为对齐 Node：`join` 空段忽略、`resolve` 从右向左找绝对路径、`normalize` 折叠 `.`/`..` 并保留前导 `//`（posix）、`parse`/`format` 双向。

---

## 5. `string_decoder` 模块

### 5.1 模块表面

- 模块名：`string_decoder`、`node:string_decoder`。
- 导出 `StringDecoder`：`write(buffer)`、`end(buffer?)`、`text(buffer)`、`fillLast`、`.encoding`。

### 5.2 实现

- Go 原生实现。默认 utf8；跨 chunk 的不完整多字节序列保留到下一 chunk，`end()` 时以 U+FFFD 处理尾部不完整字节（与 `TextDecoderStream` 的 UTF-8 桥一致）。
- 支持编码：`utf8`/`utf-8`（默认）、`ascii`、`latin1`/`binary`、`base64`、`hex`、`ucs2`/`utf16le`。非 utf8 编码的 `write`/`end` 通过 `buffer` 模块的编解码能力实现。
- streamx 使用 `text-decoder`，**不消费**本模块；本模块仅供用户代码。

---

## 6. `timers` 模块

### 6.1 模块表面

- 模块名：`timers`、`node:timers`、`timers/promises`、`node:timers/promises`。
- `timers`：`setTimeout`、`setInterval`、`setImmediate`、`clearTimeout`、`clearInterval`、`clearImmediate`。
- `timers/promises`：`setTimeout([delay, value, options])` → Promise、`setImmediate([value, options])` → Promise、`setInterval([delay, value, options])` → async iterator、`scheduler.wait`。

### 6.2 实现

- 读取 runtime 全局 `setTimeout`/`setInterval`/`setImmediate`/`clearX`（由 eventloop 安装）；缺失（无 eventloop 的裸 runtime）时导出函数调用即抛明确错误：`"timers require an event loop"`。
- `timers/promises` 用 JS 包装（`new Promise(resolve => setTimeout(resolve, delay, value))` 等）；`setInterval` 返回 async iterator 对象。

---

## 7. `streams/node` 引擎替换为 streamx

### 7.1 模块表面（不变，见 `功能规格说明书.md` §3.0.2）

- 模块名：`stream`、`node:stream`。
- 构造器：`Readable`、`Writable`、`Duplex`、`Transform`、`PassThrough`。
- 静态/辅助：`Readable.from`、`pipeline`、`finished`、`addAbortSignal`、`isDestroyed`、`isDisturbed`、`isReadable`、`isWritable`、`isEnded`、`isFinished`、`getStreamError`、`duplexPair`。
- 事件：`data`、`readable`、`end`、`finish`、`close`、`error`、`drain`、`open`。
- 适配器：`Readable.fromWeb/toWeb`、`Writable.fromWeb/toWeb`、`Duplex.fromWeb/toWeb`。
- `stream` 与 `node:stream` 共享构造器身份。

### 7.2 引擎 bundle

- 新增 `scripts/build-node-streams.mjs`（esbuild devDependency），打包入口为**改编自 bare-stream/index.js 的 facade**（去掉 `require('./web')` 及其 toWeb/fromWeb 的 JS 实现），依赖 `streamx@2.28.0` + `fast-fifo` + `text-decoder` + `b4a`（+`teex` 按需）。
- esbuild：`--format=iife`、`--target=es2015`、`--external:events`（canonical events 由 Go 侧注入）。
- 产物 `streams/internal/streamx/bundle.js`（新 vendored 目录），记录版本、LICENSE、SHA-256。
- 可选：把 `streams/internal/nodepolyfill/`（readable-stream）标记为废弃或移除。

### 7.3 Go 集成（`streams/node`）

- `mustCompilePolyfill` 换新 bundle；注入方式沿用现有 `self` 注入模式：
  - canonical `events`（require shim，指向 `events.Exports(rt)`）
  - `AbortController`/`AbortSignal`（来自 `abort` 模块）
  - `queueMicrotask`（Phase 0，eventloop 已装，缺失时兜底安装）
- 按 runtime 缓存 exports（global symbol）。
- Web 互操作 `fromWeb`/`toWeb` 在 Go 层（`streams/node/from_web.go`）用 `streams.NewReadableStream`/`NewWritableStream`/`ConsumeReadableStream` + streamx async iterator 实现，目标对象是 canonical `web-streams-polyfill` 流（保证 `streams.IsReadableStream` 与 fs 输入识别有效）。

### 7.4 与 readable-stream 的已知语义差异（测试重基线时记录）

| 项 | readable-stream (Node) | streamx |
|---|---|---|
| `read(0)` | 支持（刷 flow 状态） | 不支持（返回 null） |
| 默认 destroy 错误 | `ERR_STREAM_DESTROYED` | `STREAM_DESTROYED` |
| 状态布局 | `_readableState`/`_writableState` | `_duplexState` 位掩码 |
| `.pipe()` | 完整事件流语义 | 简化（`piping`/`pipe` 事件） |
| `write` 编码 | 自动编码 | 由 facade 用 b4a 转换 |

> facade 吸收部分差异（字符串编码、`_read(size)`→streamx `read(cb)` 桥、closed/errored getter）；残余差异为 bounded facade 可接受范围（`功能规格说明书.md` §2 明确边界）。

---

## 8. 明确不做

- 不打包 `bare-stream` 整包（避免其 `web.js` 第二套 WHATWG 实现）。
- 不替换 `stream/web`（保持 `web-streams-polyfill@4.3.0`）。
- 不改动 fs 模块（其只依赖 canonical WHATWG Streams）。
- 不实现 worker_threads / http / net / crypto（后续迭代）。
- 不引入 Go 原生 EventEmitter。

---

## 9. 回退方案

若 streamx bundle 无法在 goja 稳定运行（微任务/编码/AbortController shim 失败、回归测试大面积失败）：
- 保留 `readable-stream@4.7.0` facade；
- 仅把 `events` external 化并注入 canonical 实例（用 esbuild `--external:events` 重打 bundle，或对 `require_events` 做注入）；
- 四个新模块（events/path/string_decoder/timers）不受影响，canonical 事件身份仍达成。

---

## 10. 验收标准

- `go vet ./...`、`go build ./...`、`go test ./...`、`-race` 全绿。
- 各模块 TDD 回归通过（见各 §行为契约）。
- `stream`/`node:stream` 构造器身份一致；与 canonical `events.EventEmitter` 身份一致。
- `stream/web`、fs 回归不受影响。
- 性能：streamx 栈相比 readable-stream 不劣化（benchmark 记录可选）。
