package main

import (
	"fmt"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/streams"
	_ "github.com/dop251/goja_nodejs/streams/node"
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
			const { Readable } = require("stream");
			let emitted = false;
			const classic = new Readable({
				read() {
					if (emitted) return;
					emitted = true;
					this.push("hello");
					this.push(" ");
					this.push("streams");
					this.push(null);
				}
			});
			const decoded = Readable.toWeb(classic).pipeThrough(new TextDecoderStream());
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
