package node

import (
	_ "embed"
	"fmt"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/streams/internal/streamx"
)

var polyfillProgram = mustCompilePolyfill()

func mustCompilePolyfill() *goja.Program {
	source := `(function (require) {
		var module = { exports: {} };
		var exports = module.exports;
` + streamx.Source + `
		return module.exports;
	})(function (name) {
		if (name === "events") return globalThis.__goja_ext_canonical_events;
		throw new Error("node streams: unresolved require: " + name);
	});`
	program, err := goja.Compile("streamx@2.28.0/bundle.js", source, false)
	if err != nil {
		panic(fmt.Errorf("node streams: compile polyfill: %w", err))
	}
	return program
}
