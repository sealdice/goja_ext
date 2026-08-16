package main

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/fs"
	_ "github.com/dop251/goja_nodejs/path"
	"github.com/dop251/goja_nodejs/process"
	"github.com/dop251/goja_nodejs/require"
	"github.com/spf13/afero"
)

func main() {
	backend := afero.NewMemMapFs()
	registry := require.NewRegistry()
	loop := eventloop.NewEventLoop(
		eventloop.EnableConsole(false),
		eventloop.WithRegistry(registry),
	)
	options := []fs.Option{
		fs.WithFS(backend),
		fs.WithCwd("/workspace"),
		fs.WithStreams(false),
	}
	fs.RegisterWithLoop(registry, loop, options...)

	loop.Run(func(rt *goja.Runtime) {
		if err := fs.EnableWithLoop(rt, loop, options...); err != nil {
			panic(err)
		}
		process.Enable(rt)

		value, err := rt.RunString(`
			const nodeFS = require("fs");
			const path = require("path");
			fs.mkdirSync("nested", { recursive: true });
			nodeFS.writeFileSync("nested/message.txt", "shared cwd");
			process.chdir("nested");
			JSON.stringify({
				cwd: [fs.cwd(), process.cwd()],
				path: path.resolve("message.txt"),
				text: fs.readTextFileSync("message.txt")
			});
		`)
		if err != nil {
			panic(err)
		}
		fmt.Println(value.String()) //nolint:forbidigo // Example output.
	})
}
