# events 模块

Goja 兼容的 Node `events` 模块（canonical EventEmitter）。

## 模块名

- `require("events")` / `require("node:events")`

## Vendor 来源

- 基底：`bare-events`（https://github.com/holepunchto/bare-events），Apache-2.0
- 改编文件：`events.js`，适配点见文件头部注释
- SHA-256：见下方命令

```bash
sha256sum events.js
```

## 实现说明

- canonical 实例按 runtime 缓存（global symbol），`events.Exports(rt)` 供 streamx facade 复用，保证构造器身份一致。
- EventEmitter 为函数构造器：可 `new`、可 `class extends`、可 `EE.call(this)` + `Object.setPrototypeOf` 组合使用（与 readable-stream/streamx 的原型链操纵兼容）。
- 与 bare-events 的行为差异：
  - `listeners()`/`rawListeners()` 返回纯函数数组（Node 语义）。
  - `error` 事件无监听者时在 `emit` 内同步抛出（Node 语义）。
  - 补充 `getEventListeners`/`setMaxListeners`/`addAbortListener`/`emit`/`SymbolRejection`。

## 依赖

`events.once`/`events.on` 的 `signal` 选项依赖宿主提供 `AbortController`/`AbortSignal`（`abort` 模块）。测试中通过 `abort.Enable(rt)` 安装。
