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
declare module "node:string_decoder" { export * from "string_decoder"; }
declare module "node:timers" { export * from "timers"; }
declare module "node:timers/promises" { export * from "timers/promises"; }
declare module "node:url" { export * from "url"; }
declare module "node:util" { export * from "util"; }
declare module "node:process" { import process = require("process"); export = process; }
