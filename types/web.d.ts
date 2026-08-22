declare module "abort" {
  export const AbortController: typeof globalThis.AbortController;
  export const AbortSignal: typeof globalThis.AbortSignal;
}

declare module "streams" {
  export const ByteLengthQueuingStrategy: typeof globalThis.ByteLengthQueuingStrategy;
  export const CountQueuingStrategy: typeof globalThis.CountQueuingStrategy;
  export const ReadableByteStreamController: typeof globalThis.ReadableByteStreamController;
  export const ReadableStream: typeof globalThis.ReadableStream;
  export const ReadableStreamBYOBReader: typeof globalThis.ReadableStreamBYOBReader;
  export const ReadableStreamBYOBRequest: typeof globalThis.ReadableStreamBYOBRequest;
  export const ReadableStreamDefaultController: typeof globalThis.ReadableStreamDefaultController;
  export const ReadableStreamDefaultReader: typeof globalThis.ReadableStreamDefaultReader;
  export const TextDecoder: typeof globalThis.TextDecoder;
  export const TextDecoderStream: typeof globalThis.TextDecoderStream;
  export const TextEncoder: typeof globalThis.TextEncoder;
  export const TextEncoderStream: typeof globalThis.TextEncoderStream;
  export const TransformStream: typeof globalThis.TransformStream;
  export const TransformStreamDefaultController: typeof globalThis.TransformStreamDefaultController;
  export const WritableStream: typeof globalThis.WritableStream;
  export const WritableStreamDefaultController: typeof globalThis.WritableStreamDefaultController;
  export const WritableStreamDefaultWriter: typeof globalThis.WritableStreamDefaultWriter;
}

declare module "stream/web" { export * from "streams"; }
declare module "node:stream/web" { export * from "streams"; }
declare module "node:abort" { export * from "abort"; }

declare module "structuredclone" {
  export function structuredClone<T>(value: T): T;
}
declare module "node:structuredclone" { export * from "structuredclone"; }
