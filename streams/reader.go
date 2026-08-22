package streams

import (
	"errors"
	"io"
	"sync"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/runtimehost"
)

// DefaultChunkSize is the number of bytes read per chunk when WithChunkSize
// is not supplied.
const DefaultChunkSize = 64 * 1024

// ReaderStreamOption configures the byte-stream constructors in this file.
type ReaderStreamOption func(*readerConfig)

type readerConfig struct {
	chunkSize     int
	highWaterMark int
	chunkValue    func(rt *goja.Runtime, data []byte) goja.Value
	mapError      func(rt *goja.Runtime, err error) goja.Value
	onCancel      func(rt *goja.Runtime, reason goja.Value) goja.Value
	onSettled     func(rt *goja.Runtime, err error)
	onSettledOff  func(err error)
}

func newReaderConfig() readerConfig {
	return readerConfig{
		chunkSize:  DefaultChunkSize,
		chunkValue: Uint8ArrayChunk,
		mapError: func(rt *goja.Runtime, err error) goja.Value {
			return errorValue(rt, err)
		},
	}
}

func (config *readerConfig) apply(opts []ReaderStreamOption) {
	for _, opt := range opts {
		opt(config)
	}
	if config.chunkSize <= 0 {
		config.chunkSize = DefaultChunkSize
	}
}

// WithChunkSize sets the maximum number of bytes delivered per chunk.
func WithChunkSize(size int) ReaderStreamOption {
	return func(config *readerConfig) { config.chunkSize = size }
}

// WithHighWaterMark sets a count-based high water mark on the stream.
func WithHighWaterMark(mark int) ReaderStreamOption {
	return func(config *readerConfig) { config.highWaterMark = mark }
}

// WithChunkValue overrides how a chunk is exposed to JavaScript. The default
// wraps each chunk in a Uint8Array; the callback receives ownership of data.
func WithChunkValue(fn func(rt *goja.Runtime, data []byte) goja.Value) ReaderStreamOption {
	return func(config *readerConfig) { config.chunkValue = fn }
}

// WithMapError overrides how a failed read becomes the stream error reason.
func WithMapError(fn func(rt *goja.Runtime, err error) goja.Value) ReaderStreamOption {
	return func(config *readerConfig) { config.mapError = fn }
}

// WithOnCancel customizes stream cancellation. When set, the hook owns
// closing the reader on the cancel path (the bridge does not close it), and
// its return value becomes the cancel result.
func WithOnCancel(fn func(rt *goja.Runtime, reason goja.Value) goja.Value) ReaderStreamOption {
	return func(config *readerConfig) { config.onCancel = fn }
}

// WithOnSettled registers a callback invoked exactly once on the loop when
// the stream settles: end of input (err is io.EOF), read failure (err set),
// cancellation, or Error (err nil). It runs before the final controller
// transition so host cleanup is not observable as lagging behind the stream.
func WithOnSettled(fn func(rt *goja.Runtime, err error)) ReaderStreamOption {
	return func(config *readerConfig) { config.onSettled = fn }
}

// WithOnSettledOffLoop registers the fallback invoked instead of OnSettled
// when the scheduler can no longer deliver loop callbacks.
func WithOnSettledOffLoop(fn func(err error)) ReaderStreamOption {
	return func(config *readerConfig) { config.onSettledOff = fn }
}

// Uint8ArrayChunk wraps data in a Uint8Array backed by a new ArrayBuffer. It
// takes ownership of data.
func Uint8ArrayChunk(rt *goja.Runtime, data []byte) goja.Value {
	arrayBuffer := rt.NewArrayBuffer(data)
	typed, err := rt.New(rt.Get("Uint8Array"), rt.ToValue(arrayBuffer))
	if err != nil {
		panic(err)
	}
	return typed
}

// ArrayBufferChunk wraps data in an ArrayBuffer. It takes ownership of data.
func ArrayBufferChunk(rt *goja.Runtime, data []byte) goja.Value {
	return rt.ToValue(rt.NewArrayBuffer(data))
}

// ReaderStream is a ReadableStream that streams an io.ReadCloser. Each pull
// performs one read on a background goroutine (inline when the scheduler is
// nil) and delivers the result on the scheduler's loop.
type ReaderStream struct {
	rt     *goja.Runtime
	sched  runtimehost.Scheduler
	reader io.ReadCloser
	config readerConfig
	stream *goja.Object

	mu         sync.Mutex
	settled    bool
	settle     func(interface{}) error
	controller *goja.Object
	closeOnce  sync.Once
}

// Stream returns the ReadableStream object.
func (s *ReaderStream) Stream() *goja.Object { return s.stream }

// closeReader closes the reader exactly once across all terminal paths.
func (s *ReaderStream) closeReader() {
	s.closeOnce.Do(func() { _ = s.reader.Close() })
}

// closeReaderForRead closes the reader once and reports the close error so
// an end-of-input close failure can replace EOF.
func (s *ReaderStream) closeReaderForRead() error {
	var closeErr error
	s.closeOnce.Do(func() { closeErr = s.reader.Close() })
	return closeErr
}

// Error terminates the stream with reason, closing the reader and running
// the settle hooks. It must be called on the loop goroutine.
func (s *ReaderStream) Error(rt *goja.Runtime, reason goja.Value) {
	s.mu.Lock()
	if s.settled {
		s.mu.Unlock()
		return
	}
	s.settled = true
	settle := s.settle
	s.settle = nil
	controller := s.controller
	s.mu.Unlock()

	if settle != nil {
		_ = settle(goja.Undefined())
	}
	s.closeReader()
	if s.config.onSettled != nil {
		s.config.onSettled(rt, nil)
	}
	if controller != nil {
		callStreamController(rt, controller, "error", valueOrUndefined(reason))
	}
}

// NewReadableStreamFromReader returns a ReadableStream that delivers reader
// in chunks of WithChunkSize bytes. The bridge closes reader exactly once:
// on end of input, on Error, on construction failure, and on cancellation
// unless WithOnCancel takes over that path.
func NewReadableStreamFromReader(
	rt *goja.Runtime,
	scheduler runtimehost.Scheduler,
	reader io.ReadCloser,
	opts ...ReaderStreamOption,
) (*ReaderStream, error) {
	if reader == nil {
		return nil, errors.New("streams: reader is required")
	}
	config := newReaderConfig()
	config.apply(opts)

	source := &ReaderStream{rt: rt, sched: scheduler, reader: reader, config: config}
	var strategy goja.Value
	if config.highWaterMark > 0 {
		strategyObject := rt.NewObject()
		_ = strategyObject.Set("highWaterMark", config.highWaterMark)
		_ = strategyObject.Set("size", rt.ToValue(func(goja.FunctionCall) goja.Value {
			return rt.ToValue(1)
		}))
		strategy = strategyObject
	}
	stream, err := NewReadableStream(rt, ReadableStreamSource{
		Start: func(controller *goja.Object) goja.Value {
			source.mu.Lock()
			source.controller = controller
			source.mu.Unlock()
			return goja.Undefined()
		},
		Pull:   func(*goja.Object) goja.Value { return source.pull() },
		Cancel: func(reason goja.Value) goja.Value { return source.cancel(reason) },
	}, strategy)
	if err != nil {
		source.closeReader()
		if config.onSettled != nil {
			config.onSettled(rt, nil)
		}
		return nil, err
	}
	source.stream = stream
	return source, nil
}

func (s *ReaderStream) pull() goja.Value {
	promise, resolve, _ := s.rt.NewPromise()
	s.mu.Lock()
	if s.settled {
		s.mu.Unlock()
		_ = resolve(goja.Undefined())
		return s.rt.ToValue(promise)
	}
	s.settle = resolve
	s.mu.Unlock()

	if s.sched == nil {
		s.readOnce()
	} else {
		go s.readOnce()
	}
	return s.rt.ToValue(promise)
}

func (s *ReaderStream) cancel(reason goja.Value) goja.Value {
	s.mu.Lock()
	if s.settled {
		s.mu.Unlock()
		return goja.Undefined()
	}
	s.settled = true
	settle := s.settle
	s.settle = nil
	s.mu.Unlock()

	if settle != nil {
		_ = settle(goja.Undefined())
	}
	if s.config.onCancel != nil {
		if result := s.config.onCancel(s.rt, reason); result != nil {
			if s.config.onSettled != nil {
				s.config.onSettled(s.rt, nil)
			}
			return result
		}
	} else {
		s.closeReader()
	}
	if s.config.onSettled != nil {
		s.config.onSettled(s.rt, nil)
	}
	return goja.Undefined()
}

func (s *ReaderStream) readOnce() {
	buffer := make([]byte, s.config.chunkSize)
	n, readErr := s.reader.Read(buffer)
	if readErr != nil {
		// Terminal read: close off the loop. A close failure at end of input
		// replaces EOF so it surfaces to the consumer.
		if closeErr := s.closeReaderForRead(); closeErr != nil && errors.Is(readErr, io.EOF) {
			readErr = closeErr
		}
	}
	if s.sched == nil {
		s.deliver(buffer, n, readErr)
		return
	}
	if s.sched.RunOnLoop(func(*goja.Runtime) {
		s.deliver(buffer, n, readErr)
	}) {
		return
	}
	s.abandon(readErr)
}

// deliver runs on the loop (or inline for nil schedulers) with one result.
func (s *ReaderStream) deliver(buffer []byte, n int, readErr error) {
	s.mu.Lock()
	if s.settled {
		s.mu.Unlock()
		return
	}
	terminal := readErr != nil
	if terminal {
		s.settled = true
	}
	settle := s.settle
	s.settle = nil
	controller := s.controller
	s.mu.Unlock()

	if controller == nil {
		if settle != nil {
			_ = settle(goja.Undefined())
		}
		return
	}
	rt := s.rt
	if terminal && s.config.onSettled != nil {
		s.config.onSettled(rt, readErr)
	}
	if n > 0 {
		chunk := make([]byte, n)
		copy(chunk, buffer[:n])
		callStreamController(rt, controller, "enqueue", s.config.chunkValue(rt, chunk))
	}
	if terminal {
		if errors.Is(readErr, io.EOF) {
			callStreamController(rt, controller, "close")
		} else {
			callStreamController(rt, controller, "error", s.config.mapError(rt, readErr))
		}
	}
	if settle != nil {
		_ = settle(goja.Undefined())
	}
}

// abandon runs when the loop is gone: mark settled, close the reader, and
// run the off-loop settle hook.
func (s *ReaderStream) abandon(readErr error) {
	s.mu.Lock()
	if s.settled {
		s.mu.Unlock()
		return
	}
	s.settled = true
	s.settle = nil
	s.mu.Unlock()

	s.closeReader()
	if s.config.onSettledOff != nil {
		s.config.onSettledOff(readErr)
	}
}

func callStreamController(rt *goja.Runtime, controller *goja.Object, name string, args ...goja.Value) {
	method, ok := goja.AssertFunction(controller.Get(name))
	if !ok {
		panic(rt.NewTypeError("ReadableStream controller method %s is not callable", name))
	}
	if _, err := method(controller, args...); err != nil {
		panic(err)
	}
}

// NewReadableStreamFromBytes returns a ReadableStream that delivers data in
// fixed-size chunks. Pulls are satisfied synchronously, so no scheduler is
// required; data is copied per chunk.
func NewReadableStreamFromBytes(
	rt *goja.Runtime,
	data []byte,
	chunkSize int,
	opts ...ReaderStreamOption,
) (*goja.Object, error) {
	config := newReaderConfig()
	config.chunkSize = chunkSize
	config.apply(opts)

	offset := 0
	return NewReadableStream(rt, ReadableStreamSource{
		Pull: func(controller *goja.Object) goja.Value {
			if offset >= len(data) {
				callStreamController(rt, controller, "close")
				return goja.Undefined()
			}
			end := min(offset+config.chunkSize, len(data))
			chunk := append([]byte(nil), data[offset:end]...)
			offset = end
			callStreamController(rt, controller, "enqueue", config.chunkValue(rt, chunk))
			return goja.Undefined()
		},
		Cancel: func(goja.Value) goja.Value {
			offset = len(data)
			return goja.Undefined()
		},
	})
}
