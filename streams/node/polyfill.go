package node

import (
	_ "embed"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/streams/streamx"
)

var polyfillProgram = mustCompilePolyfill()

func mustCompilePolyfill() *goja.Program {
	source := `(function (require, queueMicrotask) {
		var module = { exports: {} };
		var exports = module.exports;
` + streamx.Source + `
		return module.exports;
})`
	program, err := goja.Compile("streamx@2.28.0/bundle.js", source, false)
	if err != nil {
		panic(fmt.Errorf("node streams: compile polyfill: %w", err))
	}
	return program
}
