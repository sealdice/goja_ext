declare module "fs" {
  export type PathLike = string;
  export type FileData = ArrayBuffer | ArrayBufferView;

  export interface ReadFileOptions {
    signal?: AbortSignal;
  }

  export interface WriteFileOptions {
    append?: boolean;
    create?: boolean;
    createNew?: boolean;
    mode?: number;
    signal?: AbortSignal;
  }

  export interface OpenOptions {
    read?: boolean;
    write?: boolean;
    append?: boolean;
    truncate?: boolean;
    create?: boolean;
    createNew?: boolean;
    mode?: number;
  }

  export interface MkdirOptions {
    recursive?: boolean;
    mode?: number;
  }

  export interface RemoveOptions {
    recursive?: boolean;
  }

  export interface MakeTempOptions {
    dir?: string;
    prefix?: string;
    suffix?: string;
  }

  export interface FileInfo {
    readonly isFile: boolean;
    readonly isDirectory: boolean;
    readonly isSymlink: boolean;
    readonly size: number;
    readonly mtime: Date | null;
    readonly atime: Date | null;
    readonly birthtime: Date | null;
    readonly ctime: Date | null;
    readonly dev: number | null;
    readonly ino: number | null;
    readonly mode: number | null;
    readonly nlink: number | null;
    readonly uid: number | null;
    readonly gid: number | null;
    readonly rdev: number | null;
    readonly blksize: number | null;
    readonly blocks: number | null;
    readonly isBlockDevice: boolean | null;
    readonly isCharDevice: boolean | null;
    readonly isFifo: boolean | null;
    readonly isSocket: boolean | null;
  }

  export interface DirEntry {
    readonly name: string;
    readonly isFile: boolean;
    readonly isDirectory: boolean;
    readonly isSymlink: boolean;
  }

  export class FsFile {
    readonly readable?: ReadableStream<Uint8Array>;
    readonly writable?: WritableStream<Uint8Array>;
    writeSync(data: FileData): number;
    write(data: FileData): Promise<number>;
    readSync(target: ArrayBufferView): number | null;
    read(target: ArrayBufferView): Promise<number | null>;
    seekSync(offset: number, whence: number): number;
    seek(offset: number, whence: number): Promise<number>;
    truncateSync(length?: number): void;
    truncate(length?: number): Promise<void>;
    statSync(): FileInfo;
    stat(): Promise<FileInfo>;
    syncSync(): void;
    sync(): Promise<void>;
    syncDataSync(): void;
    syncData(): Promise<void>;
    utimeSync(atime: number | Date, mtime: number | Date): void;
    utime(atime: number | Date, mtime: number | Date): Promise<void>;
    close(): void;
    isTerminal(): false;
  }

  export function cwd(): string;
  export function chdir(path: PathLike): void;
  export function readFileSync(path: PathLike): Uint8Array;
  export function readFile(path: PathLike, options?: ReadFileOptions): Promise<Uint8Array>;
  export function readTextFileSync(path: PathLike): string;
  export function readTextFile(path: PathLike, options?: ReadFileOptions): Promise<string>;
  export function writeFileSync(path: PathLike, data: FileData, options?: WriteFileOptions): void;
  export function writeFile(path: PathLike, data: FileData | ReadableStream<Uint8Array>, options?: WriteFileOptions): Promise<void>;
  export function writeTextFileSync(path: PathLike, data: string, options?: WriteFileOptions): void;
  export function writeTextFile(path: PathLike, data: string | ReadableStream<string>, options?: WriteFileOptions): Promise<void>;
  export function openSync(path: PathLike, options?: OpenOptions): FsFile;
  export function open(path: PathLike, options?: OpenOptions): Promise<FsFile>;
  export function createSync(path: PathLike): FsFile;
  export function create(path: PathLike): Promise<FsFile>;
  export function mkdirSync(path: PathLike, options?: MkdirOptions): void;
  export function mkdir(path: PathLike, options?: MkdirOptions): Promise<void>;
  export function chmodSync(path: PathLike, mode: number): void;
  export function chmod(path: PathLike, mode: number): Promise<void>;
  export function chownSync(path: PathLike, uid: number, gid: number): void;
  export function chown(path: PathLike, uid: number, gid: number): Promise<void>;
  export function readDirSync(path: PathLike): DirEntry[];
  export function readDir(path: PathLike): AsyncIterable<DirEntry>;
  export function statSync(path: PathLike): FileInfo;
  export function stat(path: PathLike): Promise<FileInfo>;
  export function lstatSync(path: PathLike): FileInfo;
  export function lstat(path: PathLike): Promise<FileInfo>;
  export function renameSync(oldPath: PathLike, newPath: PathLike): void;
  export function rename(oldPath: PathLike, newPath: PathLike): Promise<void>;
  export function removeSync(path: PathLike, options?: RemoveOptions): void;
  export function remove(path: PathLike, options?: RemoveOptions): Promise<void>;
  export function copyFileSync(source: PathLike, destination: PathLike): void;
  export function copyFile(source: PathLike, destination: PathLike): Promise<void>;
  export function truncateSync(path: PathLike, length?: number): void;
  export function truncate(path: PathLike, length?: number): Promise<void>;
  export function realPathSync(path: PathLike): string;
  export function realPath(path: PathLike): Promise<string>;
  export function readLinkSync(path: PathLike): string;
  export function readLink(path: PathLike): Promise<string>;
  export function symlinkSync(target: PathLike, path: PathLike): void;
  export function symlink(target: PathLike, path: PathLike): Promise<void>;
  export function linkSync(existingPath: PathLike, newPath: PathLike): void;
  export function link(existingPath: PathLike, newPath: PathLike): Promise<void>;
  export function utimeSync(path: PathLike, atime: number | Date, mtime: number | Date): void;
  export function utime(path: PathLike, atime: number | Date, mtime: number | Date): Promise<void>;
  export function makeTempDirSync(options?: MakeTempOptions): string;
  export function makeTempDir(options?: MakeTempOptions): Promise<string>;
  export function makeTempFileSync(options?: MakeTempOptions): string;
  export function makeTempFile(options?: MakeTempOptions): Promise<string>;
}

declare module "fs/promises" {
  export {
    chmod, chown, copyFile, create, link, lstat, makeTempDir, makeTempFile,
    mkdir, open, readDir, readFile, readLink, readTextFile, realPath, remove,
    rename, stat, symlink, truncate, utime, writeFile, writeTextFile
  } from "fs";
}
