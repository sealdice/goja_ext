package fetch

import (
	_ "embed"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/abort"
	"github.com/dop251/goja_nodejs/require"
	"github.com/dop251/goja_nodejs/runtimehost"
	"github.com/dop251/goja_nodejs/streams"
)

const FetchEventSourceModuleName = "@microsoft/fetch-event-source"

//go:embed internal/fetch_event_source/bundle.js
var fetchEventSourceSource string

var (
	fetchEventSourceKey     = runtimehost.NewKey("fetch-event-source.exports")
	fetchEventSourceProgram = mustCompileFetchEventSource()
)

func mustCompileFetchEventSource() *goja.Program {
	source := `(function (__gojaAbortController, __gojaTextDecoder) {
		var __gojaDocument = {
			hidden: false,
			addEventListener: function () {},
			removeEventListener: function () {}
		};
		var __gojaWindow = {
			get fetch() { return globalThis.fetch; },
			setTimeout: function () { return globalThis.setTimeout.apply(globalThis, arguments); },
			clearTimeout: function () { return globalThis.clearTimeout.apply(globalThis, arguments); }
		};
		var module = { exports: {} };
		var exports = module.exports;
` + fetchEventSourceSource + `
		return module.exports;
	})`
	program, err := goja.Compile(FetchEventSourceModuleName+"@2.0.1/index.js", source, false)
	if err != nil {
		panic(fmt.Errorf("fetch: compile embedded fetch-event-source: %w", err))
	}
	return program
}

func fetchEventSourceExports(rt *goja.Runtime) *goja.Object {
	value := runtimehost.GetOrCreate(rt, fetchEventSourceKey, func() any {
		initializerValue, err := rt.RunProgram(fetchEventSourceProgram)
		if err != nil {
			panic(fmt.Errorf("fetch: load embedded fetch-event-source: %w", err))
		}
		initializer, ok := goja.AssertFunction(initializerValue)
		if !ok {
			panic("fetch: embedded fetch-event-source did not return an initializer")
		}
		result, err := initializer(
			goja.Undefined(),
			abort.Exports(rt).Get("AbortController"),
			streams.Exports(rt).Get("TextDecoder"),
		)
		if err != nil {
			panic(fmt.Errorf("fetch: initialize embedded fetch-event-source: %w", err))
		}
		exports, ok := result.(*goja.Object)
		if !ok {
			panic("fetch: embedded fetch-event-source did not return exports")
		}
		return exports
	})
	return value.(*goja.Object)
}

func requireFetchEventSource(rt *goja.Runtime, module *goja.Object) {
	if err := module.Set("exports", fetchEventSourceExports(rt)); err != nil {
		panic(err)
	}
}

func init() { //nolint:gochecknoinits // auto-register core module via blank import
	require.RegisterCoreModule(FetchEventSourceModuleName, requireFetchEventSource)
}
