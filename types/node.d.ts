declare module "console" {
  export function log(...values: unknown[]): void;
  export function info(...values: unknown[]): void;
  export function warn(...values: unknown[]): void;
  export function error(...values: unknown[]): void;
  export function debug(...values: unknown[]): void;
  export function trace(...values: unknown[]): void;
}

declare module "path" {
  export interface ParsedPath { root: string; dir: string; base: string; name: string; ext: string; }
  export interface FormatInputPathObject { root?: string; dir?: string; base?: string; name?: string; ext?: string; }
  export interface PlatformPath {
    sep: string;
    delimiter: string;
    join(...paths: string[]): string;
    resolve(...paths: string[]): string;
    normalize(path: string): string;
    relative(from: string, to: string): string;
    isAbsolute(path: string): boolean;
    basename(path: string, suffix?: string): string;
    dirname(path: string): string;
    extname(path: string): string;
    parse(path: string): ParsedPath;
    format(path: FormatInputPathObject): string;
    toNamespacedPath(path: string): string;
  }
  export const sep: string;
  export const delimiter: string;
  export const posix: PlatformPath;
  export const win32: PlatformPath;
  export const join: PlatformPath["join"];
  export const resolve: PlatformPath["resolve"];
  export const normalize: PlatformPath["normalize"];
  export const relative: PlatformPath["relative"];
  export const isAbsolute: PlatformPath["isAbsolute"];
  export const basename: PlatformPath["basename"];
  export const dirname: PlatformPath["dirname"];
  export const extname: PlatformPath["extname"];
  export const parse: PlatformPath["parse"];
  export const format: PlatformPath["format"];
  export const toNamespacedPath: PlatformPath["toNamespacedPath"];
}

declare module "process" {
  interface RuntimeProcess { env: Record<string, string>; cwd(): string; chdir(directory: string): void; }
  const process: RuntimeProcess;
  export = process;
}

declare module "stream" {
  import { EventEmitter } from "events";
  export type Callback = (error?: Error | null, value?: unknown) => void;
  export interface ReadableOptions { read?(this: Readable, callback?: Callback): void; }
  export interface WritableOptions { write?(this: Writable, chunk: unknown, callback: Callback): void; final?(this: Writable, callback: Callback): void; }
  export interface TransformOptions extends ReadableOptions, WritableOptions { transform?(this: Transform, chunk: unknown, encoding: string, callback: Callback): void; }
  export class Stream extends EventEmitter { pipe<T extends Writable>(destination: T): T; destroy(error?: unknown): this; }
  export class Readable extends Stream implements AsyncIterable<unknown> {
    constructor(options?: ReadableOptions);
    readonly readable: boolean;
    readonly destroyed: boolean;
    push(chunk: unknown): boolean;
    read(size?: number): unknown;
    setEncoding(encoding: string): this;
    [Symbol.asyncIterator](): AsyncIterator<unknown>;
    static from(source: Iterable<unknown> | AsyncIterable<unknown>, options?: ReadableOptions): Readable;
    static toWeb(stream: Readable | Duplex): ReadableStream<unknown> | { readable: ReadableStream<unknown>; writable: WritableStream<unknown> };
    static fromWeb(stream: ReadableStream<unknown> | { readable: ReadableStream<unknown>; writable: WritableStream<unknown> }, options?: ReadableOptions & { signal?: AbortSignal; encoding?: string }): Readable | Duplex;
  }
  export class Writable extends Stream {
    constructor(options?: WritableOptions);
    readonly writable: boolean;
    write(chunk: unknown, callback?: Callback): boolean;
    end(chunk?: unknown, callback?: Callback): this;
    static toWeb(stream: Writable): WritableStream<unknown>;
    static fromWeb(stream: WritableStream<unknown>, options?: WritableOptions & { signal?: AbortSignal }): Writable;
  }
  export class Duplex extends Readable {
    readonly writable: boolean;
    write(chunk: unknown, callback?: Callback): boolean;
    end(chunk?: unknown, callback?: Callback): this;
  }
  export class Transform extends Duplex { constructor(options?: TransformOptions); }
  export class PassThrough extends Transform {}
  export function pipeline(...streams: Array<Stream | Callback>): Stream;
  export function finished(stream: Stream, callback: Callback): () => void;
  export function addAbortSignal<T extends Stream>(signal: AbortSignal, stream: T): T;
  export function duplexPair(options?: TransformOptions): [Duplex, Duplex];
  export function isStream(value: unknown): value is Stream;
  export function isReadable(value: unknown): boolean;
  export function isWritable(value: unknown): boolean;
  export function isErrored(value: unknown): boolean;
  export function isDisturbed(value: unknown): boolean;
}

declare module "string_decoder" {
  export class StringDecoder {
    readonly encoding: string;
    constructor(encoding?: string);
    write(buffer: ArrayBuffer | ArrayBufferView): string;
    end(buffer?: ArrayBuffer | ArrayBufferView): string;
    text(buffer: ArrayBuffer | ArrayBufferView): string;
    fillLast(buffer?: ArrayBuffer | ArrayBufferView): string;
  }
}

declare module "timers" {
  export interface TimerHandle { ref(): this; unref(): this; hasRef(): boolean; refresh(): this; }
  export function setTimeout(callback: (...args: unknown[]) => void, delay?: number, ...args: unknown[]): TimerHandle;
  export function clearTimeout(handle?: TimerHandle | number): void;
  export function setInterval(callback: (...args: unknown[]) => void, delay?: number, ...args: unknown[]): TimerHandle;
  export function clearInterval(handle?: TimerHandle | number): void;
  export function setImmediate(callback: (...args: unknown[]) => void, ...args: unknown[]): TimerHandle;
  export function clearImmediate(handle?: TimerHandle): void;
}

declare module "timers/promises" {
  export interface TimerOptions { ref?: boolean; signal?: AbortSignal; }
  export function setTimeout<T = void>(delay?: number, value?: T, options?: TimerOptions): Promise<T>;
  export function setImmediate<T = void>(value?: T, options?: TimerOptions): Promise<T>;
  export function setInterval<T = void>(delay?: number, value?: T, options?: TimerOptions): AsyncIterableIterator<T>;
  export const scheduler: { wait(delay?: number, options?: TimerOptions): Promise<void> };
}

declare module "util" {
  export interface InspectOptions { depth?: number; colors?: boolean; showHidden?: boolean; maxArrayLength?: number; }
  export function format(format?: unknown, ...args: unknown[]): string;
  export function inspect(value: unknown, options?: InspectOptions): string;
}

declare module "node:console" { export * from "console"; }
declare module "node:path" { export * from "path"; }
declare module "node:stream" { export * from "stream"; }
declare module "node:string_decoder" { export * from "string_decoder"; }
declare module "node:timers" { export * from "timers"; }
declare module "node:timers/promises" { export * from "timers/promises"; }
declare module "node:url" { export * from "url"; }
declare module "node:util" { export * from "util"; }
declare module "node:process" { import process = require("process"); export = process; }
