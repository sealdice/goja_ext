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
