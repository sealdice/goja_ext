package fetch //nolint:testpackage

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/abort"
	"github.com/dop251/goja_nodejs/require"
	weburl "github.com/dop251/goja_nodejs/url"
)

func TestHeadersGetMissingReturnsNull(t *testing.T) {
	rt := goja.New()
	Enable(rt)

	value, err := rt.RunString(`new Headers().get("missing") === null`)
	if err != nil {
		t.Fatal(err)
	}
	if !value.ToBoolean() {
		t.Fatal("Headers.get() did not return null for a missing header")
	}
}

func TestFormDataIteratorMethodsAreNotEnumerable(t *testing.T) {
	rt := goja.New()
	Enable(rt)

	value, err := rt.RunString(`
		["entries", "keys", "values", "forEach"].every((name) =>
			!Object.prototype.propertyIsEnumerable.call(FormData.prototype, name)
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	if !value.ToBoolean() {
		t.Fatal("FormData iterator helpers must not be enumerable")
	}
}

func TestFetchFacadeUsesCanonicalDependenciesWithoutReplacingGlobals(t *testing.T) {
	rt := goja.New()
	new(require.Registry).Enable(rt)
	for _, name := range []string{"Buffer", "URL", "URLSearchParams", "ReadableStream"} {
		if err := rt.Set(name, "sentinel:"+name); err != nil {
			t.Fatal(err)
		}
	}
	Enable(rt)

	value, err := rt.RunString(`
		const fetchAPI = require("fetch")
		const BufferCtor = require("buffer").Buffer
		const URLCtor = require("url").URL
		const blob = new fetchAPI.Blob(["ok"])
		const request = new fetchAPI.Request(new URLCtor("https://example.com/a"))
		JSON.stringify([
			blob._bytes instanceof BufferCtor,
			request.url,
			Buffer,
			URL,
			URLSearchParams,
			ReadableStream
		])
	`)
	if err != nil {
		t.Fatal(err)
	}
	want := `[true,"https://example.com/a","sentinel:Buffer","sentinel:URL","sentinel:URLSearchParams","sentinel:ReadableStream"]`
	if got := value.String(); got != want {
		t.Fatalf("canonical dependency result = %s, want %s", got, want)
	}
}

type rejectingScheduler struct {
	rt *goja.Runtime
}

func (s rejectingScheduler) Runtime() *goja.Runtime {
	return s.rt
}

func (rejectingScheduler) RunOnLoop(func(*goja.Runtime)) bool {
	return false
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type trackingBody struct {
	io.Reader
	closed chan struct{}
}

func (b *trackingBody) Close() error {
	select {
	case b.closed <- struct{}{}:
	default:
	}
	return nil
}

func TestDispatchDoesNotTouchSignalAfterSchedulerStops(t *testing.T) {
	rt := goja.New()
	body := &trackingBody{Reader: strings.NewReader("ok"), closed: make(chan struct{}, 1)}
	client := newClient(WithHTTPClient(&http.Client{Transport: roundTripperFunc(
		func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       body,
				Request:    request,
			}, nil
		},
	)}))

	var removals atomic.Int32
	signal := rt.NewObject()
	_ = signal.Set("aborted", false)
	_ = signal.Set("reason", goja.Undefined())
	_ = signal.Set("addEventListener", func(goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
	_ = signal.Set("removeEventListener", func(goja.FunctionCall) goja.Value {
		removals.Add(1)
		return goja.Undefined()
	})

	descriptor := rt.NewObject()
	_ = descriptor.Set("url", "http://example.test/")
	_ = descriptor.Set("method", http.MethodGet)
	_ = descriptor.Set("headers", rt.NewArray())
	_ = descriptor.Set("body", goja.Null())
	_ = descriptor.Set("signal", signal)
	dispatch := newDispatchFn(rt, rejectingScheduler{rt: rt}, client)
	dispatch(goja.FunctionCall{Arguments: []goja.Value{descriptor}})

	select {
	case <-body.closed:
	case <-time.After(time.Second):
		t.Fatal("response body was not closed after scheduling stopped")
	}
	if got := removals.Load(); got != 0 {
		t.Fatalf("removeEventListener called %d times off the runtime loop", got)
	}
}

func TestDefaultFetchFollowsUpToTwentyRedirects(t *testing.T) {
	const redirects = 15
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		step, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/"))
		if step < redirects {
			http.Redirect(w, r, fmt.Sprintf("/%d", step+1), http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("done"))
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		Enable(rt)
		if err := EnableFetch(rt, loop); err != nil {
			t.Fatal(err)
		}
		_, err := rt.RunString(`
			globalThis.__done = false
			fetch("` + srv.URL + `/0").then((response) => response.text()).then(
				(text) => { globalThis.__redirectLimit = text },
				(error) => { globalThis.__redirectLimit = "ERR:" + String(error) }
			).finally(() => { globalThis.__done = true })
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__redirectLimit"); got != "done" {
		t.Fatalf("redirect result = %q", got)
	}
}

func TestFetchStripsAuthorizationAcrossOrigins(t *testing.T) {
	authorization := make(chan string, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization <- r.Header.Get("Authorization")
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		Enable(rt)
		if err := EnableFetch(rt, loop); err != nil {
			t.Fatal(err)
		}
		_, err := rt.RunString(`
			globalThis.__done = false
			fetch("` + redirect.URL + `", {headers: {authorization: "Bearer secret"}}).then(
				() => { globalThis.__redirectAuth = "ok" },
				(error) => { globalThis.__redirectAuth = "ERR:" + String(error) }
			).finally(() => { globalThis.__done = true })
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__redirectAuth"); got != "ok" {
		t.Fatalf("redirect request = %q", got)
	}
	select {
	case got := <-authorization:
		if got != "" {
			t.Fatalf("authorization leaked across origins: %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("redirect target did not receive the request")
	}
}

func TestFetchAbortWhileStreamingUploadCancelsBody(t *testing.T) {
	var requests atomic.Int32
	bodyDone := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		bodyDone <- struct{}{}
	}))
	defer srv.Close()
	t.Cleanup(srv.CloseClientConnections)

	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		new(require.Registry).Enable(rt)
		abort.Enable(rt)
		Enable(rt)
		if err := EnableFetch(rt, loop); err != nil {
			t.Fatal(err)
		}
		_, err := rt.RunString(`
			globalThis.__done = false
			const { ReadableStream } = require("stream/web")
			const controller = new AbortController()
			const body = new ReadableStream({
				pull() { return new Promise(() => {}) },
				cancel(reason) { globalThis.__uploadCancelReason = reason }
			})
			fetch("` + srv.URL + `", {method: "POST", body, signal: controller.signal}).then(
				() => { globalThis.__uploadAbort = "resolved" },
				(error) => { globalThis.__uploadAbort = String(error) }
			).finally(() => { globalThis.__done = true })
			setTimeout(() => controller.abort("upload-stopped"), 10)
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__uploadAbort"); got != "upload-stopped" {
		t.Fatalf("upload abort = %q", got)
	}
	if got := gstr(t, loop, "__uploadCancelReason"); got != "upload-stopped" {
		t.Fatalf("upload stream cancel reason = %q", got)
	}
	if got := requests.Load(); got > 1 {
		t.Fatalf("aborted upload reached server %d times", got)
	}
	if requests.Load() == 1 {
		select {
		case <-bodyDone:
		case <-time.After(time.Second):
			t.Fatal("server request body did not stop after abort")
		}
	}
}

func TestFetchAbortAfterHeadersCancelsResponseBody(t *testing.T) {
	requestCanceled := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
		close(requestCanceled)
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		abort.Enable(rt)
		Enable(rt)
		if err := EnableFetch(rt, loop); err != nil {
			t.Fatal(err)
		}
		_, err := rt.RunString(`
			globalThis.__done = false
			const controller = new AbortController()
			fetch("` + srv.URL + `", {signal: controller.signal}).then(async (response) => {
				controller.abort("download-stopped")
				try {
					await response.text()
					globalThis.__downloadAbort = "resolved"
				} catch (error) {
					globalThis.__downloadAbort = "rejected"
				}
			}, (error) => {
				globalThis.__downloadAbort = "fetch:" + String(error)
			}).finally(() => { globalThis.__done = true })
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__downloadAbort"); got != "rejected" {
		t.Fatalf("download abort = %q", got)
	}
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("response abort did not cancel the server request")
	}
}

func TestBareResponseBodyAndStaticFactories(t *testing.T) {
	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		Enable(rt)
		_, err := rt.RunString(`
			globalThis.__done = false
			;(async () => {
				const original = new Response("hello")
				const clone = original.clone()
				const bytes = await original.bytes()
				const redirected = Response.redirect("https://example.com/next", 307)
				const json = Response.json({ok: true})
				globalThis.__bodyAPI = JSON.stringify([
					original.bodyUsed,
					String.fromCharCode(...bytes),
					await clone.text(),
					redirected.status,
					redirected.headers.get("location"),
					json.headers.get("content-type"),
					await json.text(),
					Response.error().type
				])
			})().catch((error) => {
				globalThis.__bodyAPI = "ERR:" + String(error)
			}).finally(() => { globalThis.__done = true })
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	want := `[true,"hello","hello",307,"https://example.com/next","application/json","{\"ok\":true}","error"]`
	if got := gstr(t, loop, "__bodyAPI"); got != want {
		t.Fatalf("body API = %s, want %s", got, want)
	}
}

func TestFetchNullBodyAndSetCookieValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Set-Cookie", "a=1; Path=/")
		w.Header().Add("Set-Cookie", "b=2; Path=/")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		Enable(rt)
		if err := EnableFetch(rt, loop); err != nil {
			t.Fatal(err)
		}
		_, err := rt.RunString(`
			globalThis.__done = false
			fetch("` + srv.URL + `").then((response) => {
				globalThis.__nullBody = JSON.stringify([
					response.body === null,
					response.headers.getSetCookie()
				])
			}, (error) => {
				globalThis.__nullBody = "ERR:" + String(error)
			}).finally(() => { globalThis.__done = true })
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	want := `[true,["a=1; Path=/","b=2; Path=/"]]`
	if got := gstr(t, loop, "__nullBody"); got != want {
		t.Fatalf("null body/set-cookie = %s, want %s", got, want)
	}
}

func TestFetchPreservesQueryOrderAndBuffersReadableStreamUpload(t *testing.T) {
	var rawQuery string
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		data, _ := io.ReadAll(r.Body)
		body = string(data)
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		new(require.Registry).Enable(rt)
		Enable(rt)
		if err := EnableFetch(rt, loop); err != nil {
			t.Fatal(err)
		}
		_, err := rt.RunString(`
			globalThis.__done = false
			const { ReadableStream } = require("stream/web")
			let index = 0
			const chunks = [new Uint8Array([97, 98]), new Uint8Array([99])]
			const body = new ReadableStream({
				pull(controller) {
					if (index < chunks.length) controller.enqueue(chunks[index++])
					else controller.close()
				}
			})
			fetch("` + srv.URL + `?z=1&a=2&z=3", {method: "POST", body}).then(
				() => { globalThis.__upload = "ok" },
				(error) => { globalThis.__upload = "ERR:" + String(error) }
			).finally(() => { globalThis.__done = true })
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__upload"); got != "ok" {
		t.Fatalf("stream upload = %s", got)
	}
	if rawQuery != "z=1&a=2&z=3" {
		t.Fatalf("query order = %q", rawQuery)
	}
	if strings.TrimSpace(body) != "abc" {
		t.Fatalf("request body = %q", body)
	}
}

func TestURLShimAcceptsStringsAndCanonicalURLInstances(t *testing.T) {
	rt := goja.New()
	urlExports := weburl.Exports(rt)
	shim := newURLShim(rt, urlExports)
	if err := rt.Set("__URL", urlExports.Get("URL")); err != nil {
		t.Fatal(err)
	}
	if err := rt.Set("__isURL", shim.Get("isURL")); err != nil {
		t.Fatal(err)
	}

	value, err := rt.RunString(`
		const url = new __URL("http://example.com/path")
		JSON.stringify([url.href, __isURL("http://example.com"), __isURL(url)])
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := value.String(), `["http://example.com/path",false,true]`; got != want {
		t.Fatalf("URL shim result = %s, want %s", got, want)
	}
}

func TestFetchResponseRedirectMetadataSurvivesClone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		Enable(rt)
		if err := EnableFetch(rt, loop); err != nil {
			t.Fatal(err)
		}
		_, err := rt.RunString(`
			globalThis.__done = false
			fetch("` + srv.URL + `/redirect").then((response) => {
				const clone = response.clone()
				globalThis.__metadata = JSON.stringify([
					response.url,
					response.redirected,
					response.statusText,
					clone.url,
					clone.redirected
				])
			}, (error) => {
				globalThis.__metadata = "ERR:" + String(error)
			}).finally(() => { globalThis.__done = true })
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")

	want := `[` + mustJSON(t, srv.URL+"/final") + `,true,"OK",` + mustJSON(t, srv.URL+"/final") + `,true]`
	if got := gstr(t, loop, "__metadata"); got != want {
		t.Fatalf("response metadata = %s, want %s", got, want)
	}
}

func TestFetchModuleExportsBlobAndFileWithoutGlobals(t *testing.T) {
	rt := goja.New()
	new(require.Registry).Enable(rt)
	Enable(rt)

	value, err := rt.RunString(`
		const web = require("fetch")
		typeof web.Blob === "function" &&
		typeof web.File === "function" &&
		typeof globalThis.Blob === "undefined" &&
		typeof globalThis.File === "undefined"
	`)
	if err != nil {
		t.Fatal(err)
	}
	if !value.ToBoolean() {
		t.Fatal("fetch module Blob/File export contract is not satisfied")
	}
}
