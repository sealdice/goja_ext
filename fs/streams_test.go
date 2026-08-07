package fs

import "testing"

func TestFsFileReadableStreamReadsFile(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		fs.writeTextFileSync("stream.txt", "hello");
		const file = fs.openSync("stream.txt", { read: true });
		const reader = file.readable.getReader();
		reader.read()
			.then((first) => reader.read().then((second) => {
				file.close();
				globalThis.__result = [
					Array.prototype.slice.call(first.value).join(","),
					first.done,
					String(second.value),
					second.done,
				].join("|");
			}));
	`)
	if result != "104,101,108,108,111|false|undefined|true" {
		t.Fatalf("unexpected readable stream result: %s", result)
	}
}

func TestFsFileWritableStreamWritesFile(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		const file = fs.openSync("writable.txt", {
			write: true,
			create: true,
			truncate: true,
		});
		const writer = file.writable.getWriter();
		writer.write(new Uint8Array([65]))
			.then(() => writer.write("B"))
			.then(() => writer.close())
			.then(() => {
				file.close();
				globalThis.__result = fs.readTextFileSync("writable.txt");
			});
	`)
	if result != "AB" {
		t.Fatalf("unexpected writable stream result: %s", result)
	}
}

func TestWriteFileConsumesReadableStream(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		const { ReadableStream } = require("streams");
		const source = new ReadableStream({
			start(controller) {
				controller.enqueue(new Uint8Array([65]));
				controller.enqueue("B");
				controller.close();
			},
		});
		fs.writeFile("from-stream.txt", source).then(() => {
			globalThis.__result = fs.readTextFileSync("from-stream.txt");
		});
	`)
	if result != "AB" {
		t.Fatalf("unexpected writeFile stream result: %s", result)
	}
}

func TestWriteTextFileConsumesReadableStream(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		const { ReadableStream } = require("streams");
		const source = new ReadableStream({
			start(controller) {
				controller.enqueue("你");
				controller.enqueue("好");
				controller.close();
			},
		});
		fs.writeTextFile("text-stream.txt", source).then(() => {
			globalThis.__result = fs.readTextFileSync("text-stream.txt");
		});
	`)
	if result != "你好" {
		t.Fatalf("unexpected writeTextFile stream result: %s", result)
	}
}
