package cloudflarekv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/cloudflarekv/store"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/streams"
)

type maximumReader struct {
	reader    io.Reader
	remaining int64
	maximum   int64
}

func (reader *maximumReader) Read(buffer []byte) (int, error) {
	if reader.maximum <= 0 {
		return reader.reader.Read(buffer)
	}
	if reader.remaining == 0 {
		var probe [1]byte
		n, err := reader.reader.Read(probe[:])
		if n > 0 {
			return 0, errors.New(reader.errorMessage())
		}
		return 0, err
	}
	if int64(len(buffer)) > reader.remaining {
		buffer = buffer[:reader.remaining]
	}
	n, err := reader.reader.Read(buffer)
	reader.remaining -= int64(n)
	return n, err
}

func (reader *maximumReader) errorMessage() string {
	return fmt.Sprintf("KV value exceeds the maximum size of %d bytes", reader.maximum)
}

type maximumReadCloser struct {
	*maximumReader
	closer io.Closer
}

func (reader *maximumReadCloser) Close() error { return reader.closer.Close() }

type storageGetResult struct {
	record       store.Record
	streamRecord *store.StreamRecord
	found        bool
}

func getStorageValue(
	ctx context.Context,
	ns store.NamespaceStore,
	key string,
	wantStream bool,
) (storageGetResult, error) {
	if wantStream {
		if getter, ok := ns.(store.StreamGetter); ok {
			record, found, err := getter.GetStream(ctx, key)
			if err != nil {
				return storageGetResult{}, err
			}
			return storageGetResult{streamRecord: &record, found: found}, nil
		}
	}
	record, found, err := ns.Get(ctx, key)
	return storageGetResult{record: record, found: found}, err
}

func getStorageValueCached(
	ctx context.Context,
	state *bindingState,
	key string,
	wantStream bool,
	cacheTTL time.Duration,
) (storageGetResult, error) {
	if record, found, cached := state.cache.get(key); cached {
		return storageGetResult{record: record, found: found}, nil
	}
	result, err := getStorageValue(ctx, state.ns, key, wantStream)
	if err != nil {
		return storageGetResult{}, err
	}
	if !result.found {
		state.cache.put(key, store.Record{Key: key}, false, cacheTTL)
		return result, nil
	}
	if result.streamRecord == nil {
		state.cache.put(key, result.record, true, cacheTTL)
		return result, nil
	}
	if state.cache == nil || cacheTTL <= 0 {
		return result, nil
	}
	record := result.streamRecord
	cacheable := record.Size < 0 || record.Size+int64(len(key)+len(record.Metadata)) <= state.cache.capacity
	record.Body = &cachingReadCloser{
		body: record.Body, cache: state.cache, key: key,
		metadata: append([]byte(nil), record.Metadata...), expiration: record.Expiration,
		ttl: cacheTTL, maximum: state.cache.capacity, cacheable: cacheable,
	}
	return result, nil
}

func (r storageGetResult) metadata() []byte {
	if r.streamRecord != nil {
		return r.streamRecord.Metadata
	}
	return r.record.Metadata
}

func streamRecordToReadableStream(
	vm *goja.Runtime,
	loop *eventloop.EventLoop,
	record store.StreamRecord,
	maximumBytes int64,
) (goja.Value, error) {
	if record.Body == nil {
		return nil, errors.New("cloudflarekv: stream record body is nil")
	}
	if maximumBytes > 0 && record.Size >= 0 && record.Size > maximumBytes {
		_ = record.Body.Close()
		return nil, fmt.Errorf("KV value exceeds the maximum size of %d bytes", maximumBytes)
	}
	body := record.Body
	if maximumBytes > 0 {
		body = &maximumReadCloser{
			maximumReader: &maximumReader{reader: record.Body, remaining: maximumBytes, maximum: maximumBytes},
			closer:        record.Body,
		}
	}
	stream, err := streams.NewReadableStreamFromReader(
		vm,
		loop,
		body,
		streams.WithChunkSize(readableStreamChunkSize),
		streams.WithChunkValue(streams.ArrayBufferChunk),
		streams.WithMapError(func(rt *goja.Runtime, err error) goja.Value {
			return rt.ToValue(err.Error())
		}),
	)
	if err != nil {
		return nil, err
	}
	return stream.Stream(), nil
}

func putReadableStreamWithCapability(
	vm *goja.Runtime,
	loop *eventloop.EventLoop,
	putter store.StreamPutter,
	key string,
	streamValue goja.Value,
	options store.PutOptions,
	maximumBytes int64,
	onSuccess func(),
) goja.Value {
	promise, resolve, reject := vm.NewPromise()
	ctx, cancel := context.WithCancel(context.Background())
	reader, writer := io.Pipe()
	backendResult := make(chan error, 1)

	go func() {
		defer reader.Close()
		body := io.Reader(reader)
		if maximumBytes > 0 {
			body = &maximumReader{reader: reader, remaining: maximumBytes, maximum: maximumBytes}
		}
		backendResult <- putter.PutStream(ctx, key, body, options)
	}()

	consumed, err := streams.ConsumeReadableStream(vm, streamValue, func(chunk goja.Value) goja.Value {
		data, chunkErr := streamChunkBytes(vm, chunk)
		if chunkErr != nil {
			panic(vm.ToValue(chunkErr.Error()))
		}
		written, writeResolve, writeReject := vm.NewPromise()
		go func() {
			_, writeErr := writer.Write(data)
			if !loop.RunOnLoop(func(loopVM *goja.Runtime) {
				if writeErr != nil {
					_ = writeReject(jsErrorValue(loopVM, writeErr))
				} else {
					_ = writeResolve(goja.Undefined())
				}
			}) {
				cancel()
				_ = writer.CloseWithError(io.ErrClosedPipe)
			}
		}()
		return vm.ToValue(written)
	})
	if err != nil {
		cancel()
		_ = writer.CloseWithError(err)
		_ = reject(jsErrorValue(vm, err))
		return vm.ToValue(promise)
	}

	finish := func(reason goja.Value, consumeErr error) {
		if consumeErr != nil {
			cancel()
			_ = writer.CloseWithError(consumeErr)
		} else {
			_ = writer.Close()
		}
		go func() {
			backendErr := <-backendResult
			cancel()
			if consumeErr == nil && backendErr == nil && onSuccess != nil {
				onSuccess()
			}
			loop.RunOnLoop(func(loopVM *goja.Runtime) {
				if consumeErr != nil {
					_ = reject(valueOrUndefined(reason))
				} else if backendErr != nil {
					_ = reject(jsErrorValue(loopVM, backendErr))
				} else {
					_ = resolve(goja.Undefined())
				}
			})
		}()
	}

	thenValue(vm, vm.ToValue(consumed),
		func(goja.FunctionCall) goja.Value {
			finish(goja.Undefined(), nil)
			return goja.Undefined()
		},
		func(call goja.FunctionCall) goja.Value {
			reason := valueOrUndefined(call.Argument(0))
			finish(reason, errors.New(reason.String()))
			return goja.Undefined()
		},
	)
	return vm.ToValue(promise)
}
