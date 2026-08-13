declare module "fs" {
  export type PathLike = string;
  export type FileData = string | ArrayBuffer | ArrayBufferView;
  export type Callback<T = void> = (error: Error | null, value: T) => void;

  export interface FileInfo {
    name: string;
    size: number;
    mode: number;
    mtime: Date;
    atime: Date;
    birthtime: Date;
    isFile(): boolean;
    isDirectory(): boolean;
    isSymbolicLink(): boolean;
  }

  export interface DirEntry {
    name: string;
    isFile(): boolean;
    isDirectory(): boolean;
    isSymlink(): boolean;
  }

  export class FsFile {
    readonly readable?: ReadableStream<Uint8Array>;
    readonly writable?: WritableStream<Uint8Array>;
    writeSync(data: FileData): number;
    write(data: FileData): Promise<number>;
    readSync(target: ArrayBufferView): number | null;
    read(target: ArrayBufferView): Promise<number | null>;
    seekSync(offset: number, whence: number | "start" | "current" | "end"): number;
    seek(offset: number, whence: number | "start" | "current" | "end"): Promise<number>;
    truncateSync(length?: number): void;
    truncate(length?: number): Promise<void>;
    statSync(): FileInfo;
    stat(): Promise<FileInfo>;
    syncSync(): void;
    sync(): Promise<void>;
    syncDataSync(): void;
    syncData(): Promise<void>;
    close(): void;
    isTerminal(): false;
  }

  export function cwd(): string;
  export function chdir(path: PathLike): void;
  export function readFileSync(path: PathLike, options?: unknown): Uint8Array | string;
  export function readFile(path: PathLike, callback: Callback<Uint8Array>): void;
  export function readFile(path: PathLike, options: unknown, callback: Callback<Uint8Array | string>): void;
  export function readFile(path: PathLike): Promise<Uint8Array>;
  export function readFile(path: PathLike, options: string | { encoding: string }): Promise<string>;
  export function readFile(path: PathLike, options?: unknown): Promise<Uint8Array | string>;
  export function readTextFileSync(path: PathLike): string;
  export function readTextFile(path: PathLike): Promise<string>;
  export function writeFileSync(path: PathLike, data: FileData, options?: unknown): void;
  export function writeFile(path: PathLike, data: FileData, options?: unknown): Promise<void>;
  export function writeFile(path: PathLike, data: FileData, callback: Callback): void;
  export function writeFile(path: PathLike, data: FileData, options: unknown, callback: Callback): void;
  export function writeTextFileSync(path: PathLike, data: string, options?: unknown): void;
  export function writeTextFile(path: PathLike, data: string, options?: unknown): Promise<void>;
  export function appendFileSync(path: PathLike, data: FileData, options?: unknown): void;
  export function appendFile(path: PathLike, data: FileData, options?: unknown): Promise<void>;
  export function openSync(path: PathLike, options?: unknown): FsFile;
  export function open(path: PathLike, options?: unknown): Promise<FsFile>;
  export function open(path: PathLike, options: unknown, callback: Callback<FsFile>): void;
  export function createSync(path: PathLike): FsFile;
  export function create(path: PathLike): Promise<FsFile>;
  export function mkdirSync(path: PathLike, options?: unknown): void;
  export function mkdir(path: PathLike, options?: unknown): Promise<void>;
  export function chmodSync(path: PathLike, mode: number): void;
  export function chmod(path: PathLike, mode: number): Promise<void>;
  export function chownSync(path: PathLike, uid: number, gid: number): void;
  export function chown(path: PathLike, uid: number, gid: number): Promise<void>;
  export function readDirSync(path: PathLike): DirEntry[];
  export function readDir(path: PathLike): AsyncIterable<DirEntry> | Promise<DirEntry[]>;
  export function statSync(path: PathLike): FileInfo;
  export function stat(path: PathLike): Promise<FileInfo>;
  export function stat(path: PathLike, callback: Callback<FileInfo>): void;
  export function lstatSync(path: PathLike): FileInfo;
  export function lstat(path: PathLike): Promise<FileInfo>;
  export function renameSync(oldPath: PathLike, newPath: PathLike): void;
  export function rename(oldPath: PathLike, newPath: PathLike): Promise<void>;
  export function removeSync(path: PathLike, options?: unknown): void;
  export function remove(path: PathLike, options?: unknown): Promise<void>;
  export function rmSync(path: PathLike, options?: unknown): void;
  export function rm(path: PathLike, options?: unknown): Promise<void>;
  export function rmdirSync(path: PathLike): void;
  export function rmdir(path: PathLike): Promise<void>;
  export function unlinkSync(path: PathLike): void;
  export function unlink(path: PathLike): Promise<void>;
  export function copyFileSync(source: PathLike, destination: PathLike): void;
  export function copyFile(source: PathLike, destination: PathLike): Promise<void>;
  export function truncateSync(path: PathLike, length?: number): void;
  export function truncate(path: PathLike, length?: number): Promise<void>;
  export function existsSync(path: PathLike): boolean;
  export function exists(path: PathLike): Promise<boolean>;
  export function accessSync(path: PathLike): void;
  export function access(path: PathLike): Promise<void>;
  export function realpathSync(path: PathLike): string;
  export function realpath(path: PathLike): Promise<string>;
  export function readlinkSync(path: PathLike): string;
  export function readlink(path: PathLike): Promise<string>;
  export function symlinkSync(target: PathLike, path: PathLike): void;
  export function symlink(target: PathLike, path: PathLike): Promise<void>;
  export function linkSync(existingPath: PathLike, newPath: PathLike): void;
  export function link(existingPath: PathLike, newPath: PathLike): Promise<void>;
  export function makeTempDirSync(options?: unknown): string;
  export function makeTempDir(options?: unknown): Promise<string>;
  export function makeTempFileSync(options?: unknown): string;
  export function makeTempFile(options?: unknown): Promise<string>;
  export const constants: Readonly<Record<string, number>>;
}

declare module "fs/promises" {
  export {
    access, appendFile, chmod, chown, copyFile, link, lstat, mkdir,
    readlink, realpath, rename, rm, rmdir, symlink, truncate, unlink
  } from "fs";
  export function readFile(path: string): Promise<Uint8Array>;
  export function readFile(path: string, options: string | { encoding: string }): Promise<string>;
  export function open(path: string, options?: unknown): Promise<import("fs").FsFile>;
  export function stat(path: string): Promise<import("fs").FileInfo>;
  export function writeFile(path: string, data: import("fs").FileData, options?: unknown): Promise<void>;
  export function readTextFile(path: string): Promise<string>;
  export function writeTextFile(path: string, data: string, options?: unknown): Promise<void>;
  export function readDir(path: string): AsyncIterable<import("fs").DirEntry> | Promise<import("fs").DirEntry[]>;
  export function remove(path: string, options?: unknown): Promise<void>;
  export function create(path: string): Promise<import("fs").FsFile>;
  export function makeTempDir(options?: unknown): Promise<string>;
  export function makeTempFile(options?: unknown): Promise<string>;
}

declare module "node:fs" { export * from "fs"; }
declare module "node:fs/promises" { export * from "fs/promises"; }
