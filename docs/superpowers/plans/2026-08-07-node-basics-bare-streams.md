# Node 基础模块与 bare-stack classic streams 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 `events`/`path`/`string_decoder`/`timers`(+`timers/promises`)四个模块，并将 `stream`/`node:stream` 引擎从 `readable-stream@4.7.0` 替换为 streamx + canonical events(bare-events 改编)。

**Architecture:** 各模块为独立 Go 包(嵌入 JS 或 Go 原生)，按 runtime 缓存 canonical 实例。`queueMicrotask` 由 eventloop 安装(Promise 实现，走 goja 原生微任务队列)。streamx 栈通过 esbuild 打 IIFE bundle，external 掉 `events`，由 Go 侧 require shim 注入 canonical events。Web 互操作走现有 `streams/integration.go`。

**Tech Stack:** Go 1.23+、Goja、esbuild(devDependency)、streamx@2.28.0、bare-events(Apache-2.0)、b4a/fast-fifo/text-decoder、web-streams-polyfill@4.3.0(不变)。

---

## Phase 0: eventloop 提供 queueMicrotask

### Task 0.1: 安装 queueMicrotask 全局并测试

**Files:**
- Modify: `eventloop/eventloop.go`(NewEventLoop 内)
- Modify: `eventloop/eventloop_test.go`

- [ ] **Step 1: 写失败测试**

```go
// eventloop/eventloop_test.go 追加
func TestQueueMicrotaskOrderAndInterleave(t *testing.T) {
	t.Parallel()
	loop := NewEventLoop(EnableConsole(false))
	defer loop.Stop()
	var result string
	loop.Run(func(vm *goja.Runtime) {
		var err error
		_, err = vm.RunString(`
			var order = [];
			queueMicrotask(function () { order.push("qm1"); });
			Promise.resolve().then(function () { order.push("p1"); });
			queueMicrotask(function () { order.push("qm2"); });
			Promise.resolve().then(function () { order.push("p2"); });
			globalThis.__order = order;
		`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	loop.Run(func(vm *goja.Runtime) {
		result = vm.Get("__order").String()
	})
	if result != "qm1,p1,qm2,p2" {
		t.Fatalf("unexpected order: %s", result)
	}
}

func TestQueueMicrotaskRequiresFunction(t *testing.T) {
	t.Parallel()
	loop := NewEventLoop(EnableConsole(false))
	defer loop.Stop()
	loop.Run(func(vm *goja.Runtime) {
		var failed bool
		_, err := vm.RunString(`queueMicrotask(42);`)
		if err != nil {
			failed = true
		}
		if !failed {
			t.Fatal("expected TypeError")
		}
	})
}

func TestQueueMicrotaskWithinTimeout(t *testing.T) {
	t.Parallel()
	loop := NewEventLoop(EnableConsole(false))
	defer loop.Stop()
	done := make(chan struct{})
	go loop.StartInForeground()
	loop.RunOnLoop(func(vm *goja.Runtime) {
		_ = vm.Set("doneCh", func(goja.FunctionCall) goja.Value {
			close(done)
			return goja.Undefined()
		})
		_, err := vm.RunString(`
			setTimeout(function () {
				queueMicrotask(function () { doneCh(); });
			}, 5);
		`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./eventloop -run 'TestQueueMicrotask' -v
```

Expected: 失败(无 queueMicrotask 全局)。

- [ ] **Step 3: 实现**

`eventloop/eventloop.go` 的 `NewEventLoop` 末尾、`setImmediate` 之后追加：

```go
	_ = vm.Set("queueMicrotask", func(call goja.FunctionCall) goja.Value {
		fn := call.Argument(0)
		if fn == nil || goja.IsUndefined(fn) || goja.IsNull(fn) || !goja.IsFunction(fn) {
			panic(vm.NewTypeError("queueMicrotask expects a function"))
		}
		resolveFn, ok := goja.AssertFunction(vm.Get("Promise").ToObject(vm).Get("resolve"))
		if !ok {
			return goja.Undefined()
		}
		promise := resolveFn(goja.Undefined())
		thenFn, ok := goja.AssertFunction(promise.ToObject(vm).Get("then"))
		if !ok {
			return goja.Undefined()
		}
		if _, err := thenFn(promise.ToObject(vm), fn); err != nil {
			panic(err)
		}
		return goja.Undefined()
	})
```

- [ ] **Step 4: 运行确认通过**

```bash
go test ./eventloop -run 'TestQueueMicrotask' -v
```

Expected: 全部 PASS。

- [ ] **Step 5: 提交**

```bash
git add eventloop/eventloop.go eventloop/eventloop_test.go
git commit -m "feat(eventloop): 提供 queueMicrotask 全局(走 goja 原生微任务队列)"
```

---

## Phase 1: events 模块(canonical)

### Task 1.1: 嵌入改编版 bare-events 并跑通 require

**Files:**
- Create: `events/events.js`(改编 bare-events)
- Create: `events/module.go`
- Create: `events/module_test.go`
- Create: `events/README.md`

- [ ] **Step 1: 写失败测试**

```go
// events/module_test.go
package events

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
)

func newRT(t *testing.T) *goja.Runtime {
	t.Helper()
	rt := goja.New()
	registry := new(require.Registry)
	registry.Enable(rt)
	RegisterWithRegistry(registry)
	return rt
}

func runScript(t *testing.T, rt *goja.Runtime, script string) string {
	t.Helper()
	v, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return v.String()
}

func TestEventEmitterBasicAndInstanceof(t *testing.T) {
	rt := newRT(t)
	out := runScript(t, rt, `
		const { EventEmitter } = require("events");
		const nodeEE = require("node:events").EventEmitter;
		if (EventEmitter !== nodeEE) throw new Error("alias identity");
		const e = new EventEmitter();
		const calls = [];
		e.on("hello", function (x) { calls.push(x); });
		e.emit("hello", "world");
		e.emit("hello", "again");
		JSON.stringify([calls, e instanceof EventEmitter, EventEmitter.EventEmitter === EventEmitter]);
	`)
	if out != `[["world","again"],true,true]` {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestEventEmitterSubclassAndCall(t *testing.T) {
	rt := newRT(t)
	out := runScript(t, rt, `
		const EventEmitter = require("events").EventEmitter;
		class MyEmitter extends EventEmitter {
			constructor() { super(); this.n = 0; }
			bump() { this.n++; this.emit("bumped", this.n); }
		}
		const m = new MyEmitter();
		let last = 0;
		m.on("bumped", (v) => { last = v; });
		m.bump(); m.bump();
		const plain = {};
		EventEmitter.call(plain);
		plain.on("x", function () {});
		const ok = plain.listenerCount("x") === 1;
		JSON.stringify([last, m instanceof EventEmitter, ok]);
	`)
	if out != `[2,true,true]` {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestEventEmitterListenersShape(t *testing.T) {
	rt := newRT(t)
	out := runScript(t, rt, `
		const { EventEmitter } = require("events");
		const e = new EventEmitter();
		function a() {} function b() {}
		e.on("x", a).once("x", b);
		const l = e.listeners("x");
		const r = e.rawListeners("x");
		JSON.stringify([l.length, r.length, l[0] === a, l[1] === b, e.listenerCount("x")]);
	`)
	if out != `[2,2,true,true,2]` {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestEventEmitterUnhandledErrorThrowsSync(t *testing.T) {
	rt := newRT(t)
	_, err := rt.RunString(`
		const { EventEmitter } = require("events");
		const e = new EventEmitter();
		try {
			e.emit("error", new Error("boom"));
			throw new Error("should have thrown");
		} catch (err) {
			if (err.message !== "boom") throw new Error("wrong error: " + err.message);
		}
		"ok";
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestEventsOnceWithSignal(t *testing.T) {
	rt := newRT(t)
	out := runScript(t, rt, `
		const events = require("events");
		const { AbortController } = globalThis;
		const e = new events.EventEmitter();
		const c = new AbortController();
		let settled = "";
		events.once(e, "foo", { signal: c.signal }).then(
			(args) => { settled = "ok:" + args.join(","); },
			(err) => { settled = "aborted:" + err.name; }
		);
		c.abort(new Error("stop"));
		JSON.stringify(settled);
	`)
	if out != `"aborted:AbortError"` {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestEventsAddAbortListener(t *testing.T) {
	rt := newRT(t)
	out := runScript(t, rt, `
		const events = require("events");
		const { AbortController } = globalThis;
		const c = new AbortController();
		let fired = 0;
		events.addAbortListener(c.signal, function () { fired++; });
		c.abort();
		c.abort();
		JSON.stringify(fired);
	`)
	if out != `1` {
		t.Fatalf("unexpected: %s", out)
	}
}
```

> 注意：`TestEventsOnceWithSignal` 与 `TestEventsAddAbortListener` 依赖 `globalThis.AbortController`。测试前用 `abort.Enable(rt)` 安装（见 Step 3），并把该依赖写入 README。若用 eventloop 启动 runtime 则可直接可用。

- [ ] **Step 2: 运行确认失败**

```bash
go test ./events -v
```

Expected: 包不存在，编译失败。

- [ ] **Step 3: 写 `events/events.js`(改编 bare-events)**

以 `https://raw.githubusercontent.com/holepunchto/bare-events/main/index.js` 为基底，应用以下改动（完整内容写入 `events/events.js`，Apache-2.0 保留头部注释与 `lib/errors.js` 内联）：
1. 删除 `const errors = require('./lib/errors')` 顶层 require，改为内联 `lib/errors.js` 的 `EventEmitterError`。
2. `listeners(name)` 返回 `e.list.map((l) => l[0])`(纯函数数组)。
3. `emit` 中 `'error'` 无监听者时同步抛出（替换原 `throwUnhandledError` 的 `queueMicrotask` 路径）：
   ```js
   if (name === 'error' && (this._events === undefined || this._events.error === undefined)) {
     const err = arguments.length > 1 ? arguments[1] : arguments[0];
     throw err === undefined ? new Error('Unhandled error.') : err;
   }
   ```
4. 追加静态/模块级函数：
   - `exports.getEventListeners = function (emitter, name) { return emitter.rawListeners(name); }`
   - `exports.addAbortListener = function (signal, listener) { if (signal.aborted) { listener(); return { unsubscribe() {} }; } signal.addEventListener('abort', listener); return { unsubscribe() { signal.removeEventListener('abort', listener); } }; }`
   - `exports.emit = function (emitter, ...args) { return emitter.emit(...args); }`
   - `exports.captureRejections = false`(作为 `Symbol.for('nodejs.rejection')` 的宿主开关，本版本提供符号即可)
   - `exports.SymbolRejection = Symbol.for('nodejs.rejection')`
5. 保留：`exports.on`、`exports.once`、`exports.forward`、`exports.listenerCount`、`exports.getMaxListeners`、`exports.setMaxListeners`、`exports.defaultMaxListeners`、`exports.EventEmitter = exports`、`exports.errors`。

> goja 兼容性：bare-events 为 ES2015 class + rest/默认参数，goja 可解析；不含 async/await。`queueMicrotask` 在本实现中不再被调用。

- [ ] **Step 4: 写 `events/module.go`**

```go
package events

import (
	_ "embed"
	"fmt"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
)

const ModuleName = "events"

//go:embed events.js
var eventsSource string

var (
	moduleSymbol  = goja.NewSymbol("goja_ext.events.module")
	eventsProgram = mustCompile()
)

func mustCompile() *goja.Program {
	source := `(function () {
		var module = { exports: {} };
		var exports = module.exports;
` + eventsSource + `
		return module.exports;
	})()`
	program, err := goja.Compile("goja_ext/events/events.js", source, false)
	if err != nil {
		panic(fmt.Errorf("events: compile: %w", err))
	}
	return program
}

func Require(rt *goja.Runtime, module *goja.Object) {
	if err := module.Set("exports", Exports(rt)); err != nil {
		panic(err)
	}
}

// Exports returns the canonical events module exports for rt.
func Exports(rt *goja.Runtime) *goja.Object {
	global := rt.GlobalObject()
	if value := global.GetSymbol(moduleSymbol); value != nil &&
		!goja.IsUndefined(value) && !goja.IsNull(value) {
		if exports, ok := value.(*goja.Object); ok {
			return exports
		}
	}
	value, err := rt.RunProgram(eventsProgram)
	if err != nil {
		panic(fmt.Errorf("events: initialize: %w", err))
	}
	exports, ok := value.(*goja.Object)
	if !ok {
		panic("events: did not return an exports object")
	}
	if err := global.SetSymbol(moduleSymbol, exports); err != nil {
		panic(err)
	}
	return exports
}

// RegisterWithRegistry registers events/node:events on a specific registry.
func RegisterWithRegistry(registry *require.Registry) {
	registry.RegisterNativeModule(ModuleName, Require)
	registry.RegisterNativeModule(require.NodePrefix+ModuleName, Require)
}

func init() {
	require.RegisterCoreModule(ModuleName, Require)
}
```

并在测试 `newRT` 中安装 AbortController：

```go
import "github.com/sealdice/goja_ext/abort"
// newRT 中追加：
abort.Enable(rt)
```

- [ ] **Step 5: 运行确认通过**

```bash
go test ./events -v
```

Expected: 全部 PASS。

- [ ] **Step 6: 记录 vendor 信息**

写 `events/README.md`：来源 `bare-events`(Apache-2.0)、SHA-256、改编点列表、AbortController 依赖说明。

```bash
sha256sum events/events.js
```

- [ ] **Step 7: 提交**

```bash
git add events/
git commit -m "feat(events): 新增 canonical EventEmitter 模块(bare-events 改编)"
```

---

## Phase 2: path 模块

### Task 2.1: 实现 path 并测试

**Files:**
- Create: `path/module.go`
- Create: `path/path_test.go`
- Create: `path/README.md`

- [ ] **Step 1: 写失败测试**

```go
// path/path_test.go
package path

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
)

func run(t *testing.T, script string) string {
	t.Helper()
	rt := goja.New()
	registry := new(require.Registry)
	registry.Enable(rt)
	v, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return v.String()
}

func TestPathPosixFunctions(t *testing.T) {
	out := run(t, `
		const p = require("path");
		JSON.stringify([
			p.join("/a", "/b", "c"),
			p.resolve("/a", "b", "..", "c"),
			p.normalize("/a//b/./c/../d"),
			p.basename("/a/b/c.txt"),
			p.basename("/a/b/c.txt", ".txt"),
			p.dirname("/a/b/c.txt"),
			p.extname("/a/b/c.txt"),
			p.isAbsolute("/a"),
			p.relative("/a/b/c", "/a/x/y"),
			p.sep, p.delimiter,
			p.posix.join("a", "b"),
			p.win32.join("C:\\a", "b"),
		]);
	`)
	want := `["/a/b/c","/a/c","/a/b/d","c.txt","c","/a/b",".txt",true,"../../x/y","/",":","a/b","C:\\a\\b"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestPathParseFormat(t *testing.T) {
	out := run(t, `
		const p = require("path");
		const parsed = p.parse("/a/b/c.txt");
		const formatted = p.format(parsed);
		JSON.stringify([parsed.root, parsed.dir, parsed.base, parsed.name, parsed.ext, formatted]);
	`)
	want := `["/","/a/b","c.txt","c",".txt","/a/b/c.txt"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestPathWin32(t *testing.T) {
	out := run(t, `
		const w = require("path").win32;
		JSON.stringify([
			w.parse("C:\\Users\\x\\a.txt").name,
			w.isAbsolute("C:\\x"),
			w.isAbsolute("\\x"),
			w.resolve("C:\\a", "..\\b"),
			w.normalize("C:\\a\\..\\b\\"),
			w.basename("C:\\a\\b.txt"),
		]);
	`)
	want := `["a",true,true,"C:\\b","C:\\b","b.txt"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./path -v
```

- [ ] **Step 3: 实现 `path/module.go`**

Go 实现，posix 基于 `path` 包，win32 单独实现。导出对象含 `join/resolve/normalize/relative/isAbsolute/basename/dirname/extname/parse/format/sep/delimiter/toNamespacedPath/posix/win32`；`posix` 与 `win32` 为独立子对象。注册 `path`/`node:path`。核心算法：

- `posix`：直接用标准库 `path` 的 `Join/Resolve/Normalize/Base/Dir/Ext/Rel` + 自写 `parse/format` 与 `isAbsolute`（前缀 `/`）。
- `win32`：自实现（盘符 `[A-Za-z]:[\\/]`、UNC `//server/share`、`\` 与 `/` 均可作分隔符、`normalize` 保留盘符、`resolve` 以盘符锚定）。

- [ ] **Step 4: 运行确认通过**

```bash
go test ./path -v
```

- [ ] **Step 5: 提交**

```bash
git add path/
git commit -m "feat(path): 新增 Node path 模块(posix/win32)"
```

---

## Phase 3: string_decoder 模块

### Task 3.1: 实现 StringDecoder 并测试

**Files:**
- Create: `string_decoder/module.go`
- Create: `string_decoder/module_test.go`
- Create: `string_decoder/README.md`

- [ ] **Step 1: 写失败测试**

```go
// string_decoder/module_test.go
package string_decoder

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/buffer"
	"github.com/sealdice/goja_ext/require"
)

func run(t *testing.T, script string) string {
	t.Helper()
	rt := goja.New()
	registry := new(require.Registry)
	registry.Enable(rt)
	buffer.Enable(rt)
	v, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return v.String()
}

func TestStringDecoderSplitUTF8(t *testing.T) {
	out := run(t, `
		const { StringDecoder } = require("string_decoder");
		const buf = Buffer.from([0xE4, 0xBD, 0xA0, 0xE5, 0xA5, 0xBD]); // "你好"
		const d = new StringDecoder("utf8");
		const a = d.write(buf.subarray(0, 2)); // 不完整前缀 -> ""
		const b = d.write(buf.subarray(2, 4));
		const c = d.end(buf.subarray(4));
		JSON.stringify([a, b, c]);
	`)
	want := `["","","你好"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestStringDecoderIncompleteAtEnd(t *testing.T) {
	out := run(t, `
		const { StringDecoder } = require("string_decoder");
		const buf = Buffer.from([0xE4, 0xBD]); // 不完整
		const d = new StringDecoder("utf8");
		const a = d.write(buf);
		const b = d.end();
		JSON.stringify([a, b]);
	`)
	want := `["","�"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestStringDecoderTextAndFillLast(t *testing.T) {
	out := run(t, `
		const { StringDecoder } = require("string_decoder");
		const d = new StringDecoder("utf8");
		const a = d.write(Buffer.from("he"));
		const b = d.text(Buffer.from("llo"));
		JSON.stringify([a, b, d.encoding]);
	`)
	want := `["he","llo","utf8"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestStringDecoderHex(t *testing.T) {
	out := run(t, `
		const { StringDecoder } = require("string_decoder");
		const d = new StringDecoder("hex");
		const a = d.write(Buffer.from([0x48, 0x49]));
		const b = d.end();
		JSON.stringify([a, b]);
	`)
	want := `["4849",""]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./string_decoder -v
```

- [ ] **Step 3: 实现 `string_decoder/module.go`**

Go 实现 `StringDecoder`：内部保存 pending 字节；`utf8` 用 `utf8.DecodeRune` 解码，不完整序列保留到下一 chunk，`end()` 时以 U+FFFD 替换；`hex`/`base64`/`ascii`/`latin1`/`ucs2` 通过 `buffer` 模块编解码。构造器用 `goja.ConstructorCall` 模式（参考 `abort`/`url`）。注册 `string_decoder`/`node:string_decoder`。

- [ ] **Step 4: 运行确认通过**

```bash
go test ./string_decoder -v
```

- [ ] **Step 5: 提交**

```bash
git add string_decoder/
git commit -m "feat(string_decoder): 新增 StringDecoder 模块"
```

---

## Phase 4: timers 模块

### Task 4.1: timers 与 timers/promises 并测试

**Files:**
- Create: `timers/module.go`
- Create: `timers/module_test.go`
- Create: `timers/README.md`

- [ ] **Step 1: 写失败测试**

```go
// timers/module_test.go
package timers

import (
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/eventloop"
)

func runWithLoop(t *testing.T, script string) string {
	t.Helper()
	loop := eventloop.NewEventLoop(eventloop.EnableConsole(false))
	go loop.StartInForeground()
	t.Cleanup(func() { loop.Stop() })

	done := make(chan struct{})
	var result string
	loop.RunOnLoop(func(vm *goja.Runtime) {
		defer func() {
			loop.SetTimeout(func(vm *goja.Runtime) {
				result = vm.Get("__result").String()
				close(done)
			}, 5*time.Millisecond)
		}()
		if _, err := vm.RunString(script); err != nil {
			t.Errorf("run: %v", err)
			close(done)
		}
	})
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
	return result
}

func TestTimersModuleSurface(t *testing.T) {
	out := runWithLoop(t, `
		const t = require("timers");
		globalThis.__result = String([
			typeof t.setTimeout, typeof t.setInterval, typeof t.setImmediate,
			typeof t.clearTimeout, typeof t.clearInterval, typeof t.clearImmediate,
		].join(","));
	`)
	if out != "function,function,function,function,function,function" {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestTimersPromisesSetTimeout(t *testing.T) {
	out := runWithLoop(t, `
		const { setTimeout, setImmediate } = require("timers/promises");
		Promise.all([
			setTimeout(1, "a"),
			setImmediate("b"),
		]).then(function (vals) {
			globalThis.__result = String(vals.join(","));
		});
	`)
	if out != "a,b" {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestTimersPromisesSetInterval(t *testing.T) {
	out := runWithLoop(t, `
		const { setInterval } = require("timers/promises");
		const it = setInterval(1, "x");
		const collected = [];
		const run = function () {
			it.next().then(function (r) {
				if (r.done) { globalThis.__result = collected.join(","); return; }
				collected.push(r.value);
				run();
			});
		};
		run();
		setTimeout(function () { it.return(); }, 10);
	`)
	if out != "x,x,x" {
		t.Fatalf("unexpected: %s", out)
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./timers -v
```

- [ ] **Step 3: 实现 `timers/module.go`**

读取 runtime 全局 timer（eventloop 已装）。缺失时导出包装函数，调用即抛 `"timers require an event loop"`。`timers/promises` 用嵌入 JS 实现（Promise 包装 + setInterval async iterator）。注册 4 个名字：`timers`/`node:timers`/`timers/promises`/`node:timers/promises`。

```js
// timers/promises.js
const timerFns = {}; // 由 Go 注入 setTimeout/setInterval/setImmediate
function makeTimeout(timerFn) {
	return function (delay, value, options) {
		return new Promise(function (resolve) {
			timerFn(function () { resolve(value); }, delay);
		});
	};
}
function makeSetInterval(intervalFn) {
	return function (delay, value, options) {
		const it = {};
		let finished = false;
		let pendingResolve = null;
		let pendingValue = null;
		it.next = function () { ... };
		it.return = function () { finished = true; intervalFn && clearInterval(...); return Promise.resolve({ value: undefined, done: true }); };
		...
		return it;
	};
}
module.exports = {
	setTimeout: makeTimeout(...), setImmediate: makeImmediate(...), setInterval: makeSetInterval(...),
	scheduler: { wait: ... },
};
```

- [ ] **Step 4: 运行确认通过**

```bash
go test ./timers -v
```

- [ ] **Step 5: 提交**

```bash
git add timers/
git commit -m "feat(timers): 新增 timers 与 timers/promises 模块"
```

---

## Phase 5: streams/node 引擎替换为 streamx

### Task 5.1: 写 esbuild 构建脚本并生成 bundle

**Files:**
- Create: `package.json`(若已存在则 modify scripts)
- Create: `scripts/build-node-streams.mjs`
- Create: `streams/internal/streamx/facade.js`(改编自 bare-stream/index.js，去掉 `./web` 依赖)
- Create: `streams/internal/streamx/bundle.js`(构建产物)
- Create: `streams/internal/streamx/README.md`、`streams/internal/streamx/LICENSE`

- [ ] **Step 1: 安装 esbuild devDependency**

```bash
npm install --save-dev esbuild
```

- [ ] **Step 2: 写 `streams/internal/streamx/facade.js`**

改编 `bare-stream/index.js`(Apache-2.0)：
- 去掉 `require('./web')` 与 `ReadableStream`/`WritableStream` 引用。
- `Readable.toWeb`/`Writable.toWeb`/`Duplex.toWeb/fromWeb` 等 web 桥删除，改为调用全局注入的 `__goja_ext_streams_to_web` / `__goja_ext_streams_from_web`(由 Go 提供，见 Task 5.4)。
- 保留 `b4a` 字符串编码转换、`closed`/`errored` getter、`pipeline`、`finished`、`addAbortSignal`、谓词、`duplexPair`。

- [ ] **Step 3: 写 `scripts/build-node-streams.mjs`**

```js
import { build } from "esbuild";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);

await build({
  entryPoints: ["streams/internal/streamx/facade.js"],
  outfile: "streams/internal/streamx/bundle.js",
  bundle: true,
  format: "iife",
  target: "es2015",
  globalName: "GojaNodeStream",
  platform: "browser",
  external: ["events"],
  alias: { "events-universal": "events" },
  define: { "process.env.NODE_ENV": '"production"' },
});
```

> 说明：`streamx` 依赖 `events-universal`，通过 alias 指向 `events`，使其在 bundle 中保持为外部 `require('events')`，由 Go 侧 require shim 解析为 canonical events。

- [ ] **Step 4: 运行构建**

```bash
npm run build:node-streams
```

Expected: 生成 `streams/internal/streamx/bundle.js`，且其中 `require("events")` 保留为外部调用（`import` 或 `require`）。

- [ ] **Step 5: 记录 vendor 信息**

`streams/internal/streamx/README.md`：来源(streamx@2.28.0、bare-stream@2.13.3、b4a、fast-fifo、text-decoder、teex)、LICENSE、SHA-256；`LICENSE` 合并各包许可(Apache-2.0 + MIT)。

- [ ] **Step 6: 提交**

```bash
git add package.json package-lock.json scripts/ streams/internal/streamx/
git commit -m "build(streams): 新增 streamx 栈 bundle 构建脚本与产物"
```

### Task 5.2: Go 集成(module.go 换 bundle + require shim)

**Files:**
- Modify: `streams/node/module.go`
- Create: `streams/node/polyfill.go`(embed + require shim，替代 `mustCompilePolyfill` 的旧实现)
- Modify: `streams/node/module_test.go`

- [ ] **Step 1: 更新 `mustCompilePolyfill` 逻辑**

新 bundle 以 `GojaNodeStream` 全局名存在，Go 侧包装为：

```go
//go:embed internal/streamx/bundle.js
var streamxBundleSource string

func mustCompilePolyfill() *goja.Program {
	source := `(function (__require) {
		(function () {
` + streamxBundleSource + `
		})();
		return GojaNodeStream;
	})(function (name) {
		if (name === "events") return __goja_ext_canonical_events;
		if (name === "streamx") return ...;
		throw new Error("unresolved require: " + name);
	});`
	...
}
```

并在 `Exports(rt)` 中注入 canonical events(`events.Exports(rt)`)、AbortController/AbortSignal(沿用现有 `self` 注入)、必要时 `queueMicrotask` 兜底。缓存仍走 global symbol。

- [ ] **Step 2: 更新测试断言**

现有 `TestNodeTransformAndEvents` 等应在 streamx 语义下继续通过(facade 吸收差异)。若个别失败，记录到 `streams/node/README.md` 差异表并调整断言。

- [ ] **Step 3: 运行确认**

```bash
go test ./streams/... -v
```

- [ ] **Step 4: 提交**

```bash
git add streams/node/
git commit -m "feat(streams): stream/node:stream 引擎切换为 streamx + canonical events"
```

### Task 5.3: Web 互操作 fromWeb/toWeb(Go 桥)

**Files:**
- Create: `streams/node/from_web.go`

- [ ] **Step 1: 写失败测试**

```go
// streams/node/from_web_test.go
func TestReadableToWeb(t *testing.T) {
	// 用 eventloop + streams + node facade 装配
	// new stream.Readable(...) → toWeb → 用 canonical ReadableStream reader 读取 → 断言 chunk
}
func TestReadableFromWeb(t *testing.T) {
	// canonical new ReadableStream(...) → fromWeb → 挂 data/end 监听 → 断言
}
func TestWritableToWeb(t *testing.T) {
	// canonical WritableStream writer.write → toWeb 的 streamx writable 收到 chunk
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./streams/node -run 'TestReadableToWeb|TestReadableFromWeb|TestWritableToWeb' -v
```

- [ ] **Step 3: 实现**

在 `Exports(rt)` 装配后，把 `toWeb`/`fromWeb` 挂到 `Readable`/`Writable`/`Duplex` 上：
- `Readable.toWeb(readable)` → `streams.NewReadableStream(rt, ReadableStreamSource{Pull: ...})`，Pull 通过 streamx 的 `[Symbol.asyncIterator]`/`read()` 取 chunk，null 时返回 Promise 等待 `readable`/`end` 事件后 `controller.close()`。
- `Writable.toWeb(writable)` → `streams.NewWritableStream(rt, WritableStreamSink{Write: ...})`，Write 调 `writable.write(chunk)`，利用 `stream.Writable.drained` 实现背压。
- `Readable.fromWeb(webStream)` / `Writable.fromWeb(webStream)` → 用 `streams.ConsumeReadableStream` 读入并 `push` 到 streamx readable；`addAbortSignal` 传播。

- [ ] **Step 4: 运行确认通过**

```bash
go test ./streams/node -run 'TestReadableToWeb|TestReadableFromWeb|TestWritableToWeb' -v
```

- [ ] **Step 5: 提交**

```bash
git add streams/node/
git commit -m "feat(streams): streamx facade 的 fromWeb/toWeb Go 桥"
```

### Task 5.4: 移除 readable-stream vendored 包(可选收尾)

**Files:**
- Delete: `streams/internal/nodepolyfill/`

- [ ] **Step 1: 确认无引用后删除**

```bash
rg -n "nodepolyfill" streams/ --type go
```

Expected: 无引用。

- [ ] **Step 2: 删除并提交**

```bash
git rm -r streams/internal/nodepolyfill/
git commit -m "chore(streams): 移除 readable-stream vendored 包"
```

---

## Phase 6: 验收与文档

### Task 6.1: 全仓验证

- [ ] **Step 1: 运行**

```bash
go vet ./...
go build ./...
go test ./... -count=1
go test ./... -race -count=1
```

Expected: 全绿。

- [ ] **Step 2: 提交(如有修复)**

```bash
git add -A
git commit -m "chore: 修复验收问题"
```

### Task 6.2: 更新规格与 README

**Files:**
- Modify: `功能规格说明书.md`(§3.0.2 引擎说明、能力矩阵、已知差异)
- Modify: `docs/goja-node-basics-bare-streams-design.md`(标记完成状态)
- Modify: `streams/node/README.md`(引擎、版本、语义差异表)
- Modify: `events/README.md`、`path/README.md`、`string_decoder/README.md`、`timers/README.md`(已建)

- [ ] **Step 1: 更新文档并提交**

```bash
git add -A
git commit -m "docs: 更新功能规格说明书与模块 README(streamx 引擎/差异/版本)"
```

---

## 自检记录

- 无 TBD/TODO 占位。
- 每 Phase 结束提交一次；失败路径有明确 go/no-go(Phase 5 Task 5.2 若 bundle 无法运行则启用回退方案：保留 readable-stream + 仅 external 化 events)。
- 类型/命名一致：`events.Exports(rt)`、`streams.Exports(rt)`、`require.NodePrefix` 均沿用既有导出。
