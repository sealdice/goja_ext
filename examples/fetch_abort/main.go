package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/abort"
	"github.com/sealdice/goja_ext/eventloop"
	"github.com/sealdice/goja_ext/fetch"
)

func main() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		select {
		case <-request.Context().Done():
		case <-time.After(5 * time.Second):
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	loop := eventloop.NewEventLoop(eventloop.EnableConsole(false))
	loop.Start()
	defer loop.Stop()

	result := make(chan string, 1)
	setup := make(chan error, 1)
	if !loop.RunOnLoop(func(rt *goja.Runtime) {
		abort.Enable(rt)
		fetch.Enable(rt)
		if err := fetch.EnableFetch(rt, loop); err != nil {
			setup <- err
			return
		}
		if err := rt.Set("__url", server.URL); err != nil {
			setup <- err
			return
		}
		if err := rt.Set("__finish", func(value string) { result <- value }); err != nil {
			setup <- err
			return
		}
		_, err := rt.RunString(`
			const controller = new AbortController();
			fetch(__url, { signal: controller.signal })
				.then(() => __finish("unexpected response"))
				.catch((reason) => __finish(String(reason)));
			setTimeout(() => controller.abort("example abort"), 10);
		`)
		setup <- err
	}) {
		panic("event loop rejected setup")
	}
	if err := <-setup; err != nil {
		panic(err)
	}

	select {
	case value := <-result:
		if value != "example abort" {
			panic("unexpected fetch result: " + value)
		}
		fmt.Println(value)
	case <-time.After(3 * time.Second):
		panic("timed out waiting for fetch cancellation")
	}
}
