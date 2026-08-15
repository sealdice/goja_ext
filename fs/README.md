# fs 模块

Afero-backed 文件系统模块，同时提供 **Deno 风格**与 **Node.js 风格**两套 API。

## 模块名

- `fs.Enable*` 安装纯 Deno 风格 `globalThis.fs`，不加载 Node streams 或
  Node-only 方法。
- `require("fs")` / `require("node:fs")` —— 向后兼容的 Deno 基础方法 +
  Node wrapper。
- `require("fs/promises")` / `require("node:fs/promises")` —— Promise API，
  包含 Node 的 `appendFile/access/rm/lstat/realpath/readlink/symlink/link`。

## Deno 风格 API

同步/异步成对：`readFileSync`/`readFile`、`writeTextFileSync`/`writeTextFile`、`mkdirSync`/`mkdir`、`removeSync`/`remove`、`renameSync`/`rename`、`statSync`/`stat`、`openSync`/`open`（返回 `FsFile` 句柄，含 `readable`/`writable` WHATWG 流）、`readDirSync`/`readDir`、`makeTempFile*`、`makeTempDir*`、`copyFile*`、`chmod*`、`chown*`、`utime*`、`truncate*`、`cwd`、`chdir`。

## Node wrapper（Node 风格）

- **Callback API**：既有 Promise 方法在最后一个参数是函数时按 Node 回调形式调用，例如 `fs.readFile(path, "utf8", cb)`、`fs.stat(path, cb)`。
- **新增方法**：`appendFile`/`appendFileSync`、`exists`/`existsSync`、`access`/`accessSync`、`unlink`/`unlinkSync`、`rmdir`/`rmdirSync`、`rm`/`rmSync`、`realpath`/`realpathSync`、`lstat`/`lstatSync`、`readlink*`、`symlink*`、`link*`、`constants`、`createReadStream`、`createWriteStream`。
- **编码**：`readFileSync(path, "utf8")` 返回字符串；`readFile`/`writeFile` 支持 encoding 选项。
- **Stats**：`statSync`/`stat`/`lstat*` 返回 Node 形状 Stats（`size`、`mode`(POSIX st_mode)、`atime/mtime/ctime/birthtime`(Date)、`uid/gid/dev/ino/nlink/rdev/blksize/blocks`、`isFile/isDirectory/isSymbolicLink/isBlockDevice/isCharDevice/isFIFO/isSocket`）。
- **open flags**：`openSync(path, "w")` / `"r+"` / `"a"` 等 Node 标志字符串。
- **流**：`createReadStream`/`createWriteStream` 基于 streamx facade
  (`streams/node`)。FS 不再隐式导入该包；宿主必须显式注册它。

## 配置（Go）

```go
import (
    "github.com/dop251/goja_nodejs/fs/extra"
    _ "github.com/dop251/goja_nodejs/streams/node" // 仅在需要 Node 文件流时
)

backend := afero.NewOsFs()
registry.RegisterNativeModule("fs", fs.RequireWithOptions(
    fs.WithFS(backend),
    fs.WithCwd("/workspace"),
    fs.WithStreams(true),
    fs.WithExtraCapabilities(extra.FromAfero(backend)...),
))
```

- `WithFS` / `WithCwd` / `WithStreams` / `WithStreamChunkSize` / `WithExtraCapabilities`。
- `RegisterWithLoop` / `RequireWithLoop` 使异步操作真正 off-loop 执行。
- 同一 runtime 的 backend/cwd/chunk size/capability 配置必须一致；冲突
  配置返回错误，不采用 first-call-wins。
- `WithStreams(false)` 不安装 `FsFile.readable/writable`、
  `createReadStream/createWriteStream` 或其别名。

## 已知边界

- 字节结果为 `Uint8Array`，不是 Node Buffer。
- `lstat/realpath/readlink/symlink/link` 只使用显式 capability；不支持时返回
  `ENOSYS`。`realpath` 会逐段解析 symlink 并检测 `ELOOP`，不会用
  `Stat` 或路径清理冒充。
- `fs/extra.FromAfero` 适配 Afero 的 `Lstater`、`LinkReader` 与 `Linker`。
  Afero 返回“已回退到 Stat”时仍视为不支持。hard link 在 backend 实现
  `Linker` 时可用；watch、文件锁和 terminal 不属于当前声明的 FS 子集。
- `createReadStream` 的 `start`/`end`/`encoding`/`highWaterMark` 已支持；其余选项（如 `autoClose` 之外的高级项）未实现。
