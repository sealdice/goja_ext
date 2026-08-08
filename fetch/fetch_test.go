package fetch

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/abort"
	"github.com/sealdice/goja_ext/eventloop"
	"github.com/sealdice/goja_ext/require"
	"github.com/sealdice/goja_ext/url"
)

func TestEnableAndRequireShareCanonicalConstructors(t *testing.T) {
	rt := goja.New()
	new(require.Registry).Enable(rt)
	Enable(rt)
	value, err := rt.RunString(`
		const web = require("fetch");
		Headers === web.Headers && Request === web.Request &&
		Response === web.Response && FormData === web.FormData &&
		require("fetch") === web;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if !value.ToBoolean() {
		t.Fatal("global and CommonJS fetch constructors are not canonical")
	}
}

func TestFetchResponseUsesCanonicalPrototypes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Test", "yes")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		new(require.Registry).Enable(vm)
		Enable(vm)
		if err := EnableFetch(vm, loop); err != nil {
			t.Fatalf("EnableFetch: %v", err)
		}
		_, err := vm.RunString(`
			globalThis.__done = false;
			fetch("` + srv.URL + `").then((response) => {
				const web = require("fetch");
				globalThis.__canonical = response instanceof Response &&
					response instanceof web.Response &&
					response.headers instanceof Headers;
				globalThis.__done = true;
			}, (error) => {
				globalThis.__prototypeError = String(error);
				globalThis.__done = true;
			});
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if errText := gstr(t, loop, "__prototypeError"); errText != "" {
		t.Fatalf("fetch rejected: %s", errText)
	}
	if got := gstr(t, loop, "__canonical"); got != "true" {
		t.Fatalf("canonical prototype check = %q", got)
	}
}

func TestEnableFetchRejectsForeignEventLoop(t *testing.T) {
	loop := eventloop.NewEventLoop(eventloop.EnableConsole(false))
	defer loop.Stop()
	err := EnableFetch(goja.New(), loop)
	if err == nil || !strings.Contains(err.Error(), "different runtime") {
		t.Fatalf("foreign loop error = %v", err)
	}
}

func TestFetchAlreadyAbortedStandardSignal(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("unexpected"))
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		Enable(vm)
		if err := EnableFetch(vm, loop); err != nil {
			t.Fatal(err)
		}
		_, err := vm.RunString(`
			globalThis.__done = false;
			const signal = {
				aborted: true,
				reason: "already stopped",
				addEventListener() { throw new Error("must not subscribe"); }
			};
			fetch("` + srv.URL + `", { signal }).then(
				() => { globalThis.__abortResult = "resolved"; },
				(reason) => { globalThis.__abortResult = String(reason); }
			).finally(() => { globalThis.__done = true; });
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__abortResult"); got != "already stopped" {
		t.Fatalf("abort result = %q", got)
	}
	if count := requests.Load(); count != 0 {
		t.Fatalf("aborted fetch reached server %d times", count)
	}
}

func TestFetchRemovesAbortListenerAfterBodyCompletes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("complete"))
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		Enable(vm)
		if err := EnableFetch(vm, loop); err != nil {
			t.Fatal(err)
		}
		_, err := vm.RunString(`
			globalThis.__done = false;
			let added = 0;
			let removed = 0;
			let subscribed;
			const signal = {
				aborted: false,
				reason: undefined,
				addEventListener(type, fn) { added++; subscribed = fn; },
				removeEventListener(type, fn) {
					if (type === "abort" && fn === subscribed) removed++;
				}
			};
			fetch("` + srv.URL + `", { signal }).then((response) => response.text()).then(
				(text) => { globalThis.__cleanup = [text, added, removed].join("|"); },
				(error) => { globalThis.__cleanup = "ERR:" + error; }
			).finally(() => { globalThis.__done = true; });
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__cleanup"); got != "complete|1|1" {
		t.Fatalf("abort listener lifecycle = %q", got)
	}
}

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
	t.Fatalf("timeout waiting for %s (stage=%q err=%q result=%q panics=%d)",
		name, gstr(t, loop, "__stage"), gstr(t, loop, "__err"),
		gstr(t, loop, "__result"), loop.GetPanicCount())
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
					const ab = await r.arrayBuffer();
					const bytes = new Uint8Array(ab);
					let text = ""; for (let i=0;i<bytes.length;i++) text += String.fromCharCode(bytes[i]);
					const json = JSON.parse(text);
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

func TestFetch_StreamingBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("chunk1"))
		if flusher != nil {
			flusher.Flush()
		}
		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write([]byte("chunk2"))
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		Enable(vm)
		_ = EnableFetch(vm, loop)
		_, err := vm.RunString(`
			globalThis.__done = false; globalThis.__err = ""; globalThis.__stage = "started";
			(async () => {
				try {
					const r = await fetch("` + srv.URL + `");
					globalThis.__stage = "response";
					const reader = r.body.getReader();
					const out = [];
					const dec = (u8) => { let s = ""; for (let i=0;i<u8.length;i++) s += String.fromCharCode(u8[i]); return s; };
					while (true) {
						globalThis.__stage = "reading-" + out.length;
						const { done, value } = await reader.read();
						globalThis.__stage = done ? "eof" : "read-" + out.length;
						if (done) break;
						out.push(dec(value));
					}
					globalThis.__result = out.join("|");
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
	if s := gstr(t, loop, "__result"); s != "chunk1|chunk2" {
		t.Fatalf("streaming result %q", s)
	}
}

func TestFetch_BodyConsumedTwiceFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("data"))
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		Enable(vm)
		_ = EnableFetch(vm, loop)
		_, err := vm.RunString(`
			globalThis.__done = false; globalThis.__err = ""; globalThis.__twice = "";
			(async () => {
				try {
					const r = await fetch("` + srv.URL + `");
					await r.text();
					try {
						await r.json();
						globalThis.__twice = "no-throw";
					} catch (e) { globalThis.__twice = "threw"; }
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
	if s := gstr(t, loop, "__twice"); s != "threw" {
		t.Fatalf("expected body reuse to throw, got %q", s)
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

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func TestHeaders_API(t *testing.T) {
	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		Enable(vm)
		_, err := vm.RunString(`
			const h = new Headers();
			h.set("Content-Type", "x");
			h.set("X-A", "1");
			h.set("X-B", "2");
			const r = [];
			r.push(h.get("content-type"));
			r.push(h.get("CONTENT-TYPE"));
			h.append("Content-Type", "y");
			r.push(h.get("content-type"));
			r.push(String(h.has("X-A")));
			r.push(String(h.has("nope")));
			const fe = [];
			h.forEach(function(v, k){ fe.push(k + "=" + v); });
			r.push(fe.sort().join("|"));
			r.push(Array.from(h.keys()).sort());
			r.push(Array.from(h.values()).sort());
			r.push(Array.from(h.entries()).sort());
			let invalidName = false;
			try { h.append(" bad", "value"); } catch { invalidName = true; }
			r.push(String(invalidName));
			h.delete("content-type");
			r.push(String(h.has("content-type")));
			globalThis.__res = JSON.stringify(r);
		`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	got := gstr(t, loop, "__res")
	want := mustJSON(t, []interface{}{
		"x", "x", "x, y", "true", "false",
		"content-type=x, y|x-a=1|x-b=2",
		[]string{"content-type", "x-a", "x-b"},
		[]string{"1", "2", "x, y"},
		[][]string{{"content-type", "x, y"}, {"x-a", "1"}, {"x-b", "2"}},
		"true",
		"false",
	})
	if got != want {
		t.Fatalf("headers api: got %s want %s", got, want)
	}
}

func TestHeaders_InitForms(t *testing.T) {
	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		Enable(vm)
		_, err := vm.RunString(`
			const h1 = new Headers({"a":"1", "B":"2"});
			const r = [];
			r.push(Array.from(h1.entries()).sort());
			const h2 = new Headers([["a","1"],["a","2"]]);
			r.push(Array.from(h2.entries()));
			r.push(h2.get("a"));
			const h3 = new Headers(h1);
			r.push(Array.from(h3.entries()).sort());
			h3.set("a","x");
			r.push(h1.get("a"));
			r.push(h3.get("a"));
			globalThis.__res = JSON.stringify(r);
		`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	got := gstr(t, loop, "__res")
	want := mustJSON(t, []interface{}{
		[][]string{{"a", "1"}, {"b", "2"}},
		[][]string{{"a", "1, 2"}},
		"1, 2",
		[][]string{{"a", "1"}, {"b", "2"}},
		"1",
		"x",
	})
	if got != want {
		t.Fatalf("headers init: got %s want %s", got, want)
	}
}

func TestFormData_API(t *testing.T) {
	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		Enable(vm)
		_, err := vm.RunString(`
			const fd = new FormData();
			fd.append("a", "1");
			fd.append("a", "2");
			fd.append("b", "3");
			const r = [];
			r.push(fd.get("a"));
			r.push(fd.getAll("a"));
			r.push(String(fd.has("a")));
			r.push(String(fd.has("c")));
			fd.set("a", "x");
			r.push(fd.get("a"));
			r.push(fd.getAll("a"));
			fd.delete("b");
			r.push(String(fd.has("b")));
			const fe = [];
			fd.forEach(function(v, k){ fe.push(k + "=" + v); });
			r.push(fe.join(","));
			r.push(Array.from(fd.keys()));
			r.push(Array.from(fd.values()));
			r.push(Array.from(fd.entries()));
			globalThis.__res = JSON.stringify(r);
		`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	got := gstr(t, loop, "__res")
	want := mustJSON(t, []interface{}{
		"1",
		[]string{"1", "2"},
		"true",
		"false",
		"x",
		[]string{"x"},
		"false",
		"a=x",
		[]string{"a"},
		[]string{"x"},
		[][]string{{"a", "x"}},
	})
	if got != want {
		t.Fatalf("formdata api: got %s want %s", got, want)
	}
}

func TestResponse_Constructor(t *testing.T) {
	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		Enable(vm)
		_, err := vm.RunString(`
			globalThis.__done = false; globalThis.__err = "";
			(async () => {
				try {
					const r1 = new Response("hi", {status:201, statusText:"Created"});
					globalThis.__ok1 = String(r1.ok);
					globalThis.__status1 = String(r1.status);
					globalThis.__st1 = r1.statusText;
					const rh = new Response("", {headers:{"X-Foo":"bar"}});
					globalThis.__foo = rh.headers.get("x-foo");
					const r2 = new Response(null, {status:404});
					globalThis.__ok2 = String(r2.ok);
					globalThis.__status2 = String(r2.status);
					globalThis.__text3 = await new Response("hi").text();
					const j = await new Response('{"a":1}').json();
					globalThis.__json4 = JSON.stringify(j);
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
	if s := gstr(t, loop, "__ok1"); s != "true" {
		t.Fatalf("ok1 %s", s)
	}
	if s := gstr(t, loop, "__status1"); s != "201" {
		t.Fatalf("status1 %s", s)
	}
	if s := gstr(t, loop, "__st1"); s != "Created" {
		t.Fatalf("statusText %s", s)
	}
	if s := gstr(t, loop, "__foo"); s != "bar" {
		t.Fatalf("header passthrough %s", s)
	}
	if s := gstr(t, loop, "__ok2"); s != "false" {
		t.Fatalf("ok2 %s", s)
	}
	if s := gstr(t, loop, "__status2"); s != "404" {
		t.Fatalf("status2 %s", s)
	}
	if s := gstr(t, loop, "__text3"); s != "hi" {
		t.Fatalf("text %s", s)
	}
	if s := gstr(t, loop, "__json4"); s != `{"a":1}` {
		t.Fatalf("json %s", s)
	}
}

func TestResponse_JSONError(t *testing.T) {
	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		Enable(vm)
		_, err := vm.RunString(`
			globalThis.__done = false;
			(async () => {
				try {
					await new Response("not json").json();
					globalThis.__result = "resolved";
				} catch(e) { globalThis.__result = "rejected:" + String(e); }
				finally { globalThis.__done = true; }
			})();
		`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	waitBool(t, loop, "__done")
	res := gstr(t, loop, "__result")
	if !strings.HasPrefix(res, "rejected:SyntaxError:") {
		t.Fatalf("expected json() to reject with SyntaxError, got %q", res)
	}
}

func TestFetch_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		Enable(vm)
		_ = EnableFetch(vm, loop)
		_, err := vm.RunString(`
			globalThis.__done = false; globalThis.__err = "";
			(async () => {
				try {
					const r = await fetch("` + srv.URL + `");
					globalThis.__ok = String(r.ok);
					globalThis.__status = String(r.status);
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
		t.Fatalf("404 should resolve not reject, err: %s", e)
	}
	if s := gstr(t, loop, "__ok"); s != "false" {
		t.Fatalf("ok %s", s)
	}
	if s := gstr(t, loop, "__status"); s != "404" {
		t.Fatalf("status %s", s)
	}
}

func TestFetch_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"a":1,"b":"hi"}`))
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		Enable(vm)
		_ = EnableFetch(vm, loop)
		_, err := vm.RunString(`
			globalThis.__done = false; globalThis.__err = "";
			(async () => {
				try {
					const r = await fetch("` + srv.URL + `");
					const j = await r.json();
					globalThis.__a = String(j.a);
					globalThis.__b = j.b;
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
	if s := gstr(t, loop, "__a"); s != "1" {
		t.Fatalf("json.a %s", s)
	}
	if s := gstr(t, loop, "__b"); s != "hi" {
		t.Fatalf("json.b %s", s)
	}
}

func TestRequest_Constructor(t *testing.T) {
	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		Enable(vm)
		_, err := vm.RunString(`
			const r1 = new Request("http://x");
			const r = [];
			r.push(r1.method);
			const r2 = new Request("http://x", {method:"post"});
			r.push(r2.method);
			const signal = { aborted: false };
			const r3 = new Request("http://x", {method:"PUT", headers:{"X-H":"v"}, body:"data", signal});
			r.push(r3.method);
			r.push(r3.headers.get("x-h"));
			r.push(String(r3.signal === signal));
			r.push(r3.url);
			globalThis.__res = JSON.stringify(r);
		`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	got := gstr(t, loop, "__res")
	want := mustJSON(t, []string{"GET", "POST", "PUT", "v", "true", "http://x/"})
	if got != want {
		t.Fatalf("request ctor: got %s want %s", got, want)
	}
}

func TestFormData_MultipartBody(t *testing.T) {
	var gotCT, field1, fileName, fileContent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		_ = r.ParseMultipartForm(10 << 20)
		field1 = r.FormValue("field1")
		if f, fh, err := r.FormFile("file1"); err == nil {
			fileName = fh.Filename
			b, _ := io.ReadAll(f)
			fileContent = string(b)
			if err := f.Close(); err != nil {
				t.Error(err)
			}
		}
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		Enable(vm)
		_ = EnableFetch(vm, loop)
		_, err := vm.RunString(`
			globalThis.__done = false; globalThis.__err = "";
			(async () => {
				try {
					const fd = new FormData();
					fd.append("field1", "value1");
					fd.append("file1", new (require("fetch").File)(["filecontent"], "test.txt", {type:"text/plain"}));
					await fetch("` + srv.URL + `", {method:"POST", body: fd});
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
	if !strings.Contains(gotCT, "multipart/form-data; boundary=") {
		t.Fatalf("content-type %q", gotCT)
	}
	if field1 != "value1" {
		t.Fatalf("field1 %q", field1)
	}
	if fileName != "test.txt" {
		t.Fatalf("filename %q", fileName)
	}
	if fileContent != "filecontent" {
		t.Fatalf("filecontent %q", fileContent)
	}
}

func TestURLSearchParams_Body(t *testing.T) {
	var gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		url.Enable(vm)
		Enable(vm)
		_ = EnableFetch(vm, loop)
		_, err := vm.RunString(`
			globalThis.__done = false; globalThis.__err = "";
			(async () => {
				try {
					const params = new URLSearchParams();
					params.append("a", "1");
					params.append("b", "2");
					await fetch("` + srv.URL + `", {method:"POST", body: params});
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
	if gotBody != "a=1&b=2" {
		t.Fatalf("body %q", gotBody)
	}
	if gotCT != "application/x-www-form-urlencoded;charset=UTF-8" {
		t.Fatalf("content-type %q", gotCT)
	}
}

func TestFetch_DefaultHeaders(t *testing.T) {
	var mu sync.Mutex
	var captured []http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		captured = append(captured, r.Header.Clone())
		mu.Unlock()
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		Enable(vm)
		_ = EnableFetch(vm, loop)
		_, err := vm.RunString(`
			globalThis.__done = false; globalThis.__err = "";
			(async () => {
				try {
					await fetch("` + srv.URL + `");
					await fetch("` + srv.URL + `", {headers:{"user-agent":"custom-ua","accept":"text/plain"}});
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
	mu.Lock()
	defer mu.Unlock()
	if len(captured) != 2 {
		t.Fatalf("captured %d requests, want 2", len(captured))
	}
	if ua := captured[0].Get("User-Agent"); ua != "goja_nodejs-fetch/1.0" {
		t.Fatalf("default user-agent %q", ua)
	}
	if ac := captured[0].Get("Accept"); ac != "*/*" {
		t.Fatalf("default accept %q", ac)
	}
	if ua := captured[1].Get("User-Agent"); ua != "custom-ua" {
		t.Fatalf("override user-agent %q", ua)
	}
	if ac := captured[1].Get("Accept"); ac != "text/plain" {
		t.Fatalf("override accept %q", ac)
	}
}

func TestFetch_ModuleRequire(t *testing.T) {
	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		_, err := vm.RunString(`
			const m = require("fetch");
			globalThis.__h = typeof m.Headers;
			globalThis.__r = typeof m.Request;
			globalThis.__rs = typeof m.Response;
			globalThis.__fd = typeof m.FormData;
		`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	for _, name := range []string{"__h", "__r", "__rs", "__fd"} {
		if gstr(t, loop, name) != "function" {
			t.Fatalf("%s = %s, want function", name, gstr(t, loop, name))
		}
	}
}
