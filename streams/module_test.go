package streams_test

import (
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/streams"
)

func runStreamsScript(t *testing.T, script string) string {
	t.Helper()

	loop := eventloop.NewEventLoop(eventloop.EnableConsole(false))
	go loop.StartInForeground()
	t.Cleanup(func() { loop.Stop() })

	done := make(chan struct{})
	var result string
	var runErr error
	var rec any

	loop.RunOnLoop(func(vm *goja.Runtime) {
		defer func() {
			rec = recover()
			loop.SetTimeout(func(vm *goja.Runtime) {
				if v := vm.Get("__result"); v != nil && !goja.IsUndefined(v) {
					result = v.String()
				}
				close(done)
			}, 10*time.Millisecond)
		}()
		streams.Enable(vm)
		_, runErr = vm.RunString(script)
	})

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for streams script")
	}
	if rec != nil {
		t.Fatalf("panic: %v", rec)
	}
	if runErr != nil {
		t.Fatalf("script failed: %v", runErr)
	}
	return result
}

func TestModuleExportsAndGlobals(t *testing.T) {
	result := runStreamsScript(t, `
		const expected = [
			"ByteLengthQueuingStrategy",
			"CountQueuingStrategy",
			"ReadableByteStreamController",
			"ReadableStream",
			"ReadableStreamBYOBReader",
			"ReadableStreamBYOBRequest",
			"ReadableStreamDefaultController",
			"ReadableStreamDefaultReader",
			"TextDecoder",
			"TextDecoderStream",
			"TextEncoder",
			"TextEncoderStream",
			"TransformStream",
			"TransformStreamDefaultController",
			"WritableStream",
			"WritableStreamDefaultController",
			"WritableStreamDefaultWriter",
		].sort().join(",");
		const actual = Object.keys(require("streams")).sort().join(",");
		if (actual !== expected) throw new Error("exports: " + actual);
		for (const name of expected.split(",")) {
			if (typeof globalThis[name] !== "function") {
				throw new Error(name + " not installed globally");
			}
		}
		globalThis.__result = actual;
	`)
	if result != "ByteLengthQueuingStrategy,CountQueuingStrategy,ReadableByteStreamController,ReadableStream,ReadableStreamBYOBReader,ReadableStreamBYOBRequest,ReadableStreamDefaultController,ReadableStreamDefaultReader,TextDecoder,TextDecoderStream,TextEncoder,TextEncoderStream,TransformStream,TransformStreamDefaultController,WritableStream,WritableStreamDefaultController,WritableStreamDefaultWriter" {
		t.Fatalf("unexpected exports: %s", result)
	}
}

func TestModuleAliasesShareConstructors(t *testing.T) {
	result := runStreamsScript(t, `
		const streams = require("streams");
		const streamWeb = require("stream/web");
		const nodeStreamWeb = require("node:stream/web");
		globalThis.__result = String(
			streams.ReadableStream === streamWeb.ReadableStream &&
			streamWeb.ReadableStream === nodeStreamWeb.ReadableStream
		);
	`)
	if result != "true" {
		t.Fatalf("module aliases do not share constructors: %s", result)
	}
}

func TestNodeStreamsPluralAliasIsNotRegistered(t *testing.T) {
	result := runStreamsScript(t, `
		try {
			require("node:streams");
			globalThis.__result = "loaded";
		} catch {
			globalThis.__result = "missing";
		}
	`)
	if result != "missing" {
		t.Fatalf("unexpected node:streams module result: %s", result)
	}
}

func TestReadableStreamDefaultReaderReadsQueuedChunks(t *testing.T) {
	result := runStreamsScript(t, `
		const stream = new ReadableStream({
			start(controller) {
				controller.enqueue("a");
				controller.enqueue("b");
				controller.close();
			},
		});
		if (stream.locked) throw new Error("stream locked before reader");
		const reader = stream.getReader();
		if (!stream.locked) throw new Error("stream not locked after reader");
		Promise.resolve()
			.then(() => reader.read())
			.then((r1) => reader.read().then((r2) => [r1, r2]))
			.then((items) => reader.read().then((r3) => {
				globalThis.__result = [
					items[0].value + ":" + items[0].done,
					items[1].value + ":" + items[1].done,
					String(r3.value) + ":" + r3.done,
				].join(",");
			}));
	`)
	if result != "a:false,b:false,undefined:true" {
		t.Fatalf("unexpected read result: %s", result)
	}
}

func TestWritableStreamDefaultWriterWritesAndCloses(t *testing.T) {
	result := runStreamsScript(t, `
		const chunks = [];
		const stream = new WritableStream({
			write(chunk, controller) {
				if (!controller) throw new Error("missing controller");
				chunks.push(chunk);
			},
			close() {
				chunks.push("closed");
			},
		});
		if (stream.locked) throw new Error("stream locked before writer");
		const writer = stream.getWriter();
		if (!stream.locked) throw new Error("stream not locked after writer");
		writer.write("a")
			.then(() => writer.write("b"))
			.then(() => writer.close())
			.then(() => {
				globalThis.__result = chunks.join(",");
			});
	`)
	if result != "a,b,closed" {
		t.Fatalf("unexpected write result: %s", result)
	}
}

func TestQueuingStrategiesAndTypeValidation(t *testing.T) {
	result := runStreamsScript(t, `
		const countStrategy = new CountQueuingStrategy({ highWaterMark: 2 });
		const byteStrategy = new ByteLengthQueuingStrategy({ highWaterMark: 8 });
		const checks = [
			countStrategy.highWaterMark,
			countStrategy.size("x"),
			byteStrategy.highWaterMark,
			byteStrategy.size(new Uint8Array(3)),
		];
		try {
			ReadableStream({});
			throw new Error("ReadableStream without new did not throw");
		} catch (err) {
			checks.push(err instanceof TypeError);
		}
		try {
			new WritableStream({ type: null });
			throw new Error("writable type did not throw");
		} catch (err) {
			checks.push(err instanceof RangeError);
		}
		globalThis.__result = checks.join(",");
	`)
	if result != "2,1,8,3,true,true" {
		t.Fatalf("unexpected checks: %s", result)
	}
}

func TestAdvancedReadableAndTransformFeatures(t *testing.T) {
	result := runStreamsScript(t, `
		const values = [];
		const source = ReadableStream.from(["a", "b"]);
		const iterator = source.values();
		const iteration = iterator.next().then(function (first) {
			values.push(first.value);
			return iterator.next();
		}).then(function (second) {
			values.push(second.value);
		});

		const transformed = ReadableStream.from(["c"]).pipeThrough(new TransformStream({
			transform(chunk, controller) {
				controller.enqueue(chunk.toUpperCase());
			},
		}));
		const transformation = transformed.getReader().read().then(function (item) {
			return item.value;
		});

		const written = [];
		const piping = ReadableStream.from(["d"]).pipeTo(new WritableStream({
			write(chunk) {
				written.push(chunk);
			},
		})).then(function () {
			return written.join("");
		});

		const branches = ReadableStream.from(["e"]).tee();
		const teeing = Promise.all(branches.map(function (branch) {
			return branch.getReader().read().then(function (item) {
				return item.value;
			});
		})).then(function (items) {
			return items.join("");
		});

		Promise.all([iteration, transformation, piping, teeing]).then(function (items) {
			globalThis.__result = [
				values.join(""),
				items[1],
				items[2],
				items[3],
			].join(",");
		});
	`)
	if result != "ab,C,d,ee" {
		t.Fatalf("unexpected advanced feature result: %s", result)
	}
}

func TestReadableByteStreamAndBYOBReader(t *testing.T) {
	result := runStreamsScript(t, `
		const stream = new ReadableStream({
			type: "bytes",
			start(controller) {
				controller.enqueue(new Uint8Array([1, 2, 3, 4]));
				controller.close();
			},
		});
		const reader = stream.getReader({ mode: "byob" });
		reader.read(new Uint8Array(4)).then(function (item) {
			globalThis.__result =
				Array.prototype.join.call(item.value, "") + ":" + item.done;
		});
	`)
	if result != "1234:false" {
		t.Fatalf("unexpected BYOB result: %s", result)
	}
}

func TestTextEncoderAndDecoderStreams(t *testing.T) {
	result := runStreamsScript(t, `
		function collect(reader, values) {
			return reader.read().then(function (item) {
				if (item.done) return values;
				values.push(item.value);
				return collect(reader, values);
			});
		}

		const stream = new TextEncoderStream();
		const reader = stream.readable.getReader();
		const writer = stream.writable.getWriter();
		const reading = collect(reader, []);
		const writing = writer.write("A€").then(function () {
			return writer.close();
		});
		Promise.all([reading, writing]).then(function (items) {
			globalThis.__result = Array.prototype.slice.call(items[0][0]).concat(
				Array.prototype.slice.call(items[0][1] || [])
			).join(",");
		});
	`)
	if result != "65,226,130,172" {
		t.Fatalf("unexpected encoder stream result: %s", result)
	}
	result = runStreamsScript(t, `
		const stream = new TextDecoderStream();
		const reader = stream.readable.getReader();
		const writer = stream.writable.getWriter();
		const reading = reader.read().then(function (item) {
			return item.value;
		});
		const writing = writer.write(new Uint8Array([0xe2])).then(function () {
			return writer.write(new Uint8Array([0x82, 0xac]));
		});
		Promise.all([reading, writing]).then(function (items) {
			globalThis.__result = items[0];
		});
	`)
	if result != "€" {
		t.Fatalf("unexpected decoder stream result: %s", result)
	}
}

func TestTextEncoderAndDecoderGlobalsAndExports(t *testing.T) {
	result := runStreamsScript(t, `
		const web = require("streams");
		const encoded = new TextEncoder().encode("A€");
		globalThis.__result = JSON.stringify([
			TextEncoder === web.TextEncoder,
			TextDecoder === web.TextDecoder,
			new TextEncoder().encoding,
			new TextDecoder().encoding,
			Array.from(encoded),
			new TextDecoder().decode(encoded),
		]);
	`)
	want := `[true,true,"utf-8","utf-8",[65,226,130,172],"A€"]`
	if result != want {
		t.Fatalf("unexpected text codec exports: got %s want %s", result, want)
	}
}

func TestTextEncoderEncodeIntoDoesNotSplitCodePoint(t *testing.T) {
	result := runStreamsScript(t, `
		const encoder = new TextEncoder();
		const short = new Uint8Array(3);
		const first = encoder.encodeInto("€A", short);
		const full = new Uint8Array(4);
		const second = encoder.encodeInto("€A", full);
		globalThis.__result = JSON.stringify([
			first, Array.from(short), second, Array.from(full)
		]);
	`)
	want := `[{"read":1,"written":3},[226,130,172],{"read":2,"written":4},[226,130,172,65]]`
	if result != want {
		t.Fatalf("unexpected encodeInto result: got %s want %s", result, want)
	}
}

func TestTextDecoderStreamingBOMAndErrors(t *testing.T) {
	result := runStreamsScript(t, `
		const decoder = new TextDecoder();
		const split = decoder.decode(new Uint8Array([0xef, 0xbb]), { stream: true }) +
			decoder.decode(new Uint8Array([0xbf, 0xe2]), { stream: true }) +
			decoder.decode(new Uint8Array([0x82, 0xac]));
		const kept = new TextDecoder("utf8", { ignoreBOM: true })
			.decode(new Uint8Array([0xef, 0xbb, 0xbf, 65]));
		const afterEmpty = new TextDecoder();
		afterEmpty.decode(new Uint8Array(0), { stream: true });
		const emptyThenBOM = afterEmpty.decode(new Uint8Array([0xef, 0xbb, 0xbf, 66]));
		let fatal = "no-throw";
		try {
			new TextDecoder("utf-8", { fatal: true }).decode(new Uint8Array([0xff]));
		} catch (error) {
			fatal = error instanceof TypeError ? "TypeError" : String(error);
		}
		let unsupported = "no-throw";
		try { new TextDecoder("utf-16"); }
		catch (error) { unsupported = error instanceof RangeError ? "RangeError" : String(error); }
		globalThis.__result = JSON.stringify([split, kept, emptyThenBOM, fatal, unsupported]);
	`)
	want := `["€","` + string(rune(0xfeff)) + `A","B","TypeError","RangeError"]`
	if result != want {
		t.Fatalf("unexpected decoder result: got %s want %s", result, want)
	}
}

func TestTextEncoderStreamEmitsNativeUint8Array(t *testing.T) {
	result := runStreamsScript(t, `
		const stream = new TextEncoderStream();
		const reader = stream.readable.getReader();
		const writer = stream.writable.getWriter();
		reader.read().then(function (item) {
			globalThis.__result = String(
				item.value.constructor === Uint8Array &&
				Object.getPrototypeOf(item.value) === Uint8Array.prototype
			);
		});
		writer.write("native").then(function () { return writer.close(); });
	`)
	if result != "true" {
		t.Fatalf("encoder emitted non-native byte chunk: %s", result)
	}
}

func TestTextEncoderStreamBuffersSplitSurrogatePair(t *testing.T) {
	result := runStreamsScript(t, `
		function collect(reader, values) {
			return reader.read().then(function (item) {
				if (item.done) return values;
				values.push(item.value);
				return collect(reader, values);
			});
		}

		const stream = new TextEncoderStream();
		const reader = stream.readable.getReader();
		const writer = stream.writable.getWriter();
		const reading = collect(reader, []);
		writer.write("\uD83D")
			.then(function () { return writer.write("\uDE00"); })
			.then(function () { return writer.close(); });
		reading.then(function (chunks) {
			let bytes = [];
			for (const chunk of chunks) {
				bytes = bytes.concat(Array.prototype.slice.call(chunk));
			}
			globalThis.__result = bytes.join(",");
		});
	`)
	if result != "240,159,152,128" {
		t.Fatalf("unexpected split surrogate encoding: %s", result)
	}
}

func TestTextDecoderStreamStripsSplitBOM(t *testing.T) {
	result := runStreamsScript(t, `
		function collect(reader, values) {
			return reader.read().then(function (item) {
				if (item.done) return values;
				values.push(item.value);
				return collect(reader, values);
			});
		}

		const stream = new TextDecoderStream();
		const reader = stream.readable.getReader();
		const writer = stream.writable.getWriter();
		const reading = collect(reader, []);
		writer.write(new Uint8Array(0))
			.then(function () { return writer.write(new Uint8Array([0xef])); })
			.then(function () { return writer.write(new Uint8Array([0xbb, 0xbf, 65])); })
			.then(function () { return writer.close(); });
		reading.then(function (chunks) {
			globalThis.__result = chunks.join("");
		});
	`)
	if result != "A" {
		t.Fatalf("unexpected split BOM decoding: %q", result)
	}
}

func TestControllerIdentityAndBrandChecks(t *testing.T) {
	result := runStreamsScript(t, `
		let readableController;
		let readablePullController;
		let writableController;
		let writableWriteController;
		let readablePulls = 0;
		const readable = new ReadableStream({
			start(controller) {
				readableController = controller;
			},
			pull(controller) {
				readablePullController = controller;
				if (++readablePulls === 1) controller.close();
			},
		});
		const readableRead = readable.getReader().read();

		const writable = new WritableStream({
			start(controller) {
				writableController = controller;
			},
			write(_, controller) {
				writableWriteController = controller;
			},
		});
		const writer = writable.getWriter();
		const writableWrite = writer.write("x");

		Promise.all([readableRead, writableWrite]).then(function () {
			const checks = [
				readableController === readablePullController,
				writableController === writableWriteController,
			];
			for (const ctor of [
				ReadableStreamDefaultController,
				WritableStreamDefaultController,
				ReadableByteStreamController,
				TransformStreamDefaultController,
			]) {
				try {
					new ctor();
					checks.push(false);
				} catch (err) {
					checks.push(err instanceof TypeError);
				}
			}
			try {
				ReadableStream.prototype.getReader.call({});
				checks.push(false);
			} catch (err) {
				checks.push(err instanceof TypeError);
			}
			globalThis.__result = checks.join(",");
		});
	`)
	if result != "true,true,true,true,true,true,true" {
		t.Fatalf("unexpected controller/brand checks: %s", result)
	}
}

func TestTextDecoderStreamFatal(t *testing.T) {
	result := runStreamsScript(t, `
		const stream = new TextDecoderStream("utf-8", { fatal: true });
		const reader = stream.readable.getReader();
		const writer = stream.writable.getWriter();
		writer.write(new Uint8Array([0xff])).catch(function () {});
		reader.read().then(
			function (item) { globalThis.__result = "value:" + item.value; },
			function (err) {
				globalThis.__result = (err instanceof TypeError ? "TypeError" : "Other") + ":" + err.message;
			}
		);
	`)
	if result != "TypeError:invalid UTF-8 sequence" {
		t.Fatalf("unexpected fatal result: %s", result)
	}
}

func TestTextDecoderStreamIgnoreBOM(t *testing.T) {
	result := runStreamsScript(t, `
		function collect(reader, values) {
			return reader.read().then(function (item) {
				if (item.done) return values;
				values.push(item.value);
				return collect(reader, values);
			});
		}

		const kept = new TextDecoderStream("utf-8", { ignoreBOM: true });
		const keptReader = kept.readable.getReader();
		const keptWriter = kept.writable.getWriter();
		const keptReading = collect(keptReader, []);
		keptWriter.write(new Uint8Array([0xEF, 0xBB, 0xBF, 0x61])).then(function () {
			return keptWriter.close();
		});

		const stripped = new TextDecoderStream("utf-8");
		const strippedReader = stripped.readable.getReader();
		const strippedWriter = stripped.writable.getWriter();
		const strippedReading = collect(strippedReader, []);
		strippedWriter.write(new Uint8Array([0xEF, 0xBB, 0xBF, 0x61])).then(function () {
			return strippedWriter.close();
		});

		Promise.all([keptReading, strippedReading]).then(function (items) {
			globalThis.__result = items[0].join("") + "|" + items[1].join("");
		});
	`)
	if result != "\ufeffa|a" {
		t.Fatalf("unexpected ignoreBOM result: %q", result)
	}
}

func TestTextDecoderStreamUnsupportedEncoding(t *testing.T) {
	result := runStreamsScript(t, `
		try {
			new TextDecoderStream("utf-16");
			globalThis.__result = "no-throw";
		} catch (err) {
			globalThis.__result = (err instanceof RangeError ? "RangeError" : "Other") + ":" + err.message;
		}
	`)
	if result != "RangeError:The encoding label is not supported" {
		t.Fatalf("unexpected unsupported encoding result: %s", result)
	}
}

func TestReadableStreamCancel(t *testing.T) {
	result := runStreamsScript(t, `
		let cancelReason = null;
		const stream = new ReadableStream({
			start(controller) { controller.enqueue("first"); },
			cancel(reason) { cancelReason = reason; },
		});
		const reader = stream.getReader();
		reader.read().then(function () {
			return reader.cancel("done");
		}).then(function () {
			return reader.read();
		}).then(function (after) {
			globalThis.__result = cancelReason + ":" + after.done;
		}, function (err) {
			globalThis.__result = "err:" + err;
		});
	`)
	if result != "done:true" {
		t.Fatalf("unexpected cancel result: %s", result)
	}
}

func TestWritableStreamAbort(t *testing.T) {
	result := runStreamsScript(t, `
		let abortReason = null;
		const stream = new WritableStream({
			abort(reason) { abortReason = String(reason); },
		});
		const writer = stream.getWriter();
		writer.abort("boom").then(function () {
			globalThis.__result = "aborted:" + abortReason;
		}, function (err) {
			globalThis.__result = "err:" + err;
		});
	`)
	if result != "aborted:boom" {
		t.Fatalf("unexpected abort result: %s", result)
	}
}
