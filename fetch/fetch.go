package fetch

import (
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/runtimehost"
)

// EnableFetch registers the global fetch(input, init) function.
// Fetch API behavior is provided by the embedded bare-fetch facade; network I/O
// is delegated to the Go dispatcher configured by opts.
func EnableFetch(rt *goja.Runtime, loop *eventloop.EventLoop, opts ...FetchOption) error {
	if loop == nil {
		return errors.New("JS event loop is required for fetch")
	}
	if err := runtimehost.ValidateScheduler(rt, loop); err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	if err := runtimehost.BindScheduler(rt, loop); err != nil {
		return fmt.Errorf("fetch: %w", err)
	}

	client := newClient(opts...)
	host := rt.NewObject()
	if err := host.Set("dispatch", newDispatchFn(rt, loop, client)); err != nil {
		return err
	}
	createFetch, ok := goja.AssertFunction(Exports(rt).Get("_createFetch"))
	if !ok {
		return errors.New("fetch: embedded facade is missing _createFetch")
	}
	fetchValue, err := createFetch(goja.Undefined(), host)
	if err != nil {
		return fmt.Errorf("fetch: initialize global fetch: %w", err)
	}
	if err := rt.Set("fetch", fetchValue); err != nil {
		return err
	}
	return nil
}
