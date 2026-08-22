/// <reference path="../index.d.ts" />

import { AbortController as RuntimeAbortController } from "abort";
import { Buffer } from "buffer";
import * as runtimeConsole from "console";
import { EventEmitter, once } from "events";
import { fetchEventSource, EventStreamContentType } from "@microsoft/fetch-event-source";
import * as fs from "fs";
import * as fsPromises from "fs/promises";
import * as path from "path";
import runtimeProcess = require("process");
import { ReadableStream as RuntimeReadableStream } from "streams";
import { StringDecoder } from "string_decoder";
import { structuredClone as clone } from "structuredclone";
import * as timers from "timers";
import { setTimeout as delay } from "timers/promises";
import { URL as RuntimeURL } from "url";
import { format, inspect } from "util";

const abortController = new RuntimeAbortController();
abortController.abort("stop");
const bytes: Uint8Array = Buffer.from("hello");
runtimeConsole.log(format("%s", inspect(bytes)));

const emitter = new EventEmitter();
const eventPromise: Promise<unknown[]> = once(emitter, "ready");
emitter.emit("ready", 1);
void eventPromise;

const request: Promise<void> = fetchEventSource("https://example.test/sse", {
  method: "POST",
  headers: { Authorization: "Bearer test" },
  body: "{}",
  async onopen(response) { void response.status; },
  onmessage(message) { void message.data; },
});
void request;
void EventStreamContentType;

fs.writeFileSync("a.txt", bytes);
fs.writeTextFileSync("text.txt", "a");
const fileInfo: Promise<fs.FileInfo> = fs.stat("a.txt");
const dirEntries: AsyncIterable<fs.DirEntry> = fs.readDir(".");
const fileBytes: Promise<Uint8Array> = fsPromises.readFile("a.txt");
void fileInfo;
void dirEntries;
void fileBytes;
void path.posix.join("a", "b");
runtimeProcess.chdir(runtimeProcess.cwd());

const webStream = new RuntimeReadableStream<Uint8Array>({ start(controller) { controller.close(); } });
const streamedWrite: Promise<void> = fs.writeFile("stream.bin", webStream, {
  signal: abortController.signal,
});
void streamedWrite;
void fetch("https://example.test/upload", {
  method: "POST",
  body: webStream,
  duplex: "half",
});

const decoder = new StringDecoder("utf8");
void decoder.write(bytes);
void clone({ ok: true });

const timer = timers.setTimeout(() => undefined, 1);
timers.clearTimeout(timer);
const delayed: Promise<string> = delay(1, "ok");
void delayed;

const parsed = new RuntimeURL("https://example.test/");
void parsed.hostname;

const encoded = new TextEncoder().encode("ok");
void new TextDecoder().decode(encoded);
const globalController = new AbortController();
globalController.abort();
void new Headers();
void new Request("https://example.test/");
void new Response();
void new FormData();
void new WebSocket("wss://example.test/");
void structuredClone({ value: 1 });
void process.env;
void console.log;
void fs;

import("node:events");
import("node:abort");
import("node:fetch");
// @ts-expect-error Node-style fs is intentionally unsupported.
import("node:fs");
// @ts-expect-error Node-style fs promises are intentionally unsupported.
import("node:fs/promises");
import("node:path");
import("node:process");
// @ts-expect-error Node classic streams are intentionally unsupported.
import("node:stream");
import("node:stream/web");
// @ts-expect-error The nonstandard plural Node alias is intentionally unsupported.
import("node:streams");
// @ts-expect-error Node classic streams are intentionally unsupported.
import("stream");
import("node:string_decoder");
import("node:structuredclone");
import("node:timers");
import("node:timers/promises");
import("node:url");
import("node:util");
import("node:@microsoft/fetch-event-source");
