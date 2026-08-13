package main

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/abort"
	"github.com/sealdice/goja_ext/eventloop"
	"github.com/sealdice/goja_ext/fetch"
	"github.com/sealdice/goja_ext/process"
	"github.com/sealdice/goja_ext/require"
	"github.com/sealdice/goja_ext/streams"
)

func main() {
	registry := require.NewRegistry()
	loop := eventloop.NewEventLoop(
		eventloop.EnableConsole(false),
		eventloop.WithRegistry(registry),
	)

	loop.Run(func(rt *goja.Runtime) {
		abort.Enable(rt)
		fetch.Enable(rt)
		process.Enable(rt)
		streams.Enable(rt)

		value, err := rt.RunString(`
			const abort = require("abort");
			const fetchAPI = require("fetch");
			const webStreams = require("stream/web");
			JSON.stringify({
				abort: AbortController === abort.AbortController,
				fetch: Response === fetchAPI.Response,
				process: process === require("process"),
				streams: ReadableStream === webStreams.ReadableStream
			});
		`)
		if err != nil {
			panic(err)
		}
		fmt.Println(value.String()) //nolint:forbidigo // Example output.
	})
}
