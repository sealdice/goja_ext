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
import { Readable, Transform } from "stream";
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

fs.writeFileSync("a.txt", "a");
fs.readFile("a.txt", (error, value) => { if (!error) void value.byteLength; });
const fileBytes: Promise<Uint8Array> = fsPromises.readFile("a.txt");
void fileBytes;
void path.posix.join("a", "b");
runtimeProcess.chdir(runtimeProcess.cwd());

const readable = new Readable({ read() { this.push(bytes); this.push(null); } });
const transform = new Transform({ transform(chunk, _encoding, callback) { callback(null, chunk); } });
readable.pipe(transform);

const webStream = new RuntimeReadableStream<Uint8Array>({ start(controller) { controller.close(); } });
const classic = Readable.fromWeb(webStream);
void classic;
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
import("node:fs");
import("node:fs/promises");
import("node:path");
import("node:process");
import("node:stream");
import("node:stream/web");
import("node:streams");
import("node:string_decoder");
import("node:structuredclone");
import("node:timers");
import("node:timers/promises");
import("node:url");
import("node:util");
import("node:@microsoft/fetch-event-source");
