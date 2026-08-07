# Afero-backed Goja FS 模块功能设计

> **范围更新（2026-08-07）**：Deno FS API 是本项目的原生文件系统层；Node `fs` 兼容性作为后续 facade 建立在同一个 Go FS Core 上。符号链接、hard link、watch、锁和 terminal 等能力不再从核心设计中强行删除，而是移动到独立的 `extra` 扩展，通过 Afero 可选接口或宿主 capability 探测后按需安装。

## 背景

当前仓库是 `github.com/sealdice/goja_ext`，已有 `require`、`buffer`、`eventloop`、`fetch`、`websocket` 等 Goja 扩展模块。根目录的 `30_fs.js` 来自 Deno 内部实现，它依赖 Deno runtime 的 `core.ops`、资源句柄、Web Streams、AbortSignal 和平台级文件系统能力，不能直接作为 Goja 模块运行。

本设计先覆盖 `30_fs.js` 的 Deno 风格 API，再在相同 Go FS Core 上提供 Node `fs` facade。目标是让调用方可以注入 `OsFs`、`MemMapFs`、`BasePathFs`、`CopyOnWriteFs`、只读 FS 等 Afero 后端，并让 JS 侧获得稳定的常用文件操作能力。Node facade 不直接复用 Deno runtime 的 `core.ops`，而只复用本项目的 Go FS Core、句柄和 stream bridge。

本设计假设仓库使用根目录 `功能规格说明书.md` 定义的 polyfill-backed WHATWG Streams 模块。当前 `streams` 包已经提供完整的 Web Streams 构造器集合，包括 byte streams/BYOB、TransformStream、`pipeTo()`、`pipeThrough()`、`tee()` 和 `ReadableStream.from()`。Streams 扩展还将提供 `TextEncoderStream`、`TextDecoderStream`，以及可独立启用的 Node classic streams facade。FS 模块复用 canonical Web Streams；Node facade 不把 WHATWG Streams 冒充为 Node.js stream。

## 目标与非目标

目标：

- 新增 `fs` 包，提供 Afero-backed 文件系统能力。
- 默认后端使用 `afero.NewOsFs()`，同时允许调用方注入自定义 `afero.Fs`。
- 以 Deno `30_fs.js` 的导出为核心，支持同步 API 和 Promise API；后续 Node facade 再提供 callback API。
- 支持路径级读写、目录、stat、权限、时间、临时文件、打开文件句柄和 `FsFile` 等能力。
- 集成 WHATWG Streams：`FsFile.readable`、`FsFile.writable`，以及 `writeFile`/`writeTextFile` 对 `ReadableStream` 数据源的支持。
- 使用 `eventloop.EventLoop` 安全完成异步 Promise settle，不在 Go 协程中直接操作 Goja runtime。
- 用 Afero 的核心接口作为主能力边界；Afero 可选接口和宿主能力通过独立扩展探测并按需安装。
- 让 Node `fs/promises`、Node `fs` 和 classic stream 适配层可以复用同一 FS Core，而不复制文件系统状态。

非目标：

- Deno FS 核心不依赖符号链接、hard link、`watch`、文件锁和 terminal raw mode；这些能力在 `fs/extra` 中独立实现。
- 不保证 Node.js `fs` 的完整错误码、Windows/junction 行为和平台专用 stat 字段。
- 不在 Deno FS 第一阶段实现 Node `stream.Readable/Writable` 语义；Node classic streams 由 `streams/node` 独立模块提供。
- 不要求 FS 自己暴露 byte streams、BYOB、TransformStream 或 Node stream API；这些能力属于独立的 `streams` 模块。FS 的文件句柄适配器使用 canonical Web Streams，调用方仍可在 FS 之外使用完整 WHATWG Streams 能力组织数据流。

## 模块表面

Go 侧提供：

```go
package fs

func Enable(rt *goja.Runtime, opts ...Option) error
func EnableWithLoop(rt *goja.Runtime, loop *eventloop.EventLoop, opts ...Option) error
func RequireWithOptions(opts ...Option) require.ModuleLoader
func RequirePromisesWithOptions(opts ...Option) require.ModuleLoader
func RequireWithLoop(loop *eventloop.EventLoop, opts ...Option) require.ModuleLoader
func RequirePromisesWithLoop(loop *eventloop.EventLoop, opts ...Option) require.ModuleLoader
func RegisterWithOptions(registry *require.Registry, opts ...Option)
func RegisterWithLoop(registry *require.Registry, loop *eventloop.EventLoop, opts ...Option)

func WithFS(afs afero.Fs) Option
func WithCwd(cwd string) Option
func WithStreams(enabled bool) Option
func WithStreamChunkSize(size int) Option
func WithExtraCapabilities(capabilities ...Capability) Option
```

默认注册：

- `require("fs")`
- `require("node:fs")`
- `require("fs/promises")`
- `require("node:fs/promises")`

自定义后端时，推荐调用 `RegisterWithOptions()` 或 `RegisterWithLoop()`，它们会同时注册 `fs`、`node:fs`、`fs/promises`、`node:fs/promises`。需要手动控制模块名时，也可以通过 `Registry.RegisterNativeModule()` 注册 `RequireWithOptions()` / `RequirePromisesWithOptions()` 或 loop-aware loader。直接 `init()` 注册的默认模块使用 `afero.NewOsFs()`。

`RequireWithOptions()` 没有办法从 goja_nodejs 的 native module loader 签名中自动取得 event loop，因此 Promise API 在该路径下只能同步完成 Afero 操作后返回已结算 Promise。需要真正 off-loop 执行时必须使用 `RequireWithLoop()`、`RequirePromisesWithLoop()`、`RegisterWithLoop()` 或 `EnableWithLoop()`。

`fs/extra` 不默认注册；宿主显式提供扩展 capability 后，才注册对应的附加入口或在主 Deno facade 中安装附加方法。这样可以在不改动核心 API 的情况下移除整组扩展。

JS 侧分四类能力：

- 同步 API：不需要 event loop，直接执行 Afero 操作。
- Promise API：使用 loop-aware 装配时，Afero 操作在 Go 协程执行，结果通过 `RunOnLoop()` 回到 JS；无 loop 装配退化为同步执行并返回 Promise。
- Callback API：不属于 Deno FS 核心；后续 Node facade 可复用同一异步执行路径并按 err-first callback 形状回调。
- Stream API：需要 `eventloop.EventLoop` 和 WHATWG Streams 模块；没有 loop 时不安装或访问时报明确配置错误。

FS 模块与 Streams 模块之间复用以下 Go 集成契约：

- 识别某个 `goja.Value` 是否为本 runtime 的 `ReadableStream`。
- 基于 Go `pull` / `cancel` 回调创建 `ReadableStream` 对象。
- 基于 Go `write` / `close` / `abort` 回调创建 `WritableStream` 对象。
- 用 `ConsumeReadableStream` 的 default reader 顺序消费 `ReadableStream`，并把每个 chunk 交给 FS 的写入逻辑；consumer 返回 Promise 时自然形成背压。

这些 helper 必须复用 Streams 模块的 canonical constructors、prototype identity 和品牌检查，不能在 FS 包里重新造另一套流对象。FS 只需识别本 runtime 的 `ReadableStream`/`WritableStream`，不应通过 `instanceof` 自己猜测 polyfill 内部字段。

## 分层与文件布局

实现分成三个可以独立编译和测试的层：

1. `fs/core`：持有 Afero、logical cwd、句柄表、路径转换、打开选项、错误映射和同步/异步 Go 操作；不创建 JS API。
2. `fs`：Deno `30_fs.js` facade，导出 `FsFile`、路径 API、default Web Streams 读写，以及 Promise/sync API。
3. `fs/extra`：能力探测式扩展，包含链接、hard link、watch、锁和 terminal 等不稳定能力；扩展只能依赖 `fs/core` 的 capability interface。

Node `fs` facade 以后放在 `fs/node`，直接依赖 `fs/core`，不通过 Deno JS facade 二次转换。当前 Streams 的 Node classic facade 同理放在 `streams/node`，不影响 `stream/web` 的 canonical constructor identity。

推荐文件布局：

```text
fs/
  core.go             # FS Core、Option、logical cwd、句柄生命周期
  paths.go            # 路径规范化和 URL/path 输入转换
  errors.go           # Go error -> JS error/code
  api.go              # Deno facade 与主模块注册
  handles.go          # FsFile 和句柄方法
  streams.go          # FsFile readable/writable 与 stream-backed 写入
  extra/
    capabilities.go   # 可选接口探测
    links.go          # lstat/readlink/symlink/link/realpath
    locking.go        # lock/tryLock/unlock
    terminal.go       # isTerminal/raw mode
    watch.go          # watch/watchFile，后端不支持时返回 ENOSYS
  node/
    api.go            # node:fs/fs facade，后续阶段
```

## Afero 能力映射

直接由 `afero.Fs` 或 `afero.File` 支撑：

| 能力 | Afero 依据 | JS API 示例 |
| --- | --- | --- |
| 创建文件 | `Fs.Create` / `Fs.OpenFile` | `writeFile`、`open` |
| 打开文件 | `Fs.Open` / `Fs.OpenFile` | `open`、`openSync` |
| 读文件 | `afero.ReadFile` / `File.Read` | `readFile`、`FileHandle.read` |
| 写文件 | `afero.WriteFile` / `File.Write` | `writeFile`、`appendFile`、`FileHandle.write` |
| 目录创建 | `Fs.Mkdir` / `Fs.MkdirAll` | `mkdir` |
| 目录读取 | `afero.ReadDir` / `File.Readdir` | `readdir`、`opendir` 的基础能力 |
| 删除 | `Fs.Remove` / `Fs.RemoveAll` | `rm`、`unlink`、`rmdir` |
| 重命名 | `Fs.Rename` | `rename` |
| stat | `Fs.Stat` / `File.Stat` | `stat`、`FileHandle.stat` |
| 权限 | `Fs.Chmod` / `Fs.Chown` | `chmod`、`chown` |
| 时间 | `Fs.Chtimes` | `utimes` |
| 截断 | `File.Truncate` | `truncate`、`FileHandle.truncate` |
| 同步落盘 | `File.Sync` | `FileHandle.sync` |
| seek | `File.Seek` | 文件句柄内部读写位置 |
| Go 层流式 I/O | `File.Read` / `File.Write` / `io.Copy` | `FsFile.readable`、`FsFile.writable`、stream-backed `writeFile` |

通过 Afero 组合实现：

- `copyFile`：打开源文件和目标文件，用 `io.Copy` 流式复制。
- `truncate(path)`：`OpenFile` 后调用 `File.Truncate`。
- `appendFile`：`OpenFile` 使用 append/create/write flags。
- `access`：默认用 `Stat` 判断存在性；权限检查只做 best-effort，不宣称等价 Node。
- `mkdtemp`：用 Afero 临时文件辅助函数；自定义 FS 后端不支持时返回明确错误。
- `FsFile.readable`：基于 WHATWG `ReadableStream` 的 `pull` 回调分块读取 `afero.File`。
- `FsFile.writable`：基于 WHATWG `WritableStream` 的 `write` 回调分块写入 `afero.File`。
- `writeFile(path, ReadableStream)`：使用 reader 循环消费 stream chunk 并写入目标文件。

进入 `fs/extra` 的能力：

- `lstat`：优先探测 `afero.Lstater`；不支持时扩展不安装。
- `readlink` / `symlink`：分别探测 `afero.LinkReader` / `afero.Linker`；不支持时返回 `ENOSYS` 或不安装，由扩展选项决定。
- `realpath`：只有后端提供显式解析能力时安装；不能用普通 `Stat` 冒充。
- `link`：Afero 核心没有 hard link interface，作为单独的宿主 capability。
- `watch` / `watchFile`：只有后端或宿主提供 watcher 时安装，否则保持 `ENOSYS`。
- `umask`：作为进程/FS 实例级 capability，不写入核心 Afero 适配。
- 文件锁、raw terminal、`isTerminal`：作为 OS/句柄 capability，放在独立扩展文件。

扩展实现必须允许调用方只注册其中一部分；不需要的文件可以在构建标签或上层注册代码中直接删除，不影响 `fs/core` 和 Deno 主 API。

## 路径与 CWD

Afero 没有进程级当前目录概念。模块为每个 runtime 维护一个 logical cwd：

- 默认 `OsFs` 使用宿主机 `os.Getwd()`。
- 自定义 Afero 后端默认 cwd 为 `/`，调用方可用 `WithCwd()` 覆盖。
- 相对路径会先和 logical cwd 拼接，再传给后端。
- 不做 `realpath` 解析，不跟随或解析符号链接。
- 沙箱边界不由模块自己发明；需要限制根目录时，调用方应传入 `afero.NewBasePathFs()` 或等价包装。

路径规则以可预测为优先。对内存 FS 等非 OS 后端，优先使用 slash-style logical path；对 `OsFs`，只保证当前运行平台的常规路径可用。

## Stat 与 Dirent

便携字段来自 `os.FileInfo`：

- `name`
- `size`
- `mode`
- `mtime`
- `isFile()`
- `isDirectory()`

不保证的平台字段：

- `dev`
- `ino`
- `uid`
- `gid`
- `rdev`
- `blksize`
- `blocks`
- `birthtime`
- `ctime`
- `atime`

这些字段默认置为 `null`、`0`，或不出现在 Afero-first 类型定义中。`isSymbolicLink()`、`isBlockDevice()`、`isCharacterDevice()`、`isFIFO()`、`isSocket()` 默认返回 `false`，不作为真实平台判断。

## 文件句柄

模块内部维护 `rid -> afero.File` 的句柄表，用于隔离 Go 文件对象和 JS 对象生命周期：

- `open/openSync` 返回 `FileHandle` 或 `FsFile` 包装对象。
- 每个句柄记录是否已关闭，重复关闭不 panic。
- 句柄支持 `read`、`write`、`seek`、`truncate`、`stat`、`sync`、`close`。
- 句柄支持惰性 `readable` / `writable` getter，返回 WHATWG Streams 对象。
- 句柄方法的异步版本仍通过 event loop settle Promise。
- 不在 Go 协程中持有或修改 Goja `Value`；进入协程前复制 path、flags、buffer 等纯 Go 数据。

并发策略：

- 每个文件句柄内部加锁，避免同一 `afero.File` 被多个异步操作同时读写导致位置竞争。
- `readable` / `writable` 与手动 `read` / `write` / `seek` 共用同一个文件 offset，并通过同一把句柄锁串行化。
- path-level 操作不做全局串行化，交给后端处理一致性。

## 异步与取消

Afero 是同步接口，异步能力由模块包装：

1. JS 调用时在 Goja 线程内解析参数并创建 Promise。
2. 将纯 Go 请求数据交给 Go 协程执行 Afero 操作。
3. 操作完成后通过 `loop.RunOnLoop()` resolve 或 reject。
4. 后续 Node callback API 使用同一执行路径，只是在 settle 时调用 err-first callback。

取消策略：

- `AbortSignal` 只做 best-effort。
- 操作开始前如果 signal 已取消，立即 reject。
- 操作执行中取消时，可以让 Promise 忽略后续成功结果并 reject abort 错误。
- Afero 无 `context.Context` 参数，不能保证真正中断底层文件读写。
- Stream 的 `cancel()` / `abort()` 只停止后续 chunk 调度并关闭当前 stream 状态；正在执行的 Afero 调用无法被强制抢占，晚到结果必须被丢弃。

## 流读写

Afero 提供 Go 层流式 I/O，WHATWG Streams 模块提供 JS 层流对象。FS 模块负责把两者连接起来：

- `afero.File` 可作为 `io.Reader`、`io.Writer`、`io.Seeker` 使用。
- `copyFile` 可以用 `io.Copy`，不会把整个文件读入内存。
- `FileHandle.read/write` 可以让 JS 传 Buffer/TypedArray 分块读写。
- `FsFile.readable` 返回 WHATWG `ReadableStream`，每次 `pull` 最多读取一个 chunk。
- `FsFile.writable` 返回 WHATWG `WritableStream`，每次 `write` 写入一个 chunk。

Stream 集成规则：

- 默认 chunk size 为 `64 * 1024`，可通过 `WithStreamChunkSize()` 调整。
- stream chunk 支持 Buffer、TypedArray、ArrayBuffer、`[]byte`、string；string 按 UTF-8 编码。
- `ReadableStream` 遇到 EOF 时调用 controller close；读错误时 controller error。
- `WritableStream` 写入失败时 controller error；close 时做 best-effort `Sync()`，不保证所有 Afero 后端都有真实落盘语义。
- `writeFile` / `writeTextFile` 如果收到 `ReadableStream`，由 FS 模块调用 `ConsumeReadableStream` 显式顺序消费；不依赖 `pipeTo()`，但调用方可以在进入 FS 前使用 `TransformStream`、`tee()` 或其他 WHATWG 组合能力。
- `writeFile(path, stream)` 打开的目标文件由该操作拥有，成功、失败、取消后都关闭文件。
- `FsFile.readable` / `FsFile.writable` 是现有句柄的视图，不自动关闭 `FsFile`；调用方仍通过 `close()` 释放句柄。

第一阶段仍不实现：

- `fs.createReadStream`、`fs.createWriteStream` 的 Node stream 语义；由后续 `fs/node` 复用 `streams/node`。
- FS 自己的 byte-stream/BYOB 文件句柄 API；Deno FS 使用 canonical Web Streams，Streams 模块本身提供 byte/BYOB。

## 错误模型

错误对象尽量接近 Node：

- `.message`
- `.code`
- `.errno`
- `.syscall`
- `.path`
- `.dest`，用于 rename/copy 等双路径操作

错误映射：

- `os.ErrNotExist` -> `ENOENT`
- `os.ErrExist` -> `EEXIST`
- `os.ErrPermission` -> `EACCES`
- invalid argument -> `EINVAL`
- unsupported capability -> `ENOSYS`

自定义 Afero 后端返回的非标准错误只能 best-effort 映射。模块不承诺完全复刻 Node 的错误文本。

## 功能覆盖估算

以 `30_fs.js` 的能力清单作参考，核心阶段覆盖 Deno API 的常用和句柄能力；扩展阶段按 capability 增加平台能力：

- 路径级常用操作：大部分可覆盖。
- 文件句柄基础 I/O：可覆盖。
- 目录读取和 stat 基础字段：可覆盖。
- WHATWG `ReadableStream` / `WritableStream` 文件读写：可覆盖。
- 真实平台 stat 扩展字段：Afero 能提供的字段覆盖，其余保持空值。
- 符号链接、hard link、watch、锁、终端、raw mode：核心不依赖，扩展按能力覆盖。
- Node `fs` 和 classic stream：后续 facade，不阻塞 Deno FS。

实际工作量主要在 JS API 适配、错误模型、异步回调、句柄表、Streams 集成和测试，而不是 Afero 后端本身。

## 测试策略

单元测试优先使用 Afero 后端：

- `MemMapFs`：覆盖 read/write/open/stat/mkdir/remove/rename/copy/temp 等核心能力。
- `ReadOnlyFs`：覆盖写入失败和错误映射。
- `BasePathFs`：覆盖 logical cwd 和路径隔离。
- 自定义 stub FS：覆盖 unsupported capability 和异常路径。

异步测试：

- 用 `eventloop.EventLoop` 测试 Promise resolve/reject。
- Node facade 阶段再测试 callback API 的 err-first 形状。
- 测试异步操作不在 Go 协程中直接访问 Goja runtime。

Stream 集成测试：

- `FsFile.readable` 分块读取、EOF close、读错误 error。
- `FsFile.writable` 写入 Buffer/TypedArray/ArrayBuffer/string chunk、close、abort。
- `writeFile(path, ReadableStream)` 和 `writeTextFile(path, ReadableStream)`。
- stream 与手动 `read` / `write` / `seek` 共享 offset 且通过句柄锁串行。
- 没有 event loop 或 Streams 模块未启用时，stream API 返回明确配置错误；Streams 模块本身的 byte/BYOB/Transform/pipe/tee 能力不属于这个错误条件。

少量真实文件系统集成测试：

- `OsFs` 下验证 chmod、chown、truncate、temp file、sync。
- 对平台差异敏感的测试只做 smoke，不作为跨平台强断言。

## 验收标准

- 调用方可注入任意 `afero.Fs`，核心读写目录操作能在 `MemMapFs` 上通过。
- 同步 API 不依赖 event loop。
- loop-aware 装配下，Promise API 必须通过 event loop 回到 JS。
- `FsFile.readable` / `FsFile.writable` 和 stream-backed `writeFile` 在 WHATWG Streams 模块可用时通过。
- 符号链接、watch、锁、终端和 Node stream 不作为 Deno FS 核心验收范围；它们各自有独立扩展/适配测试。
- 错误对象包含可被 JS 侧判断的 `.code`。
- 文件句柄关闭后继续使用会返回稳定错误，不 panic。
