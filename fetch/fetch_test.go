package fetch

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/abort"
	"github.com/dop251/goja_nodejs/eventloop"
)

func startFetchLoop(t *testing.T) *eventloop.EventLoop {
	t.Helper()
	loop := eventloop.NewEventLoop(eventloop.EnableConsole(false))
	go loop.StartInForeground()
	t.Cleanup(func() { loop.Stop() })
	return loop
}

func runSync(t *testing.T, loop *eventloop.EventLoop, f func(*goja.Runtime)) {
	t.Helper()
	done := make(chan struct{})
	var rec any
	loop.RunOnLoop(func(vm *goja.Runtime) {
		defer close(done)
		defer func() { rec = recover() }()
		f(vm)
	})
	<-done
	if rec != nil {
		t.Fatalf("loop panic: %v", rec)
	}
}

func waitBool(t *testing.T, loop *eventloop.EventLoop, name string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var b bool
		runSync(t, loop, func(vm *goja.Runtime) {
			v := vm.Get(name)
			b = v != nil && !goja.IsUndefined(v) && v.ToBoolean()
		})
		if b {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", name)
}

func gstr(t *testing.T, loop *eventloop.EventLoop, name string) string {
	t.Helper()
	var s string
	runSync(t, loop, func(vm *goja.Runtime) {
		v := vm.Get(name)
		if v != nil && !goja.IsUndefined(v) {
			s = v.String()
		}
	})
	return s
}

func TestFetch_GetJSONTextArrayBuffer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"bin":[0,128,255]}`))
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
			(async () => {
				try {
					const r = await fetch("` + srv.URL + `?b=2&a=1");
					const text = await r.text();
					const json = await r.json();
					const ab = await r.arrayBuffer();
					const bytes = new Uint8Array(ab);
					let bt = ""; for (let i=0;i<bytes.length;i++){ if(i) bt+=","; bt+=String(bytes[i]); }
					globalThis.__status = String(r.status);
					globalThis.__ok = String(r.ok);
					globalThis.__ct = r.headers.get("content-type");
					globalThis.__json = String(json.ok);
					globalThis.__bytes = bt;
					globalThis.__hasBin = text.includes('"bin":[0,128,255]');
				} catch(e) { globalThis.__err = String(e && e.stack || e); }
				finally { globalThis.__done = true; }
			})();
		`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	waitBool(t, loop, "__done")
	if e := gstr(t, loop, "__err"); e != "" {
		t.Fatalf("err: %s", e)
	}
	if s := gstr(t, loop, "__status"); s != "200" {
		t.Fatalf("status %s", s)
	}
	if s := gstr(t, loop, "__ok"); s != "true" {
		t.Fatalf("ok %s", s)
	}
	if s := gstr(t, loop, "__ct"); s != "application/json" {
		t.Fatalf("ct %s", s)
	}
	if s := gstr(t, loop, "__json"); s != "true" {
		t.Fatalf("json %s", s)
	}
	if s := gstr(t, loop, "__bytes"); s != "123,34,111,107,34,58,116,114,117,101,44,34,98,105,110,34,58,91,48,44,49,50,56,44,50,53,53,93,125" {
		t.Fatalf("bytes %s", s)
	}
	if gstr(t, loop, "__hasBin") != "true" {
		t.Fatalf("text missing bin")
	}
}

func TestFetch_PostBodyHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_, _ = w.Write([]byte(r.Header.Get("X-Test") + "|" + string(body)))
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		Enable(vm)
		_ = EnableFetch(vm, loop)
		_, err := vm.RunString(`
			globalThis.__done=false; globalThis.__err="";
			fetch("` + srv.URL + `", { method:"POST", headers:{"X-Test":"header-ok"}, body:"payload-ok" })
				.then(r => r.text()).then(t => { globalThis.__result = t; })
				.catch(e => { globalThis.__err = String(e); })
				.finally(() => { globalThis.__done = true; });
		`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	waitBool(t, loop, "__done")
	if e := gstr(t, loop, "__err"); e != "" {
		t.Fatalf("err: %s", e)
	}
	if s := gstr(t, loop, "__result"); s != "header-ok|payload-ok" {
		t.Fatalf("got %s", s)
	}
}

func TestFetch_AbortSignal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		abort.Enable(vm)
		Enable(vm)
		_ = EnableFetch(vm, loop)
		_, err := vm.RunString(`
			globalThis.__done=false; globalThis.__err="";
			(async () => {
				const c = new AbortController();
				const p = fetch("` + srv.URL + `", { signal: c.signal })
					.then(() => { globalThis.__result = "resolved"; })
					.catch(e => { globalThis.__result = String(e); });
				setTimeout(() => c.abort("abort-ok"), 10);
				try { await p; } catch(e) { globalThis.__err = String(e); }
				finally { globalThis.__done = true; }
			})();
		`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	waitBool(t, loop, "__done")
	if gstr(t, loop, "__result") != "abort-ok" {
		t.Fatalf("expected abort-ok, got %q err=%q", gstr(t, loop, "__result"), gstr(t, loop, "__err"))
	}
}
