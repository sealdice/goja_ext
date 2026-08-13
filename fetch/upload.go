package fetch

import (
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/runtimehost"
	"github.com/sealdice/goja_ext/streams"
)

type uploadItem struct {
	data []byte
	err  error
	eof  bool
}

type uploadBody struct {
	scheduler runtimehost.Scheduler
	reader    *goja.Object
	items     chan uploadItem
	aborted   chan struct{}
	abortOnce sync.Once
	release   sync.Once

	mu       sync.Mutex
	current  *uploadItem
	finished bool
}

func newUploadBody(
	rt *goja.Runtime,
	scheduler runtimehost.Scheduler,
	stream goja.Value,
) (*uploadBody, error) {
	if !streams.IsReadableStream(rt, stream) {
		return nil, errors.New("fetch: request bodyStream must be a ReadableStream")
	}
	readerValue, err := callUploadMethod(stream.ToObject(rt), "getReader")
	if err != nil {
		return nil, fmt.Errorf("fetch: acquire request body reader: %w", err)
	}
	body := &uploadBody{
		scheduler: scheduler,
		reader:    readerValue.ToObject(rt),
		items:     make(chan uploadItem, 1),
		aborted:   make(chan struct{}),
	}
	body.readNextOnLoop(rt)
	return body, nil
}

func uploadChunkBytes(rt *goja.Runtime, value goja.Value) (data []byte, err error) {
	if exception := rt.Try(func() {
		if object, ok := value.(*goja.Object); ok {
			if bytes, ok := bytesFromArrayBufferView(object); ok {
				data = bytes
				return
			}
		}
		if arrayBuffer, ok := value.Export().(goja.ArrayBuffer); ok {
			data = append([]byte(nil), arrayBuffer.Bytes()...)
			return
		}
		err = errors.New("fetch: request body stream chunk must be a byte buffer")
	}); exception != nil {
		return nil, errors.New("fetch: request body stream chunk byte conversion failed")
	}
	return data, err
}

func (b *uploadBody) readNextOnLoop(rt *goja.Runtime) {
	if b.isFinished() {
		return
	}
	readResult, err := callUploadMethod(b.reader, "read")
	if err != nil {
		b.finishOnLoop(uploadItem{err: fmt.Errorf("request body stream: %w", err)})
		return
	}
	if err := observeUploadPromise(rt, readResult,
		func(value goja.Value) {
			if b.isFinished() {
				return
			}
			item := value.ToObject(rt)
			if item.Get("done").ToBoolean() {
				b.finishOnLoop(uploadItem{eof: true})
				return
			}
			chunk, err := uploadChunkBytes(rt, item.Get("value"))
			if err != nil {
				b.failChunkOnLoop(rt, err)
				return
			}
			b.enqueue(uploadItem{data: chunk})
		},
		func(reason goja.Value) {
			if b.isFinished() {
				return
			}
			b.finishOnLoop(uploadItem{
				err: fmt.Errorf("request body stream: %s", uploadReasonText(rt, reason)),
			})
		},
	); err != nil {
		b.finishOnLoop(uploadItem{err: err})
	}
}

func uploadReasonText(rt *goja.Runtime, reason goja.Value) string {
	text := "unprintable JavaScript rejection reason"
	_ = rt.Try(func() { text = reason.String() })
	return text
}

func observeUploadPromise(
	rt *goja.Runtime,
	promise goja.Value,
	onFulfilled func(goja.Value),
	onRejected func(goja.Value),
) error {
	object := promise.ToObject(rt)
	then, ok := goja.AssertFunction(object.Get("then"))
	if !ok {
		return errors.New("fetch: request body read did not return a Promise")
	}
	_, err := then(object,
		rt.ToValue(func(call goja.FunctionCall) goja.Value {
			onFulfilled(call.Argument(0))
			return goja.Undefined()
		}),
		rt.ToValue(func(call goja.FunctionCall) goja.Value {
			onRejected(call.Argument(0))
			return goja.Undefined()
		}),
	)
	return err
}

func callUploadMethod(object *goja.Object, name string, args ...goja.Value) (goja.Value, error) {
	method, ok := goja.AssertFunction(object.Get(name))
	if !ok {
		return nil, fmt.Errorf("%s is not callable", name)
	}
	return method(object, args...)
}

func (b *uploadBody) enqueue(item uploadItem) {
	select {
	case b.items <- item:
	case <-b.aborted:
	}
}

func (b *uploadBody) finishOnLoop(item uploadItem) {
	b.mu.Lock()
	if b.finished {
		b.mu.Unlock()
		return
	}
	b.finished = true
	b.mu.Unlock()
	b.releaseReaderOnLoop()
	b.enqueue(item)
}

func (b *uploadBody) failChunkOnLoop(rt *goja.Runtime, err error) {
	b.mu.Lock()
	if b.finished {
		b.mu.Unlock()
		return
	}
	b.finished = true
	b.mu.Unlock()
	b.enqueue(uploadItem{err: err})
	b.cancelReaderOnLoop(rt.NewGoError(err))
}

func (b *uploadBody) isFinished() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.finished
}

func (b *uploadBody) Read(target []byte) (int, error) {
	if len(target) == 0 {
		return 0, nil
	}
	for {
		b.mu.Lock()
		current := b.current
		b.mu.Unlock()
		if current == nil {
			select {
			case item := <-b.items:
				current = &item
				b.mu.Lock()
				b.current = current
				b.mu.Unlock()
			case <-b.aborted:
				return 0, io.ErrClosedPipe
			}
		}

		if current.err != nil {
			return 0, current.err
		}
		if current.eof {
			return 0, io.EOF
		}
		if len(current.data) == 0 {
			b.clearCurrentAndReadNext()
			continue
		}

		n := copy(target, current.data)
		current.data = current.data[n:]
		if len(current.data) == 0 {
			b.clearCurrentAndReadNext()
		}
		return n, nil
	}
}

func (b *uploadBody) clearCurrentAndReadNext() {
	b.mu.Lock()
	b.current = nil
	finished := b.finished
	b.mu.Unlock()
	if !finished {
		if b.scheduler.RunOnLoop(func(rt *goja.Runtime) {
			b.readNextOnLoop(rt)
		}) {
			return
		}
		b.terminateOffLoop()
	}
}

func (b *uploadBody) terminateOffLoop() {
	b.abortOnce.Do(func() {
		b.mu.Lock()
		b.finished = true
		b.mu.Unlock()
		close(b.aborted)
	})
}

func (b *uploadBody) abortOnLoop(reason goja.Value) {
	b.abortOnce.Do(func() {
		b.mu.Lock()
		if b.finished {
			b.mu.Unlock()
			return
		}
		b.finished = true
		b.mu.Unlock()
		close(b.aborted)
		b.cancelReaderOnLoop(valueOrUndefined(reason))
	})
}

func (b *uploadBody) cancelReaderOnLoop(reason goja.Value) {
	result, err := callUploadMethod(b.reader, "cancel", reason)
	if err != nil {
		b.releaseReaderOnLoop()
		return
	}
	rt := b.scheduler.Runtime()
	if err := observeUploadPromise(rt, result,
		func(goja.Value) { b.releaseReaderOnLoop() },
		func(goja.Value) { b.releaseReaderOnLoop() },
	); err != nil {
		b.releaseReaderOnLoop()
	}
}

func (b *uploadBody) releaseReaderOnLoop() {
	b.release.Do(func() {
		_, _ = callUploadMethod(b.reader, "releaseLock")
	})
}

func (b *uploadBody) Close() error {
	if b.isFinished() {
		return nil
	}
	b.abortOnce.Do(func() {
		b.mu.Lock()
		b.finished = true
		b.mu.Unlock()
		close(b.aborted)
		b.scheduler.RunOnLoop(func(rt *goja.Runtime) {
			b.cancelReaderOnLoop(rt.NewGoError(errors.New("fetch: request body is closed")))
		})
	})
	return nil
}
