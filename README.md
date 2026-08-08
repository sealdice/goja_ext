# goja_ext

goja_ext 是一组可按需组合的 Go 模块，为 Goja 提供 Node.js 风格的 CommonJS
模块、Web Platform API，以及文件系统、事件循环和网络等宿主能力。

它不是完整的 Node.js 运行时，也不包含 V8、libuv 或 npm。这里的 Node 兼容指的是
“在 Goja 中提供一组常用、可嵌入的 Node API 子集”。每个模块都可以单独启用；带有
异步行为的模块必须绑定到创建该 Goja runtime 的事件循环。

本文先面向只阅读 JavaScript API 的使用者，再介绍 Go 宿主如何创建 runtime 和配置
模块。模块目录下还有更细的说明：[`fetch/README.md`](fetch/README.md)、
[`fs/README.md`](fs/README.md)、[`streams/README.md`](streams/README.md) 和
[`websocket/README.md`](websocket/README.md) 等。

## 快速判断

- 只需要 URL、事件、Buffer、路径等同步能力：启用对应模块即可，不需要 event loop。
- 需要 setTimeout、Promise timers、Fetch、SSE、FS 异步 API、WebSocket 或流式
  网络处理：使用 eventloop.NewEventLoop，并在 loop 所属 runtime 中安装模块。
- 需要 Node classic stream 或 FS 的 createReadStream/createWriteStream：
  显式导入 github.com/sealdice/goja_ext/streams/node，并为 FS 设置
  fs.WithStreams(true)。
- 需要完整 Node 内置模块、npm 包、process.argv、child_process 等能力：本项目
  不是合适的替代品，应使用真正的 Node.js 运行时。

## JavaScript API 使用者

### 模块总览

所有下表中的 core module 都支持 require("模块名")。由 require.RegisterCoreModule
注册的模块同时支持 require("node:模块名")；同一个 runtime 内两种写法返回同一份
导出对象。global 一栏表示调用 Go 的 Enable 后会额外安装的全局对象。

| 模块 | JavaScript 入口 | 提供的功能 | 与 Node.js 的主要区别 |
| --- | --- | --- | --- |
| require | require("./x")、require("pkg") | CommonJS、.js/.json、node_modules、Go 原生模块 | 只有宿主提供的 source loader 和文件内容可被加载；不自动执行 npm 安装，也没有 Node 的全部内置模块 |
| events | require("events") | EventEmitter、once、on、监听器管理、Abort signal 集成 | 基于 bare-events 改编；行为以项目实现为准，带 signal 的部分 API 需要 abort |
| buffer | require("buffer")；global: Buffer | Buffer 构造、from、alloc、concat、数字读写和编码转换 | 只承诺当前 codec 表；Web Streams 和 FS 字节结果仍是 Uint8Array，不会自动变成 Buffer |
| console | require("console")；global: console | log、info、warn、error、debug、trace | 输出交给 Go 的 Printer；格式化是轻量 util.format，不是 Node 完整 console |
| abort | require("abort")；global: AbortController、AbortSignal | 取消控制、timeout、throwIfAborted | 面向 Fetch 等模块的轻量实现，不承诺浏览器/Node 完整 Abort API |
| fetch | require("fetch")；global: fetch、EventSource、Headers、Request、Response、FormData | HTTP Fetch、请求体、响应体 Web Stream、SSE、超时、代理、取消 | HTTP 由 Go Resty 执行；mode/credentials 等浏览器约束不完整，必须有 event loop |
| fs | require("fs")、require("fs/promises")；global: fs | Afero-backed Deno 风格文件 API、Node callback/Promise wrapper、Stats、文件流 | 后端由宿主注入；symlink/hard link 等能力必须显式提供；watch、文件锁、terminal 不在当前子集；字节为 Uint8Array |
| path | require("path")；另有 posix、win32 | 路径拼接、解析、规范化、相对路径、盘符和 UNC | resolve 读取 runtime-local cwd，不调用宿主进程 chdir；只实现列出的 path API |
| process | require("process")；global: process | env 快照、cwd、runtime-local chdir | 当前不含 nextTick、argv、platform 等进程控制信息；不会修改宿主进程 cwd |
| streams | require("streams")、require("stream/web")；global: Web Streams | WHATWG ReadableStream、WritableStream、TransformStream、BYOB、reader/writer、文本流 | 使用嵌入的 web-streams-polyfill；流块通常是原生 Uint8Array，不依赖 Buffer |
| Node streams | require("stream") | Readable、Writable、Duplex、Transform、PassThrough、pipeline、Web adapter | 基于嵌入的 streamx，不是 Node 原生实现；字符串 chunk 转成 Uint8Array，read(0) 不支持，且需要显式注册 |
| string_decoder | require("string_decoder") | 跨 chunk 的 utf8、utf16le/ucs2、base64、hex、latin1 解码 | 独立的 Go 实现；streamx 内部使用自己的 text-decoder，不自动接管经典流内部 |
| timers | require("timers")、require("timers/promises") | timeout、interval、immediate、Promise timers、async iterator、scheduler.wait | 必须有 event loop；顶层脚本中解构 Promise timers 的 setTimeout 可能触发 Goja TDZ 冲突 |
| url | require("url")；global: URL、URLSearchParams | URL 解析、查询参数、域名 ASCII/Unicode 转换 | Go 原生 Node 风格子集；默认端口会规范化，完整 WHATWG 边界行为不以 Node 版本兼容为承诺 |
| util | require("util") | format、inspect、循环引用检测和深度控制 | 只覆盖项目需要的占位符和 inspect 选项，不保证与 Node 输出逐字符一致 |
| structuredclone | require("structuredclone")；global: structuredClone | 复制数组、对象、Map、Set、Date、RegExp、ArrayBuffer、TypedArray 和循环引用 | 不支持函数、Symbol、WeakMap、WeakSet、Promise 及未知宿主对象，会抛 DataCloneError |
| websocket | global: WebSocket | WebSocket 客户端、事件监听、文本消息、协议、关闭和连接管理 | 不是 require core module；底层是 Gorilla WebSocket，主要覆盖客户端基础生命周期，必须有 event loop |

### 常用 JavaScript 写法

启用模块后，JavaScript 侧的调用方式尽量保持 Node/Web API 的习惯：

~~~javascript
const { URL, URLSearchParams } = require("url");
const { EventEmitter } = require("events");
const { Buffer } = require("buffer");
const { inspect } = require("util");

const url = new URL("https://example.com/search?q=goja");
const query = new URLSearchParams(url.search);
const emitter = new EventEmitter();
emitter.on("message", (value) => console.log(value));

const bytes = Buffer.from("hello", "utf8");
console.log(query.get("q"), inspect(bytes));
~~~

启用 abort 和 fetch 后可以取消异步请求：

fetch.Enable 会安装 Headers、Request、Response、FormData；真正的 fetch 函数和
EventSource 需要 Go 宿主再调用 EnableFetch，并提供 event loop。

~~~javascript
const controller = new AbortController();

fetch("https://example.com/data", { signal: controller.signal })
  .then((response) => response.json())
  .then((data) => console.log(data))
  .catch((reason) => console.error(reason));

setTimeout(() => controller.abort("request cancelled"), 1000);
~~~

Fetch 的 Response.body 是 Web ReadableStream，读取时得到 Uint8Array：

~~~javascript
(async () => {
  const response = await fetch("https://example.com/data");
  const reader = response.body.getReader();

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    // value 是 Uint8Array；需要字符串时显式使用 TextDecoderStream 或 Buffer。
    console.log(value.byteLength);
  }
})();
~~~

Node classic streams 是可选表面。宿主完成 streams/node 注册后，才可以使用：

~~~javascript
const { Readable } = require("stream");

const source = Readable.from(["hello", " ", "streams"]);
const web = Readable.toWeb(source);
const reader = web.getReader();
~~~

### 各模块的使用要点

#### require 与原生模块

require 支持相对路径、绝对路径、node_modules 查找、package.json 的 main，
以及 .js 和 .json 文件。Go 宿主可把自己的能力注册成原生模块，JavaScript
侧像普通 CommonJS 模块一样使用：

~~~javascript
const host = require("host");
host.log("hello from JavaScript");
~~~

它不会替你安装或解析 npm 依赖；默认 source loader 读取宿主文件系统，沙箱场景应
使用自定义 WithLoader 和 WithPathResolver。

#### events、abort 与 console

events 提供 canonical EventEmitter。require("events").EventEmitter 与 Node
streams facade 使用同一个构造器，因此可以安全地混用事件和流对象。带 signal 的
events.once/events.on 需要先安装 abort。

console 的输出通过 Go Printer 接口接收；eventloop.NewEventLoop() 默认安装
console，也可以用 eventloop.EnableConsole(false) 关闭 runtime 内的全局对象。

#### buffer 与字节边界

需要 Node Buffer 语义时显式启用 buffer，并优先使用 Buffer.alloc()、
Buffer.from()。但是 Web Streams、Fetch 响应体和 FS 读操作的结果是原生
Uint8Array；它们不会因为 Buffer 模块已加载就自动转换。

#### fs、path 与 process

fs 有两种表面：

- fs.Enable* 安装 Deno 风格的全局 fs，包括同步/异步文件操作、目录、临时文件、
  FsFile 和 WHATWG 流。
- require("fs") 和 require("fs/promises") 提供 Node 风格 wrapper。最后一个参数
  是函数时使用 callback，否则返回 Promise。

path.resolve()、fs.cwd() 和 process.cwd() 共享 runtime-local cwd。调用
process.chdir() 或 fs.chdir() 不会改变 Go 进程的工作目录，也不会影响其他 runtime。

#### Web Streams 与 Node streams

streams.Enable(rt) 安装 Web Streams 全局构造器；require("stream/web") 返回同一
份 canonical 导出。Node classic streams 由 streams/node 包通过 blank import 注册，
并且只在首次使用 Web adapter 时加载 Web Streams。两套流不能假定 chunk 类型与 Node
完全相同：classic stream 的字符串 chunk 会转换为 Uint8Array。

#### timers

event loop 会安装全局 setTimeout、setInterval、setImmediate、对应的清理函数
和 queueMicrotask。require("timers/promises") 只是对这些调度器的 Promise 封装，
不会创建第二套计时器。

#### url、util 与 structuredclone

这些模块适合常见脚本和数据处理，但不是完整标准库替代品。特别是 util.inspect
的输出格式、structuredClone 的可复制对象范围和 URL 的边界解析，应按项目当前
测试和源码行为验证，不要仅凭 Node 文档推断全部细节。

#### websocket

new WebSocket(url) 创建客户端连接，事件通过 open、message、close、error
触发。可以使用 addEventListener/removeEventListener 或 onopen 等属性。
默认 TLS 会校验证书和主机名；私有 CA、客户端证书或测试拨号器要由 Go 宿主注入。

## Go 宿主接入

### 安装与版本

~~~bash
go get github.com/sealdice/goja_ext/eventloop
~~~

项目 go.mod 声明 Go 1.23，并指定 toolchain go1.24.5；开发和部署建议使用
Go 1.24.5 或更新版本。

### 创建一个组合 runtime

下面的代码安装 CommonJS、Abort、Fetch、Process 和 Web Streams，并在同一个事件循环
中运行 JavaScript：

~~~go
package main

import (
    "github.com/dop251/goja"
    "github.com/sealdice/goja_ext/abort"
    "github.com/sealdice/goja_ext/eventloop"
    "github.com/sealdice/goja_ext/fetch"
    "github.com/sealdice/goja_ext/process"
    "github.com/sealdice/goja_ext/require"
    "github.com/sealdice/goja_ext/streams"
)

func main() {
    registry := require.NewRegistry()
    loop := eventloop.NewEventLoop(
        eventloop.EnableConsole(true),
        eventloop.WithRegistry(registry),
    )

    loop.Run(func(rt *goja.Runtime) {
        abort.Enable(rt)
        fetch.Enable(rt)
        process.Enable(rt)
        streams.Enable(rt)

        if err := fetch.EnableFetch(rt, loop); err != nil {
            panic(err)
        }

        value, err := rt.RunString(`
            fetch("https://example.com/")
              .then((response) => response.text())
              .then((text) => console.log(text.length));
        `)
        if err != nil {
            panic(err)
        }
        _ = value
    })
}
~~~

实际服务通常使用 loop.Start()/loop.StartInForeground()，再通过
loop.RunOnLoop(func(*goja.Runtime) { ... }) 调度工作：

~~~go
loop := eventloop.NewEventLoop(eventloop.EnableConsole(false))
loop.Start()
defer loop.Stop()

if ok := loop.RunOnLoop(func(rt *goja.Runtime) {
    // 安装模块、调用 RunString、读取或修改 Goja 值
}); !ok {
    panic("event loop is not running")
}
~~~

### 模块安装方式

大多数 core module 在 Go 包的 init() 中已注册到全局 core module 表。宿主只需在
所属 runtime 中调用 Enable，或直接使用 require：

~~~go
abort.Enable(rt)
buffer.Enable(rt)
url.Enable(rt)

// 等价地，使用 require("abort")、require("buffer")、require("url")。
~~~

Enable 和 require(...) 在同一 runtime 内共享 canonical 构造器和导出对象，避免
AbortController、ReadableStream、EventEmitter 等出现多份身份不一致的实现。

Fetch 和 WebSocket 等需要调度器的能力必须传入所属 loop：

~~~go
fetch.Enable(rt)
if err := fetch.EnableFetch(rt, loop); err != nil {
    return err
}

if err := websocket.Enable(rt, loop); err != nil {
    return err
}
~~~

Fetch 的 WithTimeout、WithProxy、WithHTTPClient、WithTransport 和 WithRestyClient
只影响 Go 侧 HTTP 执行器，不会暴露为 JavaScript 全局属性。WebSocket 的 TLS、拨号器、
日志器和连接管理器可通过 EnableWithOptions 注入；默认 TLS 会校验证书和主机名。

### 配置 FS 与 Node 文件流

FS 后端由 Afero 注入，默认是 afero.OsFs；内存测试或沙箱常用
afero.NewMemMapFs()。配置应在同一 runtime/registry 中保持一致：

~~~go
backend := afero.NewOsFs()
options := []fs.Option{
    fs.WithFS(backend),
    fs.WithCwd("/workspace"),
    fs.WithStreams(false),
}

registry := require.NewRegistry()
loop := eventloop.NewEventLoop(
    eventloop.EnableConsole(false),
    eventloop.WithRegistry(registry),
)
fs.RegisterWithLoop(registry, loop, options...)

loop.Run(func(rt *goja.Runtime) {
    if err := fs.EnableWithLoop(rt, loop, options...); err != nil {
        panic(err)
    }
    process.Enable(rt)
})
~~~

需要 require("fs") 的 createReadStream/createWriteStream 或 FsFile 的
readable/writable 时，同时：

~~~go
import _ "github.com/sealdice/goja_ext/streams/node"

options := []fs.Option{
    fs.WithFS(backend),
    fs.WithCwd("/workspace"),
    fs.WithStreams(true),
}
~~~

fs.WithExtraCapabilities 可以显式注入 lstat、readlink、symlink 和 hard link
能力；Afero 不支持这些接口时，相关调用返回 ENOSYS，不会伪造 Node 结果。

### 注册自定义 CommonJS/Go 模块

使用 registry 可以让原生模块只对指定 runtime 集合可见；使用包级函数注册则是全局
core module：

~~~go
registry.RegisterNativeModule("host", func(rt *goja.Runtime, module *goja.Object) {
    exports := module.Get("exports").ToObject(rt)
    _ = exports.Set("log", func(call goja.FunctionCall) goja.Value {
        fmt.Println(call.Argument(0).String())
        return goja.Undefined()
    })
})
~~~

require.NewRegistry 还可配置 WithLoader、WithPathResolver 和 WithGlobalFolders，
适合把模块文件放在内存、数据库或受控沙箱中。

### 仅供 Go 使用的支持包

- eventloop：创建 runtime、调度定时器和 microtask、串行化 Goja 调用，并提供 logger
  和生命周期管理；它本身不是 JavaScript 模块。
- runtimehost：保存 runtime-local 的 scheduler、cwd 和模块状态，适合自定义宿主
  实现 `runtimehost.Scheduler` 或复用共享 cwd。
- errors：生成带 Node 风格 code 的 TypeError、RangeError 等 JS 错误。
- goutil：读取和校验 Goja 参数，并把 JavaScript 值转换为 Go 类型。

这些包是模块实现和宿主适配的基础，不会额外向 JavaScript 暴露对象。

### TypeScript 类型定义

仓库提供拆分的类型包，当前包括：

- @dop251/types-goja_nodejs-global
- @dop251/types-goja_nodejs-url
- @dop251/types-goja_nodejs-buffer

在 TypeScript 项目中安装所需包，并把 node_modules/@dop251 纳入 typeRoots。类型
包按模块拆分，和 Go 侧按需启用的模型一致；它们只描述已实现的 API 子集，不代表完整
Node 类型。

## 运行时和并发注意事项

### Goja runtime 所有权

Goja 不是 goroutine-safe。以下操作都必须在拥有 runtime 的 loop 线程执行：

- 调用 rt.RunString、rt.RunProgram 或 Goja 函数；
- 创建、读取、修改或传递 Goja Value；
- 安装模块和设置全局变量；
- 处理异步模块回调。

网络、FS 和 WebSocket 的后台工作可以在 Go 协程中进行，但完成回调会回到同一个
runtime-bound event loop。不要把一个 loop 传给另一个 runtime。

### 配置必须一致

同一个 runtime 的 scheduler、FS backend、cwd、stream chunk size 和 capability
配置只能等价地重复安装。发现冲突时模块会返回错误，不采用 first-call-wins；这样
可以避免 require("fs")、全局 fs 和 process 看到不同的文件系统状态。

### Node 兼容边界

- 本项目没有 Node 的完整内置模块集合，也没有 node:test、child_process、worker_threads、
  cluster 等运行时服务。
- process.env 是初始化时的环境变量快照；process.chdir 是 runtime-local 操作。
- Fetch 的 body 只能按 Web API 语义消费一次；Response.body、FS 流和 Web Streams
  使用 Uint8Array，需要 Buffer 时必须显式转换。
- Node classic streams 是 streamx facade，任何依赖 Node readable-stream 私有字段、
  特定错误对象或精确 chunk 行为的第三方包都需要单独验证。
- FS 的 symlink、hard link、lstat 等能力来自宿主 capability；不能假定任意 Afero
  backend 都具备这些操作。
- WebSocket 默认验证 TLS；不要为了绕过证书问题在生产环境设置
  InsecureSkipVerify。
- timers/promises 依赖 event loop；在顶层脚本把导出的 setTimeout 用 const
  解构时，可能和全局 timer 产生 Goja 的暂时性死区冲突，放在 CommonJS 模块内部或
  使用别名可规避。

### 生命周期

宿主应在退出前停止 event loop，并关闭自己持有的 HTTP/WebSocket 资源。使用独立的
websocket.WebSocketManager 时调用 CloseAll()；不要在 loop 已停止后继续向其
RunOnLoop 提交 Goja 回调。

## 示例与验证

仓库中的可运行示例：

~~~text
go run ./examples/composed_runtime
go run ./examples/fetch_abort
go run ./examples/fs_runtime
go run ./examples/streams
go run ./examples/websocket_tls
~~~

常用验证命令：

~~~bash
go test -count=1 ./...
go test -count=1 -race ./...
go vet ./...
go build ./...
git diff --check
~~~

更细的模块行为、边界和测试用例请直接查看对应目录的 README 和 *_test.go 文件。

## 许可证

本项目使用 LICENSE 中声明的许可证。嵌入的 Web Streams、Node streams 和
events 实现还带有各自上游项目的许可证说明，详见对应目录中的 LICENSE/README。
