package fetch

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dop251/goja"
)

func TestEventSource_MessageEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		if flusher != nil {
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
		_, _ = fmt.Fprint(w, "data: hello\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = fmt.Fprint(w, "event: custom\ndata: custom-data\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		Enable(vm)
		if err := EnableFetch(vm, loop); err != nil {
			t.Fatalf("EnableFetch: %v", err)
		}
		_, err := vm.RunString(`
			globalThis.__done = false; globalThis.__err = "";
			const es = new EventSource("` + srv.URL + `");
			const events = [];
			es.onopen = function () { events.push("open"); };
			es.onmessage = function (e) { events.push("msg:" + e.data); };
			es.addEventListener("custom", function (e) { events.push("custom:" + e.data); });
			es.onerror = function () { events.push("error"); };
			setTimeout(function () {
				es.close();
				globalThis.__result = events.join(",");
				globalThis.__state = String(es.readyState);
				globalThis.__done = true;
			}, 200);
		`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	waitBool(t, loop, "__done")
	if s := gstr(t, loop, "__state"); s != "2" {
		t.Fatalf("readyState after close = %s", s)
	}
	if s := gstr(t, loop, "__result"); s != "open,msg:hello,custom:custom-data" {
		t.Fatalf("events = %s", s)
	}
}

func TestEventSource_LastEventIDHeader(t *testing.T) {
	var gotLastEventID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLastEventID = r.Header.Get("Last-Event-ID")
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "retry: 100\nid: 42\ndata: first\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		Enable(vm)
		_ = EnableFetch(vm, loop)
		_, err := vm.RunString(`
			globalThis.__done = true;
			const es = new EventSource("` + srv.URL + `");
			globalThis.__es = es;
		`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if gotLastEventID == "42" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if gotLastEventID != "42" {
		t.Fatalf("expected reconnect to send Last-Event-ID=42, got %q", gotLastEventID)
	}
}
