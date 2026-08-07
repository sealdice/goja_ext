package string_decoder

import (
	"strings"
	"unicode/utf8"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/buffer"
	"github.com/sealdice/goja_ext/require"
)

const ModuleName = "string_decoder"

type stringDecoder struct {
	rt       *goja.Runtime
	encoding string
	pending  []byte
}

func newStringDecoderCtor(rt *goja.Runtime) func(call goja.ConstructorCall) *goja.Object {
	return func(call goja.ConstructorCall) *goja.Object {
		enc := "utf8"
		if v := call.Argument(0); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			enc = normalizeEncoding(v.String())
		}
		if enc == "" {
			panic(rt.NewTypeError("Unknown encoding"))
		}
		sd := &stringDecoder{rt: rt, encoding: enc}
		obj := call.This
		_ = obj.Set("encoding", enc)
		_ = obj.Set("write", sd.write)
		_ = obj.Set("end", sd.end)
		_ = obj.Set("text", sd.text)
		_ = obj.Set("fillLast", sd.fillLast)
		return obj
	}
}

func normalizeEncoding(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "utf8", "utf-8":
		return "utf8"
	case "ascii":
		return "ascii"
	case "latin1", "binary":
		return "latin1"
	case "base64":
		return "base64"
	case "hex":
		return "hex"
	case "ucs2", "ucs-2", "utf16le", "utf-16le":
		return "ucs2"
	}
	return ""
}

func (sd *stringDecoder) write(call goja.FunctionCall) goja.Value {
	data := buffer.Bytes(sd.rt, call.Argument(0))
	if sd.encoding == "utf8" {
		sd.pending = append(sd.pending, data...)
		return sd.rt.ToValue(sd.decodePending(false))
	}
	return sd.rt.ToValue(sd.encode(data))
}

func (sd *stringDecoder) end(call goja.FunctionCall) goja.Value {
	if v := call.Argument(0); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		data := buffer.Bytes(sd.rt, v)
		if sd.encoding == "utf8" {
			sd.pending = append(sd.pending, data...)
		} else {
			return sd.rt.ToValue(sd.encode(data))
		}
	}
	if sd.encoding == "utf8" {
		out := sd.decodePending(true)
		sd.pending = nil
		return sd.rt.ToValue(out)
	}
	return sd.rt.ToValue("")
}

func (sd *stringDecoder) text(call goja.FunctionCall) goja.Value {
	data := buffer.Bytes(sd.rt, call.Argument(0))
	if sd.encoding == "utf8" {
		sd.pending = append(sd.pending, data...)
		out := sd.decodePending(true)
		sd.pending = nil
		return sd.rt.ToValue(out)
	}
	return sd.rt.ToValue(sd.encode(data))
}

func (sd *stringDecoder) fillLast(call goja.FunctionCall) goja.Value {
	if v := call.Argument(0); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		sd.pending = append(sd.pending, buffer.Bytes(sd.rt, v)...)
	}
	if sd.encoding == "utf8" {
		out := sd.decodePending(true)
		sd.pending = nil
		return sd.rt.ToValue(out)
	}
	data := sd.pending
	sd.pending = nil
	return sd.rt.ToValue(sd.encode(data))
}

// decodePending decodes sd.pending as UTF-8. When flush is false, an incomplete
// trailing sequence is retained for the next write; when flush is true, the
// whole incomplete tail is consumed as a single U+FFFD.
func (sd *stringDecoder) decodePending(flush bool) string {
	var sb strings.Builder
	pending := sd.pending
	for len(pending) > 0 {
		if !utf8.FullRune(pending) {
			if flush {
				sb.WriteRune(utf8.RuneError)
				pending = nil
			}
			break
		}
		r, size := utf8.DecodeRune(pending)
		if r == utf8.RuneError && size == 1 {
			sb.WriteRune(utf8.RuneError)
			pending = pending[1:]
			continue
		}
		sb.WriteRune(r)
		pending = pending[size:]
	}
	sd.pending = pending
	return sb.String()
}

func (sd *stringDecoder) encode(data []byte) goja.Value {
	return buffer.EncodeBytes(sd.rt, data, sd.rt.ToValue(sd.encoding))
}

func Require(rt *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)
	_ = exports.Set("StringDecoder", newStringDecoderCtor(rt))
}

func init() {
	require.RegisterCoreModule(ModuleName, Require)
}
