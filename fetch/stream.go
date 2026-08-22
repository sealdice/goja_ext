package fetch

import (
	"io"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/runtimehost"
	"github.com/dop251/goja_nodejs/streams"
)

// fetchReadableStream builds a canonical ReadableStream that streams the given
// HTTP body. Reads run one per pull on background goroutines, so the loop
// thread never blocks and backpressure follows the stream contract exactly.
func fetchReadableStream(
	rt *goja.Runtime,
	scheduler runtimehost.Scheduler,
	body io.ReadCloser,
	cleanup func(),
	cleanupOffLoop func(),
	abort *dispatchAbortState,
) (*goja.Object, error) {
	reader, err := streams.NewReadableStreamFromReader(
		rt,
		scheduler,
		body,
		streams.WithMapError(func(rt *goja.Runtime, err error) goja.Value {
			return fetchNetworkError(rt, rt.NewGoError(err))
		}),
		streams.WithOnSettled(func(_ *goja.Runtime, err error) { println("DEBUG onSettled errNil:", err == nil); cleanup() }),
		streams.WithOnSettledOffLoop(func(error) { cleanupOffLoop() }),
	)
	if err != nil {
		return nil, err
	}
	if abort != nil {
		abort.setHandler(func(reason goja.Value) {
			reader.Error(rt, reason)
		})
	}
	return reader.Stream(), nil
}

func fetchNetworkError(rt *goja.Runtime, cause goja.Value) goja.Value {
	fetchError := Exports(rt).Get("_FetchError").ToObject(rt)
	networkError, ok := goja.AssertFunction(fetchError.Get("NETWORK_ERROR"))
	if !ok {
		return cause
	}
	value, err := networkError(fetchError, rt.ToValue("Network error"), cause)
	if err != nil {
		return cause
	}
	return value
}
