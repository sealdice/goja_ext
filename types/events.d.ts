declare module "events" {
  export type EventName = string | symbol;
  export type Listener = (...args: unknown[]) => unknown;

  export class EventEmitter {
    static defaultMaxListeners: number;
    addListener(name: EventName, listener: Listener): this;
    on(name: EventName, listener: Listener): this;
    once(name: EventName, listener: Listener): this;
    prependListener(name: EventName, listener: Listener): this;
    prependOnceListener(name: EventName, listener: Listener): this;
    removeListener(name: EventName, listener: Listener): this;
    off(name: EventName, listener: Listener): this;
    removeAllListeners(name?: EventName): this;
    emit(name: EventName, ...args: unknown[]): boolean;
    eventNames(): EventName[];
    listeners(name: EventName): Listener[];
    rawListeners(name: EventName): Listener[];
    listenerCount(name: EventName): number;
    getMaxListeners(): number;
    setMaxListeners(value: number): this;
  }

  export function once(emitter: EventEmitter, name: EventName, options?: { signal?: AbortSignal }): Promise<unknown[]>;
  export function on(emitter: EventEmitter, name: EventName, options?: { signal?: AbortSignal }): AsyncIterableIterator<unknown[]>;
  export function listenerCount(emitter: EventEmitter, name: EventName): number;
  export function getEventListeners(emitter: EventEmitter, name: EventName): Listener[];
  export function getMaxListeners(emitter: EventEmitter): number;
  export function setMaxListeners(value: number, ...emitters: EventEmitter[]): void;
  export function addAbortListener(signal: AbortSignal, listener: Listener): { dispose(): void };
  export function emit(emitter: EventEmitter, ...args: unknown[]): boolean;
  export const SymbolRejection: symbol;
  export let defaultMaxListeners: number;
}

declare module "node:events" { export * from "events"; }
