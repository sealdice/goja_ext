package streams

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/buffer"
)

var textExportNames = [...]string{
	"TextEncoderStream",
	"TextDecoderStream",
}

func encodeUTF8(rt *goja.Runtime, call goja.FunctionCall) goja.Value {
	return buffer.WrapBytes(rt, []byte(call.Argument(0).String()))
}

func decodeUTF8(rt *goja.Runtime, call goja.FunctionCall) goja.Value {
	chunk := decodeInputBytes(rt, call.Argument(0))
	pending := decodeInputBytes(rt, call.Argument(1))
	final := call.Argument(2).ToBoolean()
	fatal := call.Argument(3).ToBoolean()
	stripBOM := call.Argument(4).ToBoolean()

	data := make([]byte, 0, len(pending)+len(chunk))
	data = append(data, pending...)
	data = append(data, chunk...)
	bomPending := false
	if stripBOM {
		data, bomPending = stripUTF8BOM(data, final)
	}
	if bomPending {
		result := rt.NewObject()
		if err := result.Set("text", ""); err != nil {
			panic(err)
		}
		if err := result.Set("pending", buffer.WrapBytes(rt, data)); err != nil {
			panic(err)
		}
		if err := result.Set("bomPending", true); err != nil {
			panic(err)
		}
		return result
	}

	text, remaining, err := decodeUTF8Chunk(data, final, fatal)
	if err != nil {
		panic(rt.NewTypeError(err.Error()))
	}

	result := rt.NewObject()
	if err := result.Set("text", text); err != nil {
		panic(err)
	}
	if err := result.Set("pending", buffer.WrapBytes(rt, remaining)); err != nil {
		panic(err)
	}
	if err := result.Set("bomPending", false); err != nil {
		panic(err)
	}
	return result
}

func decodeInputBytes(rt *goja.Runtime, value goja.Value) []byte {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil
	}
	var data []byte
	if exception := rt.Try(func() {
		data = buffer.DecodeBytes(rt, value, goja.Undefined())
	}); exception != nil {
		panic(exception)
	}
	return data
}

func stripUTF8BOM(data []byte, final bool) ([]byte, bool) {
	if len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf {
		return data[3:], false
	}
	if !final && isUTF8BOMPrefix(data) {
		return data, true
	}
	return data, false
}

func isUTF8BOMPrefix(data []byte) bool {
	switch len(data) {
	case 0:
		return false
	case 1:
		return data[0] == 0xef
	case 2:
		return data[0] == 0xef && data[1] == 0xbb
	default:
		return false
	}
}

func decodeUTF8Chunk(data []byte, final, fatal bool) (string, []byte, error) {
	var out strings.Builder
	for offset := 0; offset < len(data); {
		sequenceLength := utf8SequenceLength(data[offset])
		if sequenceLength == 0 {
			if fatal {
				return "", nil, errors.New("invalid UTF-8 sequence")
			}
			out.WriteRune(utf8.RuneError)
			offset++
			continue
		}
		if offset+sequenceLength > len(data) {
			if !final {
				return out.String(), append([]byte(nil), data[offset:]...), nil
			}
			if fatal {
				return "", nil, errors.New("incomplete UTF-8 sequence")
			}
			out.WriteRune(utf8.RuneError)
			break
		}

		sequence := data[offset : offset+sequenceLength]
		if !utf8.Valid(sequence) {
			if fatal {
				return "", nil, errors.New("invalid UTF-8 sequence")
			}
			out.WriteRune(utf8.RuneError)
			offset++
			continue
		}
		out.Write(sequence)
		offset += sequenceLength
	}
	return out.String(), nil, nil
}

func utf8SequenceLength(first byte) int {
	switch {
	case first < 0x80:
		return 1
	case first >= 0xc2 && first <= 0xdf:
		return 2
	case first >= 0xe0 && first <= 0xef:
		return 3
	case first >= 0xf0 && first <= 0xf4:
		return 4
	default:
		return 0
	}
}
