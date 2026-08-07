package fs

import (
	"testing"
)

func TestNodeCallbackAPI(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		const results = [];
		fs.mkdirSync("docs", { recursive: true });
		fs.writeFileSync("docs/a.txt", "alpha");
		fs.readFile("docs/a.txt", "utf8", function (err, data) {
			results.push("read:" + data);
			fs.stat("docs/a.txt", function (err2, stat) {
				results.push("isFile:" + stat.isFile() + ",size:" + stat.size);
				fs.appendFile("docs/a.txt", "-beta", function (err3) {
					fs.readFile("docs/a.txt", "utf8", function (err4, data2) {
						results.push("append:" + data2);
						fs.unlink("docs/a.txt", function (err5) {
							results.push("exists:" + fs.existsSync("docs/a.txt"));
							globalThis.__result = results.join("|");
						});
					});
				});
			});
		});
	`)
	if result != "read:alpha|isFile:true,size:5|append:alpha-beta|exists:false" {
		t.Fatalf("unexpected: %s", result)
	}
}

func TestNodeEncodingAndStats(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		fs.writeFileSync("enc.txt", "你好");
		const str = fs.readFileSync("enc.txt", "utf8");
		const raw = fs.readFileSync("enc.txt");
		const stat = fs.statSync("enc.txt");
		const isDate = Object.prototype.toString.call(stat.mtime) === "[object Date]";
		const isFile = (stat.mode & 0xF000) === 0x8000;
		globalThis.__result = JSON.stringify({
			str, isDate, isFile,
			type: Object.prototype.toString.call(raw),
			size: stat.size,
		});
	`)
	if result != `{"str":"你好","isDate":true,"isFile":true,"type":"[object Uint8Array]","size":6}` {
		t.Fatalf("unexpected: %s", result)
	}
}

func TestNodeConstantsAndFlags(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		const c = fs.constants;
		const ok = c.R_OK === 4 && c.W_OK === 2 && c.X_OK === 1 &&
			(c.S_IFREG & 0xF000) === 0x8000 && c.O_CREAT === 0x40;
		const fd = fs.openSync("flags.txt", "w");
		fd.writeSync("data");
		fd.close();
		const content = fs.readFileSync("flags.txt", "utf8");
		globalThis.__result = String(ok) + ":" + content;
	`)
	if result != "true:data" {
		t.Fatalf("unexpected: %s", result)
	}
}

func TestNodeCreateReadStream(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		const { Buffer } = require("buffer");
		fs.writeFileSync("stream.txt", "stream-data");
		const chunks = [];
		const rs = fs.createReadStream("stream.txt");
		rs.on("data", function (c) { chunks.push(Buffer.from(c).toString("utf8")); });
		rs.on("end", function () { globalThis.__result = chunks.join(","); });
		rs.on("error", function (e) { globalThis.__result = "ERR:" + e.message; });
	`)
	if result != "stream-data" {
		t.Fatalf("unexpected: %s", result)
	}
}

func TestNodeCreateWriteStream(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		const ws = fs.createWriteStream("out.txt");
		ws.write("hello");
		ws.write("world");
		ws.end();
		ws.on("finish", function () {
			globalThis.__result = fs.readFileSync("out.txt", "utf8");
		});
		ws.on("error", function (e) { globalThis.__result = "ERR:" + e.message; });
	`)
	if result != "helloworld" {
		t.Fatalf("unexpected: %s", result)
	}
}

func TestNodeRmRealpathAccess(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		fs.mkdirSync("tmp", { recursive: true });
		fs.writeFileSync("tmp/x.txt", "x");
		fs.accessSync("tmp/x.txt");
		const rp = fs.realpathSync("tmp/x.txt");
		fs.rmSync("tmp", { recursive: true, force: true });
		const gone = !fs.existsSync("tmp");
		globalThis.__result = String(gone) + ":" + rp;
	`)
	if result != "true:/workspace/tmp/x.txt" {
		t.Fatalf("unexpected: %s", result)
	}
}

func TestStatsDirectoryFields(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		fs.mkdirSync("d");
		const st = fs.statSync("d");
		globalThis.__result = [
			String(st.isDirectory()),
			String(st.isFile()),
			String((st.mode & 0xF000) === 0x4000),
		].join("|");
	`)
	if result != "true|false|true" {
		t.Fatalf("unexpected directory stats: %s", result)
	}
}

func TestStatsFields(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		fs.writeFileSync("f.txt", "x");
		const st = fs.statSync("f.txt");
		globalThis.__result = [
			String(st.isFile()),
			String((st.mode & 0xF000) === 0x8000),
			String(st.birthtime === null),
			String(st.atime === null),
			String(st.ctime === null),
			Object.prototype.toString.call(st.mtime),
		].join("|");
	`)
	if result != "true|true|true|true|true|[object Date]" {
		t.Fatalf("unexpected stats fields: %s", result)
	}
}

func TestCreateReadStreamStartEnd(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		fs.writeFileSync("f", "0123456789");
		const chunks = [];
		const rs = fs.createReadStream("f", { start: 2, end: 5, encoding: "utf8" });
		rs.on("data", function (c) { chunks.push(c); });
		rs.on("end", function () { globalThis.__result = chunks.join(""); });
		rs.on("error", function (e) { globalThis.__result = "ERR:" + e.message; });
	`)
	if result != "2345" {
		t.Fatalf("unexpected start/end stream result: %s", result)
	}
}

func TestCreateReadStreamHighWaterMark(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		fs.writeFileSync("big", new Array(70000).fill("x").join(""));
		const chunks = [];
		const rs = fs.createReadStream("big", { highWaterMark: 1024 });
		rs.on("data", function (c) { chunks.push(c); });
		rs.on("end", function () { globalThis.__result = String(chunks.length > 1); });
		rs.on("error", function (e) { globalThis.__result = "ERR:" + e.message; });
	`)
	if result != "true" {
		t.Fatalf("unexpected highWaterMark chunking result: %s", result)
	}
}

func TestParseNodeFlags(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		const results = [];

		fs.writeFileSync("a.txt", "AB");
		const fa = fs.openSync("a.txt", "a");
		fa.writeSync("C");
		fa.close();
		results.push(fs.readFileSync("a.txt", "utf8"));

		fs.writeFileSync("wx.txt", "x");
		let wxErr = null;
		try { fs.openSync("wx.txt", "wx"); } catch (e) { wxErr = e; }
		results.push(wxErr ? wxErr.code : "no-error");

		fs.writeFileSync("r.txt", "read");
		const fr = fs.openSync("r.txt", "r");
		const buf = new Uint8Array(4);
		fr.readSync(buf);
		fr.close();
		results.push(Array.prototype.slice.call(buf).join(","));

		globalThis.__result = results.join("|");
	`)
	if result != "ABC|EEXIST|114,101,97,100" {
		t.Fatalf("unexpected parseNodeFlags result: %s", result)
	}
}

func TestInvalidNodeFlags(t *testing.T) {
	result := runFSAPIScript(t, `
		const fs = require("fs");
		let err = null;
		try { fs.openSync("f", "zzz"); } catch (e) { err = e; }
		globalThis.__result = [
			String(err instanceof TypeError),
			String(err.message.indexOf("Unknown file open flags") >= 0),
		].join("|");
	`)
	if result != "true|true" {
		t.Fatalf("unexpected invalid flags result: %s", result)
	}
}
