package fetch //nolint:testpackage

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/abort"
	"github.com/sealdice/goja_ext/require"
	"github.com/sealdice/goja_ext/streams"
	"go.uber.org/goleak"
)

func TestUploadBodyDoesNotReadNextChunkUntilCurrentChunkIsConsumed(t *testing.T) {
	rt := goja.New()
	streams.Enable(rt)
	stream, err := rt.RunString(`
		new ReadableStream({
			start(controller) {
				controller.enqueue(new Uint8Array([1, 2, 3, 4]));
				controller.enqueue(new Uint8Array([5, 6, 7, 8]));
				controller.close();
			}
		})
	`)
	if err != nil {
		t.Fatal(err)
	}
	body, err := newUploadBody(rt, immediateScheduler{rt: rt}, stream)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := body.Close(); err != nil {
			t.Errorf("close upload body: %v", err)
		}
	}()

	firstHalf := make([]byte, 2)
	if _, err := io.ReadFull(body, firstHalf); err != nil {
		t.Fatal(err)
	}
	if got := len(body.items); got != 0 {
		t.Fatalf("queued chunks while current chunk is partially consumed = %d", got)
	}
	secondHalf := make([]byte, 2)
	if _, err := io.ReadFull(body, secondHalf); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 0, len(firstHalf)+len(secondHalf))
	got = append(got, firstHalf...)
	got = append(got, secondHalf...)
	if string(got) != string([]byte{1, 2, 3, 4}) {
		t.Fatalf("first chunk = %v", got)
	}
	second := make([]byte, 4)
	if _, err := io.ReadFull(body, second); err != nil {
		t.Fatal(err)
	}
	if string(second) != string([]byte{5, 6, 7, 8}) {
		t.Fatalf("second chunk = %v", second)
	}
	if n, err := body.Read(make([]byte, 1)); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("terminal read = (%d, %v), want (0, EOF)", n, err)
	}
}

func TestUploadChunkBytesCatchesThrowingBufferGetter(t *testing.T) {
	rt := goja.New()
	value, err := rt.RunString(`({
		get buffer() { throw new Error("buffer getter exploded"); },
		byteLength: 1
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uploadChunkBytes(rt, value); err == nil || !strings.Contains(err.Error(), "conversion failed") {
		t.Fatalf("throwing byte getter error = %v", err)
	}
}

func TestUploadBodySchedulerRejectionUnblocksHTTPReader(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	rt := goja.New()
	streams.Enable(rt)
	stream, err := rt.RunString(`
		new ReadableStream({
			start(controller) {
				controller.enqueue(new Uint8Array([1, 2, 3, 4]));
				controller.enqueue(new Uint8Array([5, 6, 7, 8]));
				controller.close();
			}
		})
	`)
	if err != nil {
		t.Fatal(err)
	}
	body, err := newUploadBody(rt, rejectingScheduler{rt: rt}, stream)
	if err != nil {
		t.Fatal(err)
	}

	first := make([]byte, 4)
	if _, err := io.ReadFull(body, first); err != nil {
		t.Fatal(err)
	}
	readDone := make(chan error, 1)
	go func() {
		_, readErr := body.Read(make([]byte, 1))
		readDone <- readErr
	}()
	select {
	case readErr := <-readDone:
		if readErr == nil {
			t.Fatal("read after scheduler rejection returned no error")
		}
	case <-time.After(100 * time.Millisecond):
		_ = body.Close()
		<-readDone
		t.Fatal("HTTP reader remained blocked after scheduler rejected upload continuation")
	}
}

func TestFetchStaticRequestBodyKeepsContentLength(t *testing.T) {
	type receivedRequest struct {
		body           string
		contentLength  int64
		transferCoding []string
	}
	received := make(chan receivedRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		received <- receivedRequest{
			body:           string(body),
			contentLength:  r.ContentLength,
			transferCoding: append([]string(nil), r.TransferEncoding...),
		}
		w.WriteHeader(http.StatusNoContent)
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
			fetch("` + srv.URL + `", { method: "POST", body: "static-body" }).then(
				(response) => { globalThis.__result = String(response.status); },
				(error) => { globalThis.__result = String(error); }
			).finally(() => { globalThis.__done = true; });
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__result"); got != "204" {
		t.Fatalf("fetch result = %q", got)
	}
	got := <-received
	if got.body != "static-body" {
		t.Fatalf("request body = %q", got.body)
	}
	if got.contentLength != int64(len("static-body")) {
		t.Fatalf("Content-Length = %d", got.contentLength)
	}
	if len(got.transferCoding) != 0 {
		t.Fatalf("Transfer-Encoding = %v", got.transferCoding)
	}
}

func TestFetchRequestBodyRedirectCompatibility(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		streaming  bool
		wantMethod string
		wantBody   string
	}{
		{name: "streaming 303 becomes GET", status: http.StatusSeeOther, streaming: true, wantMethod: http.MethodGet},
		{name: "static 307 replays POST", status: http.StatusTemporaryRedirect, wantMethod: http.MethodPost, wantBody: "payload"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			type targetRequest struct{ method, body string }
			target := make(chan targetRequest, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/start":
					_, _ = io.Copy(io.Discard, r.Body)
					http.Redirect(w, r, "/target", test.status)
				case "/target":
					body, _ := io.ReadAll(r.Body)
					target <- targetRequest{method: r.Method, body: string(body)}
					w.WriteHeader(http.StatusNoContent)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			loop := startFetchLoop(t)
			runSync(t, loop, func(vm *goja.Runtime) {
				new(require.Registry).Enable(vm)
				Enable(vm)
				if err := EnableFetch(vm, loop); err != nil {
					t.Fatal(err)
				}
				bodyExpression := `"payload"`
				duplex := ""
				if test.streaming {
					bodyExpression = `new (require("streams").ReadableStream)({
						start(controller) {
							controller.enqueue(new Uint8Array([112, 97, 121, 108, 111, 97, 100]));
							controller.close();
						}
					})`
					duplex = `, duplex: "half"`
				}
				_, err := vm.RunString(`
					globalThis.__done = false;
					fetch("` + srv.URL + `/start", {
						method: "POST",
						body: ` + bodyExpression + duplex + `
					}).then(
						(response) => { globalThis.__result = String(response.status); },
						(error) => { globalThis.__result = String(error); }
					).finally(() => { globalThis.__done = true; });
				`)
				if err != nil {
					t.Fatal(err)
				}
			})
			waitBool(t, loop, "__done")
			if got := gstr(t, loop, "__result"); got != "204" {
				t.Fatalf("fetch result = %q", got)
			}
			select {
			case got := <-target:
				if got.method != test.wantMethod || got.body != test.wantBody {
					t.Fatalf("target request = %s %q, want %s %q", got.method, got.body, test.wantMethod, test.wantBody)
				}
			case <-time.After(time.Second):
				t.Fatal("redirect target was not reached")
			}
		})
	}
}

func TestFetchAbortCancelsPendingUploadReaderWithExactReason(t *testing.T) {
	started := make(chan struct{}, 1)
	serverBodyDone := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		_, err := io.Copy(io.Discard, r.Body)
		serverBodyDone <- err
	}))
	defer srv.Close()
	t.Cleanup(srv.CloseClientConnections)

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		new(require.Registry).Enable(vm)
		abort.Enable(vm)
		Enable(vm)
		if err := EnableFetch(vm, loop); err != nil {
			t.Fatal(err)
		}
		_, err := vm.RunString(`
			globalThis.__done = false;
			globalThis.__cancelled = "no";
			const { ReadableStream } = require("streams");
			const reason = { marker: "exact-upload-abort" };
			const controller = new AbortController();
			const body = new ReadableStream({
				pull() { return new Promise(() => {}); },
				cancel(value) {
					globalThis.__cancelled = value === reason ? "same" : String(value);
				}
			});
			globalThis.__abortUpload = () => controller.abort(reason);
			fetch("` + srv.URL + `", {
				method: "POST",
				body,
				duplex: "half",
				signal: controller.signal
			}).then(
				() => { globalThis.__result = "resolved"; },
				(error) => { globalThis.__result = error === reason ? "same" : String(error); }
			).finally(() => { globalThis.__done = true; });
		`)
		if err != nil {
			t.Fatal(err)
		}
	})

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("server did not receive streaming request headers")
	}
	runSync(t, loop, func(vm *goja.Runtime) {
		_, err := vm.RunString(`globalThis.__abortUpload()`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__result"); got != "same" {
		t.Fatalf("fetch abort result = %q", got)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && gstr(t, loop, "__cancelled") != "same" {
		time.Sleep(10 * time.Millisecond)
	}
	if got := gstr(t, loop, "__cancelled"); got != "same" {
		t.Fatalf("upload reader cancel reason = %q", got)
	}
	select {
	case <-serverBodyDone:
	case <-time.After(time.Second):
		t.Fatal("server request body read was not interrupted")
	}
}

func TestFetchUploadProducerErrorRejectsAsNetworkError(t *testing.T) {
	firstChunk := make(chan string, 1)
	serverBodyDone := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		first := make([]byte, len("prefix"))
		if _, err := io.ReadFull(r.Body, first); err != nil {
			serverBodyDone <- err
			return
		}
		firstChunk <- string(first)
		_, err := io.Copy(io.Discard, r.Body)
		serverBodyDone <- err
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	t.Cleanup(srv.CloseClientConnections)

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		new(require.Registry).Enable(vm)
		Enable(vm)
		if err := EnableFetch(vm, loop); err != nil {
			t.Fatal(err)
		}
		_, err := vm.RunString(`
			globalThis.__done = false;
			const { ReadableStream } = require("streams");
			const body = new ReadableStream({
				start(controller) {
					globalThis.__uploadController = controller;
					controller.enqueue(new Uint8Array([112, 114, 101, 102, 105, 120]));
				}
			});
			fetch("` + srv.URL + `", { method: "POST", body, duplex: "half" }).then(
				() => { globalThis.__result = "resolved"; },
				(error) => {
					globalThis.__result = JSON.stringify([
						error.name,
						error.code,
						String(error.cause).includes("upload exploded")
					]);
				}
			).finally(() => { globalThis.__done = true; });
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	select {
	case got := <-firstChunk:
		if got != "prefix" {
			t.Fatalf("first chunk = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not receive upload prefix")
	}
	runSync(t, loop, func(vm *goja.Runtime) {
		_, err := vm.RunString(`globalThis.__uploadController.error(new Error("upload exploded"))`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got, want := gstr(t, loop, "__result"), `["FetchError","NETWORK_ERROR",true]`; got != want {
		t.Fatalf("producer error = %s, want %s", got, want)
	}
	select {
	case <-serverBodyDone:
	case <-time.After(time.Second):
		t.Fatal("server body read did not finish after producer error")
	}
}

func TestFetchUploadRejectsNonByteChunks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		new(require.Registry).Enable(vm)
		Enable(vm)
		if err := EnableFetch(vm, loop); err != nil {
			t.Fatal(err)
		}
		_, err := vm.RunString(`
			globalThis.__done = false;
			const { ReadableStream } = require("streams");
			const body = new ReadableStream({
				start(controller) {
					controller.enqueue("not bytes");
					controller.close();
				}
			});
			fetch("` + srv.URL + `", { method: "POST", body, duplex: "half" }).then(
				(response) => { globalThis.__result = "resolved:" + response.status; },
				(error) => {
					globalThis.__result = JSON.stringify([
						error.name,
						error.code,
						String(error.cause).includes("byte")
					]);
				}
			).finally(() => { globalThis.__done = true; });
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got, want := gstr(t, loop, "__result"), `["FetchError","NETWORK_ERROR",true]`; got != want {
		t.Fatalf("non-byte upload result = %s, want %s", got, want)
	}
}

func TestFetchUploadThrowingRejectionReasonDoesNotHang(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	t.Cleanup(srv.CloseClientConnections)

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		new(require.Registry).Enable(vm)
		abort.Enable(vm)
		Enable(vm)
		if err := EnableFetch(vm, loop); err != nil {
			t.Fatal(err)
		}
		_, err := vm.RunString(`
			globalThis.__done = false;
			const { ReadableStream } = require("streams");
			const controller = new AbortController();
			const reason = {
				toString() { throw new Error("reason coercion exploded"); }
			};
			const body = new ReadableStream({
				start(streamController) { streamController.error(reason); }
			});
			fetch("` + srv.URL + `", {
				method: "POST",
				body,
				duplex: "half",
				signal: controller.signal
			}).then(
				(response) => { globalThis.__result = "resolved:" + response.status; },
				(error) => {
					globalThis.__result = error === "fallback abort"
						? "fallback abort"
						: JSON.stringify([error.name, error.code]);
				}
			).finally(() => { globalThis.__done = true; });
			setTimeout(() => controller.abort("fallback abort"), 50);
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got, want := gstr(t, loop, "__result"), `["FetchError","NETWORK_ERROR"]`; got != want {
		t.Fatalf("throwing rejection result = %s, want %s", got, want)
	}
}

func TestFetchSignalSubscriptionFailureRollsBackStreamingBody(t *testing.T) {
	var requests atomic.Int32
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		_ = request.Body.Close()
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Status:     "204 No Content",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})}

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		new(require.Registry).Enable(vm)
		Enable(vm)
		if err := EnableFetch(vm, loop, WithHTTPClient(client)); err != nil {
			t.Fatal(err)
		}
		_, err := vm.RunString(`
			globalThis.__done = false;
			globalThis.__uploadCancelled = false;
			globalThis.__signalRemoved = false;
			const { ReadableStream } = require("streams");
			const body = new ReadableStream({
				pull() { return new Promise(() => {}); },
				cancel() { globalThis.__uploadCancelled = true; }
			});
			const signal = {
				aborted: false,
				reason: undefined,
				addEventListener() { throw new Error("subscribe exploded"); },
				removeEventListener() { globalThis.__signalRemoved = true; }
			};
			fetch("http://example.test/upload", {
				method: "POST",
				body,
				duplex: "half",
				signal
			}).then(
				(response) => { globalThis.__result = "resolved:" + response.status; },
				(error) => {
					globalThis.__result = JSON.stringify([
						error.name,
						error.code,
						String(error.cause).includes("subscribe exploded")
					]);
				}
			).finally(() => { globalThis.__done = true; });
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got, want := gstr(t, loop, "__result"), `["FetchError","NETWORK_ERROR",true]`; got != want {
		t.Fatalf("subscription failure result = %s, want %s", got, want)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("subscription failure reached transport %d times", got)
	}
	if got := gstr(t, loop, "__uploadCancelled"); got != "true" {
		t.Fatalf("upload cancel state = %q", got)
	}
	if got := gstr(t, loop, "__signalRemoved"); got != "true" {
		t.Fatalf("signal listener rollback = %q", got)
	}
}

func TestFetchStreamingUploadRejectsReplayRedirects(t *testing.T) {
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var targetRequests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/start":
					_, _ = io.Copy(io.Discard, r.Body)
					http.Redirect(w, r, "/target", status)
				case "/target":
					targetRequests.Add(1)
					w.WriteHeader(http.StatusNoContent)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			loop := startFetchLoop(t)
			runSync(t, loop, func(vm *goja.Runtime) {
				new(require.Registry).Enable(vm)
				Enable(vm)
				if err := EnableFetch(vm, loop); err != nil {
					t.Fatal(err)
				}
				_, err := vm.RunString(`
					globalThis.__done = false;
					const { ReadableStream } = require("streams");
					const body = new ReadableStream({
						start(controller) {
							controller.enqueue(new Uint8Array([100, 97, 116, 97]));
							controller.close();
						}
					});
					fetch("` + srv.URL + `/start", { method: "POST", body, duplex: "half" }).then(
						(response) => { globalThis.__result = "resolved:" + response.status; },
						(error) => {
							globalThis.__result = JSON.stringify([
								error.name,
								error.code,
								String(error.cause).includes("cannot replay")
							]);
						}
					).finally(() => { globalThis.__done = true; });
				`)
				if err != nil {
					t.Fatal(err)
				}
			})
			waitBool(t, loop, "__done")
			if got, want := gstr(t, loop, "__result"), `["FetchError","NETWORK_ERROR",true]`; got != want {
				t.Fatalf("redirect result = %s, want %s", got, want)
			}
			if got := targetRequests.Load(); got != 0 {
				t.Fatalf("redirect target requests = %d, want 0", got)
			}
		})
	}
}

func TestFetchRejectsUnsupportedRequestDuplex(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		new(require.Registry).Enable(vm)
		Enable(vm)
		if err := EnableFetch(vm, loop); err != nil {
			t.Fatal(err)
		}
		_, err := vm.RunString(`
			globalThis.__done = false;
			const { ReadableStream } = require("streams");
			const body = new ReadableStream({
				start(controller) { controller.close(); }
			});
			fetch("` + srv.URL + `", { method: "POST", body, duplex: "full" }).then(
				() => { globalThis.__result = "resolved"; },
				(error) => { globalThis.__result = error.name + ":" + error.message; }
			).finally(() => { globalThis.__done = true; });
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__result"); !strings.HasPrefix(got, "TypeError:") || !strings.Contains(got, "duplex") {
		t.Fatalf("unsupported duplex result = %q", got)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("network requests = %d, want 0", got)
	}
}

func TestFetchReadableStreamRequestBodyIsSentIncrementally(t *testing.T) {
	type receivedRequest struct {
		body           string
		contentLength  int64
		transferCoding []string
	}

	firstChunk := make(chan string, 1)
	received := make(chan receivedRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		first := make([]byte, len("first"))
		if _, err := io.ReadFull(r.Body, first); err != nil {
			t.Errorf("read first chunk: %v", err)
			return
		}
		firstChunk <- string(first)
		rest, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read remaining body: %v", err)
			return
		}
		received <- receivedRequest{
			body:           string(first) + string(rest),
			contentLength:  r.ContentLength,
			transferCoding: append([]string(nil), r.TransferEncoding...),
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	loop := startFetchLoop(t)
	runSync(t, loop, func(vm *goja.Runtime) {
		new(require.Registry).Enable(vm)
		Enable(vm)
		if err := EnableFetch(vm, loop); err != nil {
			t.Fatal(err)
		}
		_, err := vm.RunString(`
			globalThis.__done = false;
			globalThis.__err = "";
			const { ReadableStream } = require("streams");
			const body = new ReadableStream({
				start(controller) {
					globalThis.__uploadController = controller;
					controller.enqueue(new Uint8Array([102, 105, 114, 115, 116]));
				}
			});
			fetch("` + srv.URL + `", { method: "POST", body })
				.then((response) => response.text())
				.then((text) => { globalThis.__result = text; })
				.catch((error) => { globalThis.__err = String(error && error.stack || error); })
				.finally(() => { globalThis.__done = true; });
		`)
		if err != nil {
			t.Fatal(err)
		}
	})

	select {
	case got := <-firstChunk:
		if got != "first" {
			t.Fatalf("first chunk = %q", got)
		}
	case <-time.After(500 * time.Millisecond):
		runSync(t, loop, func(vm *goja.Runtime) {
			_, _ = vm.RunString(`globalThis.__uploadController.close()`)
		})
		waitBool(t, loop, "__done")
		t.Fatal("server did not receive the first chunk before the request stream closed")
	}

	runSync(t, loop, func(vm *goja.Runtime) {
		_, err := vm.RunString(`
			globalThis.__uploadController.enqueue(new Uint8Array([45, 115, 101, 99, 111, 110, 100]));
			globalThis.__uploadController.close();
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__done")
	if errText := gstr(t, loop, "__err"); errText != "" {
		t.Fatalf("fetch rejected: %s", errText)
	}
	if result := gstr(t, loop, "__result"); result != "ok" {
		t.Fatalf("response body = %q", result)
	}

	select {
	case got := <-received:
		if got.body != "first-second" {
			t.Fatalf("request body = %q", got.body)
		}
		if got.contentLength != -1 {
			t.Fatalf("streaming Content-Length = %d, want -1", got.contentLength)
		}
		if len(got.transferCoding) != 1 || got.transferCoding[0] != "chunked" {
			t.Fatalf("Transfer-Encoding = %v, want [chunked]", got.transferCoding)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not finish reading request body")
	}
}
