package string_decoder

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/buffer"
	"github.com/sealdice/goja_ext/require"
)

func run(t *testing.T, script string) string {
	t.Helper()
	rt := goja.New()
	registry := new(require.Registry)
	registry.Enable(rt)
	buffer.Enable(rt)
	v, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return v.String()
}

func TestStringDecoderSplitUTF8(t *testing.T) {
	out := run(t, `
		const { StringDecoder } = require("string_decoder");
		const buf = Buffer.from([0xE4, 0xBD, 0xA0, 0xE5, 0xA5, 0xBD]); // "你好"
		const d = new StringDecoder("utf8");
		const a = d.write(buf.subarray(0, 2)); // 不完整前缀 -> ""
		const b = d.write(buf.subarray(2, 4));
		const c = d.end(buf.subarray(4));
		JSON.stringify([a, b, c]);
	`)
	want := `["","你","好"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestStringDecoderSplitUTF16LE(t *testing.T) {
	out := run(t, `
		const { StringDecoder } = require("string_decoder");
		const d = new StringDecoder("utf16le");
		const a = d.write(Buffer.from([0x41]));
		const b = d.write(Buffer.from([0x00, 0x3d, 0xd8]));
		const c = d.write(Buffer.from([0x00, 0xde]));
		const e = d.end();
		JSON.stringify([a, b, c, e]);
	`)
	want := `["","A","😀",""]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestStringDecoderBase64CarriesIncompleteGroups(t *testing.T) {
	out := run(t, `
		const { StringDecoder } = require("string_decoder");
		const d = new StringDecoder("base64");
		const a = d.write(Buffer.from([1, 2]));
		const b = d.write(Buffer.from([3]));
		const c = d.write(Buffer.from([4, 5]));
		const e = d.end();
		JSON.stringify([a, b, c, e]);
	`)
	want := `["","AQID","","BAU="]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestStringDecoderSingleByteEncodings(t *testing.T) {
	out := run(t, `
		const { StringDecoder } = require("string_decoder");
		const ascii = new StringDecoder("ascii").write(Buffer.from([0xff, 0x41]));
		const latin1 = new StringDecoder("latin1").write(Buffer.from([0xff, 0x41]));
		JSON.stringify([ascii, latin1]);
	`)
	want := "[\"\x7fA\",\"ÿA\"]"
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestStringDecoderIncompleteAtEnd(t *testing.T) {
	out := run(t, `
		const { StringDecoder } = require("string_decoder");
		const buf = Buffer.from([0xE4, 0xBD]); // 不完整
		const d = new StringDecoder("utf8");
		const a = d.write(buf);
		const b = d.end();
		JSON.stringify([a, b]);
	`)
	want := `["","�"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestStringDecoderTextAndFillLast(t *testing.T) {
	out := run(t, `
		const { StringDecoder } = require("string_decoder");
		const d = new StringDecoder("utf8");
		const a = d.write(Buffer.from("he"));
		const b = d.text(Buffer.from("llo"));
		JSON.stringify([a, b, d.encoding]);
	`)
	want := `["he","llo","utf8"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestStringDecoderHex(t *testing.T) {
	out := run(t, `
		const { StringDecoder } = require("string_decoder");
		const d = new StringDecoder("hex");
		const a = d.write(Buffer.from([0x48, 0x49]));
		const b = d.end();
		JSON.stringify([a, b]);
	`)
	want := `["4849",""]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestStringDecoderInvalidEncoding(t *testing.T) {
	rt := goja.New()
	registry := new(require.Registry)
	registry.Enable(rt)
	_, err := rt.RunString(`
		const { StringDecoder } = require("string_decoder");
		new StringDecoder("nope");
	`)
	if err == nil {
		t.Fatal("expected error for invalid encoding")
	}
}

func TestStringDecoder_FillLast(t *testing.T) {
	out := run(t, `
		const { StringDecoder } = require("string_decoder");
		const d = new StringDecoder("utf8");
		const r = d.fillLast(Buffer.from([0xE4, 0xBD]));
		const after = d.text(Buffer.from([0x41]));
		JSON.stringify([r, after]);
	`)
	want := `["�","A"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestStringDecoder_NonUTF8EndText(t *testing.T) {
	out := run(t, `
		const { StringDecoder } = require("string_decoder");
		const d = new StringDecoder("hex");
		const e = d.end(Buffer.from([0x48]));
		const x = d.text(Buffer.from([0x48]));
		JSON.stringify([e, x]);
	`)
	want := `["48","48"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestStringDecoder_EncodingAliases(t *testing.T) {
	out := run(t, `
		const { StringDecoder } = require("string_decoder");
		const cases = [
			["utf-8","utf8"], ["UTF-8","utf8"], [" utf8 ","utf8"],
			["ascii","ascii"], ["ASCII","ascii"],
			["latin1","latin1"], ["Latin1","latin1"], ["binary","latin1"],
			["base64","base64"], ["BASE64","base64"],
			["ucs2","ucs2"], ["ucs-2","ucs2"], ["utf16le","ucs2"], ["utf-16le","ucs2"],
			["UTF16LE","ucs2"], ["  UCS2  ","ucs2"]
		];
		const bad = cases.filter(([alias, want]) => {
			try { return new StringDecoder(alias).encoding !== want; }
			catch (e) { return true; }
		}).map(([alias]) => alias);
		JSON.stringify(bad);
	`)
	if out != `[]` {
		t.Fatalf("unexpected or failed aliases: %s", out)
	}
}

func TestStringDecoder_DefaultConstructor(t *testing.T) {
	out := run(t, `
		const { StringDecoder } = require("string_decoder");
		const a = new StringDecoder();
		const b = new StringDecoder(null);
		const c = new StringDecoder(undefined);
		JSON.stringify([a.encoding, b.encoding, c.encoding]);
	`)
	want := `["utf8","utf8","utf8"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}

func TestStringDecoder_MidStreamInvalidByte(t *testing.T) {
	out := run(t, `
		const { StringDecoder } = require("string_decoder");
		const d = new StringDecoder("utf8");
		const r = d.write(Buffer.from([0xFF, 0xE4, 0xBD, 0xA0]));
		JSON.stringify([r]);
	`)
	want := `["�你"]`
	if out != want {
		t.Fatalf("got %s want %s", out, want)
	}
}
