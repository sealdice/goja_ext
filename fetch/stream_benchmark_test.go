package fetch //nolint:testpackage

import (
	"bytes"
	"io"
	"runtime"
	"testing"

	"github.com/dop251/goja"
)

func BenchmarkResponseBytesValue64KiB(b *testing.B) {
	rt := goja.New()
	chunk := make([]byte, 64*1024)
	b.SetBytes(int64(len(chunk)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		value := bytesValue(rt, chunk)
		runtime.KeepAlive(value)
	}
}

func BenchmarkStreamingBodyPump1MiB(b *testing.B) {
	payload := make([]byte, 1024*1024)
	rt := goja.New()
	scheduler := immediateScheduler{rt: rt}
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		done := make(chan struct{})
		body := newStreamingBody(
			scheduler,
			io.NopCloser(bytes.NewReader(payload)),
			func() { close(done) },
			nil,
		)
		body.highWater = len(payload)/(64*1024) + 1
		body.start()
		<-done
	}
}
