# fs 模块

Afero-backed 文件系统模块，同时提供 **Deno 风格**与 **Node.js 风格**两套 API。

## 模块名

- `require("fs")` / `require("node:fs")` —— Deno 风格 + Node wrapper
- `require("fs/promises")` / `require("node:fs/promises")` —— Promise API

## Deno 风格 API

同步/异步成对：`readFileSync`/`readFile`、`writeTextFileSync`/`writeTextFile`、`mkdirSync`/`mkdir`、`removeSync`/`remove`、`renameSync`/`rename`、`statSync`/`stat`、`openSync`/`open`（返回 `FsFile` 句柄，含 `readable`/`writable` WHATWG 流）、`readDirSync`/`readDir`、`makeTempFile*`、`makeTempDir*`、`copyFile*`、`chmod*`、`chown*`、`utime*`、`truncate*`、`cwd`、`chdir`。

## Node wrapper（Node 风格）

- **Callback API**：既有 Promise 方法在最后一个参数是函数时按 Node 回调形式调用，例如 `fs.readFile(path, "utf8", cb)`、`fs.stat(path, cb)`。
- **新增方法**：`appendFile`/`appendFileSync`、`exists`/`existsSync`、`access`/`accessSync`、`unlink`/`unlinkSync`、`rmdir`/`rmdirSync`、`rm`/`rmSync`、`realpath`/`realpathSync`、`lstat`/`lstatSync`、`constants`、`createReadStream`、`createWriteStream`。
- **编码**：`readFileSync(path, "utf8")` 返回字符串；`readFile`/`writeFile` 支持 encoding 选项。
- **Stats**：`statSync`/`stat`/`lstat*` 返回 Node 形状 Stats（`size`、`mode`(POSIX st_mode)、`atime/mtime/ctime/birthtime`(Date)、`uid/gid/dev/ino/nlink/rdev/blksize/blocks`、`isFile/isDirectory/isSymbolicLink/isBlockDevice/isCharDevice/isFIFO/isSocket`）。
- **open flags**：`openSync(path, "w")` / `"r+"` / `"a"` 等 Node 标志字符串。
- **流**：`createReadStream`/`createWriteStream` 基于 streamx facade（`streams/node`）。

## 配置（Go）

```go
registry.RegisterNativeModule("fs", fs.RequireWithOptions(
    fs.WithFS(afero.NewMemMapFs()),
    fs.WithCwd("/workspace"),
    fs.WithStreams(true),
))
```

- `WithFS` / `WithCwd` / `WithStreams` / `WithStreamChunkSize` / `WithExtraCapabilities`。
- `RegisterWithLoop` / `RequireWithLoop` 使异步操作真正 off-loop 执行。

## 已知边界

- `fs/promises` 为 Deno 风格（返回 Uint8Array，非 Node Buffer）；Node 风格以 `fs` 模块为主。
- 符号链接、hard link、watch、文件锁等能力属 `fs/extra` 扩展，需宿主显式提供。
- `createReadStream` 的 `start`/`end`/`encoding`/`highWaterMark` 已支持；其余选项（如 `autoClose` 之外的高级项）未实现。
