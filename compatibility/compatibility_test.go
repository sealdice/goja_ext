package compatibility_test

import (
	"testing"

	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/require"
	_ "github.com/dop251/goja_nodejs/streams"
)

func TestLegacyModulePathCanReplaceGojaNodeJS(t *testing.T) {
	loop := eventloop.NewEventLoop()
	if loop == nil {
		t.Fatal("event loop is nil")
	}

	if require.NewRegistry() == nil {
		t.Fatal("require registry is nil")
	}
}
