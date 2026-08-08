package string_decoder

import (
	"encoding/base64"
	"encoding/hex"
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
	if sd.isStateful() {
		sd.pending = append(sd.pending, data...)
		return sd.decodePending(false)
	}
	return sd.encode(data)
}

func (sd *stringDecoder) end(call goja.FunctionCall) goja.Value {
	if v := call.Argument(0); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		data := buffer.Bytes(sd.rt, v)
		if sd.isStateful() {
			sd.pending = append(sd.pending, data...)
		} else {
			return sd.encode(data)
		}
	}
	if sd.isStateful() {
		out := sd.decodePending(true)
		sd.pending = nil
		return out
	}
	return sd.rt.ToValue("")
}

func (sd *stringDecoder) text(call goja.FunctionCall) goja.Value {
	data := buffer.Bytes(sd.rt, call.Argument(0))
	if sd.isStateful() {
		sd.pending = append(sd.pending, data...)
		out := sd.decodePending(true)
		sd.pending = nil
		return out
	}
	return sd.encode(data)
}

func (sd *stringDecoder) fillLast(call goja.FunctionCall) goja.Value {
	if v := call.Argument(0); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		sd.pending = append(sd.pending, buffer.Bytes(sd.rt, v)...)
	}
	if sd.isStateful() {
		out := sd.decodePending(true)
		sd.pending = nil
		return out
	}
	data := sd.pending
	sd.pending = nil
	return sd.encode(data)
}

func (sd *stringDecoder) isStateful() bool {
	return sd.encoding == "utf8" || sd.encoding == "ucs2" || sd.encoding == "base64"
}

func (sd *stringDecoder) decodePending(flush bool) goja.Value {
	switch sd.encoding {
	case "utf8":
		return sd.rt.ToValue(sd.decodePendingUTF8(flush))
	case "ucs2":
		return sd.decodePendingUTF16LE(flush)
	case "base64":
		return sd.rt.ToValue(sd.decodePendingBase64(flush))
	default:
		panic(sd.rt.NewTypeError("StringDecoder encoding is not stateful: %s", sd.encoding))
	}
}

// decodePendingUTF8 retains an incomplete trailing sequence for the next
// write, or consumes it as one U+FFFD when flushing.
func (sd *stringDecoder) decodePendingUTF8(flush bool) string {
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

func (sd *stringDecoder) decodePendingUTF16LE(flush bool) goja.Value {
	usable := len(sd.pending) - len(sd.pending)%2
	if !flush && usable >= 2 {
		last := uint16(sd.pending[usable-2]) | uint16(sd.pending[usable-1])<<8
		if last >= 0xd800 && last <= 0xdbff {
			usable -= 2
		}
	}

	units := make([]uint16, usable/2)
	for i := range units {
		units[i] = uint16(sd.pending[i*2]) | uint16(sd.pending[i*2+1])<<8
	}
	if flush {
		sd.pending = nil
	} else {
		sd.pending = sd.pending[usable:]
	}
	return goja.StringFromUTF16(units)
}

func (sd *stringDecoder) decodePendingBase64(flush bool) string {
	usable := len(sd.pending)
	if !flush {
		usable -= usable % 3
	}
	encoded := base64.StdEncoding.EncodeToString(sd.pending[:usable])
	if flush {
		sd.pending = nil
	} else {
		sd.pending = sd.pending[usable:]
	}
	return encoded
}

func (sd *stringDecoder) encode(data []byte) goja.Value {
	switch sd.encoding {
	case "ascii":
		ascii := make([]byte, len(data))
		for i, value := range data {
			ascii[i] = value & 0x7f
		}
		return sd.rt.ToValue(string(ascii))
	case "latin1":
		latin1 := make([]rune, len(data))
		for i, value := range data {
			latin1[i] = rune(value)
		}
		return sd.rt.ToValue(string(latin1))
	case "hex":
		return sd.rt.ToValue(hex.EncodeToString(data))
	default:
		panic(sd.rt.NewTypeError("Unsupported StringDecoder encoding: %s", sd.encoding))
	}
}

func Require(rt *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)
	_ = exports.Set("StringDecoder", newStringDecoderCtor(rt))
}

func init() {
	require.RegisterCoreModule(ModuleName, Require)
}
