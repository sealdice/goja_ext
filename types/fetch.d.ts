declare module "fetch" {
  export const Headers: typeof globalThis.Headers;
  export const Request: typeof globalThis.Request;
  export const Response: typeof globalThis.Response;
  export const FormData: typeof globalThis.FormData;
  export const Blob: typeof globalThis.Blob;
  export const File: typeof globalThis.File;
}

declare module "@microsoft/fetch-event-source" {
  export const EventStreamContentType: "text/event-stream";

  export interface EventSourceMessage {
    data: string;
    event: string;
    id: string;
    retry?: number;
  }

  export interface FetchEventSourceInit extends RequestInit {
    headers?: Record<string, string>;
    onopen?(response: Response): Promise<void>;
    onmessage?(message: EventSourceMessage): void;
    onclose?(): void;
    onerror?(error: unknown): number | null | undefined | void;
    openWhenHidden?: boolean;
    fetch?: typeof globalThis.fetch;
  }

  export function fetchEventSource(input: RequestInfo, init: FetchEventSourceInit): Promise<void>;
}

declare module "node:fetch" { export * from "fetch"; }
declare module "node:@microsoft/fetch-event-source" { export * from "@microsoft/fetch-event-source"; }
