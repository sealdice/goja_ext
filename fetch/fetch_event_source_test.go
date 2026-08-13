package fetch //nolint:testpackage

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/require"
)

func TestFetchEventSourceModuleExportsWithoutBrowserGlobals(t *testing.T) {
	rt := goja.New()
	new(require.Registry).Enable(rt)

	value, err := rt.RunString(`
		const before = [typeof window, typeof document];
		const module = require("@microsoft/fetch-event-source");
		JSON.stringify([
			Object.keys(module).sort(),
			typeof module.fetchEventSource,
			module.EventStreamContentType,
			before,
			[typeof window, typeof document]
		]);
	`)
	if err != nil {
		t.Fatal(err)
	}
	want := `[["EventStreamContentType","fetchEventSource"],"function","text/event-stream",["undefined","undefined"],["undefined","undefined"]]`
	if got := value.String(); got != want {
		t.Fatalf("module contract = %s, want %s", got, want)
	}
}

func TestEnableFetchDoesNotInstallEventSource(t *testing.T) {
	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		Enable(rt)
		if err := EnableFetch(rt, loop); err != nil {
			t.Fatal(err)
		}
		value, err := rt.RunString(`typeof EventSource`)
		if err != nil {
			t.Fatal(err)
		}
		if got := value.String(); got != "undefined" {
			t.Fatalf("typeof EventSource = %q", got)
		}
	})
}

func TestFetchEventSourceAlreadyAbortedSignalDoesNotStartRequest(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
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
			globalThis.__done = false;
			const { AbortController } = require("abort");
			const { fetchEventSource } = require("@microsoft/fetch-event-source");
			const controller = new AbortController();
			controller.abort("already stopped");
			fetchEventSource("` + srv.URL + `", { signal: controller.signal }).then(
				() => { globalThis.__result = "resolved"; },
				(error) => { globalThis.__result = String(error); }
			).finally(() => { globalThis.__done = true; });
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__result"); got != "resolved" {
		t.Fatalf("already-aborted result = %q", got)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("already-aborted SSE started %d requests", got)
	}
}

func TestFetchEventSourceDispatchesExplicitEmptyDataEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, ": comment\n\ndata:\n\n")
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
			globalThis.__done = false;
			globalThis.__events = [];
			require("@microsoft/fetch-event-source").fetchEventSource("` + srv.URL + `", {
				onmessage(message) {
					globalThis.__events.push([message.data, message.event, message.id]);
				}
			}).then(
				() => { globalThis.__result = JSON.stringify(globalThis.__events); },
				(error) => { globalThis.__err = String(error); }
			).finally(() => { globalThis.__done = true; });
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if errText := gstr(t, loop, "__err"); errText != "" {
		t.Fatalf("empty data SSE rejected: %s", errText)
	}
	if got, want := gstr(t, loop, "__result"), `[["","",""]]`; got != want {
		t.Fatalf("empty data events = %s, want %s", got, want)
	}
}

func TestFetchEventSourceOpenAIStylePOSTCompletesWithoutRetry(t *testing.T) {
	var requests atomic.Int32
	requestOK := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		body, _ := io.ReadAll(r.Body)
		requestOK <- strings.Join([]string{r.Method, r.Header.Get("Authorization"), string(body)}, "|")
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, chunk := range []string{
			": keepalive\r\n",
			"\r\ndata: {\"choices\":[{\"delta\":{\"content\":\"A\"}}]}\r",
			"\n\r\ndata: first line\r\ndata: second line\r\n\r\n",
			"data: {\"usage\":{\"total_tokens\":7}}\n\n",
			"data: [DONE]\n\n",
		} {
			_, _ = fmt.Fprint(w, chunk)
			flusher.Flush()
		}
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
			globalThis.__done = false;
			globalThis.__events = [];
			const { fetchEventSource } = require("@microsoft/fetch-event-source");
			fetchEventSource("` + srv.URL + `", {
				method: "POST",
				headers: {
					"Authorization": "Bearer test-key",
					"Content-Type": "application/json"
				},
				body: JSON.stringify({ model: "test", stream: true }),
				onmessage(event) {
					globalThis.__events.push(event.data);
				},
				onclose() {
					globalThis.__closed = true;
				}
			}).then(() => {
				globalThis.__result = JSON.stringify(globalThis.__events);
			}, (error) => {
				globalThis.__err = String(error && error.stack || error);
			}).finally(() => { globalThis.__done = true; });
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__err"); got != "" {
		t.Fatalf("fetchEventSource rejected: %s", got)
	}
	wantEvents := `["{\"choices\":[{\"delta\":{\"content\":\"A\"}}]}","first line\nsecond line","{\"usage\":{\"total_tokens\":7}}","[DONE]"]`
	if got := gstr(t, loop, "__result"); got != wantEvents {
		t.Fatalf("events = %s, want %s", got, wantEvents)
	}
	if got := gstr(t, loop, "__closed"); got != "true" {
		t.Fatalf("onclose = %q", got)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("request count = %d, want 1", got)
	}
	select {
	case got := <-requestOK:
		want := `POST|Bearer test-key|{"model":"test","stream":true}`
		if got != want {
			t.Fatalf("request = %q, want %q", got, want)
		}
	default:
		t.Fatal("server did not receive request")
	}
}

func TestFetchEventSourceParsesEveryChunkSplit(t *testing.T) {
	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		new(require.Registry).Enable(rt)
		_, err := rt.RunString(`
			globalThis.__done = false;
			const { fetchEventSource } = require("@microsoft/fetch-event-source");
			const Stream = require("streams").ReadableStream;
			const text = ": comment\r\nid: 42\r\nretry: 7\r\nevent: delta\r\ndata: first\r\ndata: second\r\n\r\n";
			const bytes = new Uint8Array(Array.from(text, (char) => char.charCodeAt(0)));
			(async () => {
				for (let split = 0; split <= bytes.length; split++) {
					const messages = [];
					await fetchEventSource("http://unused.test/", {
						openWhenHidden: true,
						fetch: async () => ({
							headers: { get: () => "text/event-stream; charset=utf-8" },
							body: new Stream({
								start(controller) {
									controller.enqueue(bytes.slice(0, split));
									controller.enqueue(bytes.slice(split));
									controller.close();
								}
							})
						}),
						onmessage: (message) => messages.push(JSON.stringify(message))
					});
					if (messages.length !== 1) throw new Error("split " + split + " count " + messages.length);
					const message = JSON.parse(messages[0]);
					if (message.data !== "first\nsecond" || message.event !== "delta" ||
						message.id !== "42" || message.retry !== 7) {
						throw new Error("split " + split + " message " + messages[0]);
					}
				}
				globalThis.__result = "ok:" + (bytes.length + 1);
			})().catch((error) => {
				globalThis.__err = String(error && error.stack || error);
			}).finally(() => { globalThis.__done = true; });
		`)
		if err != nil {
			_ = rt.Set("__err", err.Error())
			_ = rt.Set("__done", true)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__err"); got != "" {
		t.Fatalf("split parser failed: %s", got)
	}
	if got := gstr(t, loop, "__result"); !strings.HasPrefix(got, "ok:") {
		t.Fatalf("split parser result = %q", got)
	}
}

func TestFetchEventSourceRetriesWithLastEventID(t *testing.T) {
	var requests atomic.Int32
	lastEventID := make(chan string, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := requests.Add(1)
		lastEventID <- r.Header.Get("Last-Event-ID")
		w.Header().Set("Content-Type", "text/event-stream")
		if attempt == 1 {
			_, _ = fmt.Fprint(w, "id: 42\nretry: 1\ndata: first\n\n")
			return
		}
		_, _ = fmt.Fprint(w, "data: second\n\n")
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
			globalThis.__done = false;
			const messages = [];
			let closes = 0;
			require("@microsoft/fetch-event-source").fetchEventSource("` + srv.URL + `", {
				onmessage(message) { messages.push(message.data); },
				onclose() {
					if (++closes === 1) throw new Error("retry normal EOF");
				},
				onerror() { return 1; }
			}).then(() => {
				globalThis.__result = messages.join("|") + ":" + closes;
			}, (error) => { globalThis.__err = String(error); })
			.finally(() => { globalThis.__done = true; });
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__err"); got != "" {
		t.Fatalf("retry failed: %s", got)
	}
	if got := gstr(t, loop, "__result"); got != "first|second:2" {
		t.Fatalf("retry result = %q", got)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("request count = %d, want 2", got)
	}
	if got := <-lastEventID; got != "" {
		t.Fatalf("initial Last-Event-ID = %q", got)
	}
	if got := <-lastEventID; got != "42" {
		t.Fatalf("retry Last-Event-ID = %q", got)
	}
}

func TestFetchEventSourceAbortResolvesAndClosesRequest(t *testing.T) {
	requestClosed := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(requestClosed)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: started\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
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
			globalThis.__done = false;
			const controller = new (require("abort").AbortController)();
			require("@microsoft/fetch-event-source").fetchEventSource("` + srv.URL + `", {
				signal: controller.signal,
				onmessage(message) {
					globalThis.__message = message.data;
					controller.abort("finished");
				}
			}).then(() => { globalThis.__result = "resolved"; },
				(error) => { globalThis.__err = String(error); })
			.finally(() => { globalThis.__done = true; });
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__err"); got != "" {
		t.Fatalf("abort failed: %s", got)
	}
	if got := gstr(t, loop, "__message"); got != "started" {
		t.Fatalf("message = %q", got)
	}
	if got := gstr(t, loop, "__result"); got != "resolved" {
		t.Fatalf("abort result = %q", got)
	}
	select {
	case <-requestClosed:
	case <-time.After(time.Second):
		t.Fatal("request context was not canceled")
	}
}

func TestFetchEventSourceLiveOpenAICompatible(t *testing.T) {
	endpoint := os.Getenv("OPENAI_COMPAT_URL")
	apiKey := os.Getenv("OPENAI_API_KEY")
	model := os.Getenv("OPENAI_MODEL")
	if endpoint == "" || apiKey == "" || model == "" {
		t.Skip("set OPENAI_COMPAT_URL, OPENAI_API_KEY and OPENAI_MODEL to run the live smoke test")
	}
	payload, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{{
			"role": "user", "content": "Reply with exactly: ok",
		}},
		"stream":         true,
		"stream_options": map[string]bool{"include_usage": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	quotedEndpoint, _ := json.Marshal(endpoint)
	quotedKey, _ := json.Marshal("Bearer " + apiKey)
	quotedPayload, _ := json.Marshal(string(payload))

	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		new(require.Registry).Enable(rt)
		Enable(rt)
		if err := EnableFetch(rt, loop); err != nil {
			t.Fatal(err)
		}
		_, runErr := rt.RunString(`
			globalThis.__done = false;
			const controller = new (require("abort").AbortController)();
			let sawDone = false;
			let sawUsage = false;
			require("@microsoft/fetch-event-source").fetchEventSource(` + string(quotedEndpoint) + `, {
				method: "POST",
				headers: {
					"Authorization": ` + string(quotedKey) + `,
					"Content-Type": "application/json"
				},
				body: ` + string(quotedPayload) + `,
				signal: controller.signal,
				onmessage(message) {
					if (message.data === "[DONE]") {
						sawDone = true;
						controller.abort("done");
						return;
					}
					const frame = JSON.parse(message.data);
					if (frame.usage) sawUsage = true;
				}
			}).then(() => {
				globalThis.__result = JSON.stringify([sawUsage, sawDone]);
			}, (error) => { globalThis.__err = String(error && error.stack || error); })
			.finally(() => { globalThis.__done = true; });
		`)
		if runErr != nil {
			t.Fatal(runErr)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__err"); got != "" {
		t.Fatalf("live stream failed: %s", got)
	}
	if got := gstr(t, loop, "__result"); got != "[true,true]" {
		t.Fatalf("live stream usage/DONE = %s", got)
	}
}
