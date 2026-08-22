package fetch //nolint:testpackage

import (
	"runtime"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/streams"
)

func BenchmarkResponseBytesValue64KiB(b *testing.B) {
	rt := goja.New()
	chunk := make([]byte, 64*1024)
	b.SetBytes(int64(len(chunk)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		value := streams.Uint8ArrayChunk(rt, chunk)
		runtime.KeepAlive(value)
	}
}
