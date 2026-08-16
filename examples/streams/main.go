package main

import (
	"fmt"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/streams"
)

func main() {
	loop := eventloop.NewEventLoop(eventloop.EnableConsole(false))
	loop.Start()
	defer loop.Stop()

	result := make(chan string, 1)
	setup := make(chan error, 1)
	if !loop.RunOnLoop(func(rt *goja.Runtime) {
		streams.Enable(rt)
		if err := rt.Set("__finish", func(value string) { result <- value }); err != nil {
			setup <- err
			return
		}
		_, err := rt.RunString(`
			const { ReadableStream, TextEncoder, TextDecoderStream } = require("streams");
			const encoder = new TextEncoder();
			const source = new ReadableStream({
				start(controller) {
					controller.enqueue(encoder.encode("hello"));
					controller.enqueue(encoder.encode(" "));
					controller.enqueue(encoder.encode("streams"));
					controller.close();
				}
			});
			const decoded = source.pipeThrough(new TextDecoderStream());
			const reader = decoded.getReader();
			let text = "";
			function pump(result) {
				if (result.done) {
					__finish(text);
					return;
				}
				text += result.value;
				reader.read().then(pump, (error) => __finish("error: " + error));
			}
			reader.read().then(pump, (error) => __finish("error: " + error));
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
		if value != "hello streams" {
			panic("unexpected streams result: " + value)
		}
		fmt.Println(value) //nolint:forbidigo // Example output.
	case <-time.After(3 * time.Second):
		panic("timed out waiting for stream consumption")
	}
}
