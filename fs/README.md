# fs 模块

基于 Afero 的 Deno 风格文件系统模块。JavaScript 入口为 `require("fs")`、
`require("fs/promises")` 和 `globalThis.fs`；不提供 `node:fs` 或 Node callback API。

## API

- 同步/异步文件操作：`readFile*`、`readTextFile*`、`writeFile*`、
  `writeTextFile*`、`open*`、`create*`、`stat*`、`readDir*`、`mkdir*`、
  `remove*`、`rename*`、`copyFile*`、`truncate*`、`chmod*`、`chown*`、
  `utime*`、`makeTempFile*` 和 `makeTempDir*`。
- `FsFile` 提供读写、seek、truncate、stat、sync、utime、close，以及可选的
  WHATWG `readable`/`writable`。
- `FileInfo` 和 `DirEntry` 使用 Deno 属性形态，例如 `info.isFile`，不是
  Node `Stats`/`Dirent` 方法。
- 字节 API 接受并返回 `ArrayBuffer`/`ArrayBufferView` 与 `Uint8Array`；文本应
  使用 `readTextFile*`/`writeTextFile*`。
- `readFile`、`readTextFile`、`writeFile`、`writeTextFile` 支持
  `AbortSignal`。`writeFile` 还可消费 `ReadableStream<Uint8Array>`，
  `writeTextFile` 可消费 `ReadableStream<string>`。

## 配置

```go
backend := afero.NewOsFs()
fs.RegisterWithLoop(registry, loop,
    fs.WithFS(backend),
    fs.WithCwd("/workspace"),
    fs.WithStreams(true),
    fs.WithStreamChunkSize(64*1024),
    fs.WithExtraCapabilities(extra.FromAfero(backend)...),
)
```

- `RegisterWithLoop`/`RequireWithLoop` 把异步 Afero 操作放到 worker goroutine，
  Promise 只在 runtime event loop 上结算。未提供 scheduler 时保留同步 fallback。
- `WithStreams(false)` 不安装 `FsFile.readable/writable`，也不接受流式文件输入。
- readable 按配置大小真实分块；EOF、cancel 和错误会关闭文件。writable 的
  close/abort/错误同样关闭文件，close 不隐式执行 `Sync()`。
- `WithExtraCapabilities` 可注入 `lstat`、`realPath`、`readLink`、symlink 和
  hard link；未提供时相应导出不存在或返回 `ENOSYS`。

## 取消边界

普通 Afero 文件没有 context-aware I/O。Abort 会立即停止 JavaScript 侧消费并
拒绝 Promise，但已经进入底层的单次阻塞调用可能继续执行；调用返回后句柄会被关闭。
这适合本地文件和内存文件后端。需要可物理中断远程 I/O 的后端应在自身架构中提供
更强的取消机制。
