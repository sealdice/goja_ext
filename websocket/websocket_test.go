package websocket //nolint:testpackage

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/gorilla/websocket"
	"github.com/sealdice/goja_ext/eventloop"
)

type recordingDialer struct {
	calls chan []string
}

func (d *recordingDialer) DialContext(
	_ context.Context,
	_ string,
	_ http.Header,
	protocols []string,
	_ bool,
) (*websocket.Conn, *http.Response, error) {
	d.calls <- append([]string(nil), protocols...)
	return nil, nil, errors.New("injected dial failure")
}

func TestEnableRejectsForeignEventLoop(t *testing.T) {
	loop := eventloop.NewEventLoop(eventloop.EnableConsole(false))
	defer loop.Stop()
	err := Enable(goja.New(), loop)
	if err == nil || !strings.Contains(err.Error(), "different runtime") {
		t.Fatalf("foreign loop error = %v", err)
	}
}

func TestNewUsesInstanceScopedDependencies(t *testing.T) {
	first := New()
	second := New()

	if first.manager == second.manager {
		t.Fatal("New() modules unexpectedly share a connection manager")
	}
	if first.manager == GlobalConnManager || second.manager == GlobalConnManager {
		t.Fatal("New() unexpectedly uses the compatibility global connection manager")
	}
	if first.logger == nil || second.logger == nil || first.dialer == nil || second.dialer == nil {
		t.Fatal("New() did not initialize instance-scoped dependencies")
	}
}

func TestInjectedDialerOwnsConnectionLifecycle(t *testing.T) {
	loop := startLoop(t)
	manager := &WebSocketManager{}
	dialer := &recordingDialer{calls: make(chan []string, 1)}

	runOnLoopSync(loop, func(vm *goja.Runtime) {
		err := EnableWithOptions(vm, loop, WithDialer(dialer), WithConnectionManager(manager))
		if err != nil {
			t.Fatalf("enable websocket: %v", err)
		}
		_, err = vm.RunString(`
			globalThis.__injectedError = "";
			const ws = new WebSocket("ws://example.invalid/socket", ["chat", "json"]);
			ws.onerror = (event) => { globalThis.__injectedError = String(event.error || "error"); };
		`)
		if err != nil {
			t.Fatalf("construct websocket: %v", err)
		}
	})

	select {
	case protocols := <-dialer.calls:
		if strings.Join(protocols, ",") != "chat,json" {
			t.Fatalf("dial protocols = %v, want [chat json]", protocols)
		}
	case <-time.After(time.Second):
		t.Fatal("injected dialer was not called")
	}

	waitForCondition(t, time.Second, func() bool {
		return manager.Len() == 0
	})
}

func TestWebSocketTLSDefaultsToVerification(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer closeTestConnection(t, conn)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.StartTLS()
	defer srv.Close()
	wssURL := "wss" + strings.TrimPrefix(srv.URL, "https")

	loop := startLoop(t)
	defaultManager := &WebSocketManager{}
	runOnLoopSync(loop, func(vm *goja.Runtime) {
		err := EnableWithOptions(vm, loop, WithConnectionManager(defaultManager))
		if err != nil {
			t.Fatalf("enable default websocket: %v", err)
		}
		_, err = vm.RunString(`
			globalThis.__tlsOpened = false;
			globalThis.__tlsError = "";
			const ws = new WebSocket("` + wssURL + `");
			ws.onopen = () => { globalThis.__tlsOpened = true; ws.close(); };
			ws.onerror = (event) => { globalThis.__tlsError = String(event.error || "error"); };
		`)
		if err != nil {
			t.Fatalf("construct default websocket: %v", err)
		}
	})

	waitForCondition(t, 3*time.Second, func() bool {
		failed := false
		runOnLoopSync(loop, func(vm *goja.Runtime) {
			failed = vm.Get("__tlsError").String() != ""
			if vm.Get("__tlsOpened").ToBoolean() {
				t.Fatal("default TLS configuration accepted an untrusted certificate")
			}
		})
		return failed
	})

	trustedLoop := startLoop(t)
	trustedManager := &WebSocketManager{}
	roots := x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	runOnLoopSync(trustedLoop, func(vm *goja.Runtime) {
		err := EnableWithOptions(
			vm,
			trustedLoop,
			WithConnectionManager(trustedManager),
			WithTLSConfig(&tls.Config{RootCAs: roots}),
		)
		if err != nil {
			t.Fatalf("enable trusted websocket: %v", err)
		}
		_, err = vm.RunString(`
			globalThis.__trustedOpened = false;
			globalThis.__trustedError = "";
			const ws = new WebSocket("` + wssURL + `");
			ws.onopen = () => { globalThis.__trustedOpened = true; ws.close(); };
			ws.onerror = (event) => { globalThis.__trustedError = String(event.error || "error"); };
		`)
		if err != nil {
			t.Fatalf("construct trusted websocket: %v", err)
		}
	})

	waitForCondition(t, 3*time.Second, func() bool {
		opened := false
		runOnLoopSync(trustedLoop, func(vm *goja.Runtime) {
			if errText := vm.Get("__trustedError").String(); errText != "" {
				t.Fatalf("trusted TLS websocket failed: %s", errText)
			}
			opened = vm.Get("__trustedOpened").ToBoolean()
		})
		return opened
	})
}

func startLoop(t *testing.T) *eventloop.EventLoop {
	t.Helper()
	loop := eventloop.NewEventLoop(eventloop.EnableConsole(false))
	go loop.StartInForeground()
	time.Sleep(20 * time.Millisecond)
	t.Cleanup(func() {
		loop.Stop()
	})
	return loop
}

func runOnLoopSync(loop *eventloop.EventLoop, f func(*goja.Runtime)) {
	done := make(chan struct{})
	loop.RunOnLoop(func(vm *goja.Runtime) {
		f(vm)
		close(done)
	})
	<-done
}

func waitForCondition(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timeout waiting for condition")
}

func closeTestConnection(t *testing.T, connection *websocket.Conn) {
	t.Helper()
	if err := connection.Close(); err != nil {
		t.Logf("close test websocket: %v", err)
	}
}

func TestWebSocketBasicMessage(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer closeTestConnection(t, conn)
		_ = conn.WriteMessage(websocket.TextMessage, []byte("hello-ws"))
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	loop := startLoop(t)
	GlobalConnManager.CloseAll()
	t.Cleanup(func() { GlobalConnManager.CloseAll() })

	runOnLoopSync(loop, func(vm *goja.Runtime) {
		if err := Enable(vm, loop); err != nil {
			t.Error(err)
			return
		}
		_, err := vm.RunString(`globalThis.__wsMsg = ""; globalThis.__wsErr = "";`)
		if err != nil {
			t.Fatalf("init ws globals failed: %v", err)
		}

		script := `
			const ws = new WebSocket("` + wsURL + `");
			ws.onmessage = (ev) => { globalThis.__wsMsg = String(ev.data); ws.close(); };
			ws.onerror = (ev) => { globalThis.__wsErr = String(ev.error || "ws error"); };
		`
		_, err = vm.RunString(script)
		if err != nil {
			t.Fatalf("run websocket script failed: %v", err)
		}
	})

	waitForCondition(t, 3*time.Second, func() bool {
		done := false
		runOnLoopSync(loop, func(vm *goja.Runtime) {
			msg := vm.Get("__wsMsg").String()
			errText := vm.Get("__wsErr").String()
			if errText != "" {
				t.Fatalf("websocket failed: %s", errText)
			}
			done = msg == "hello-ws"
		})
		return done
	})
}

func wsURL(serverURL string) string {
	return "ws" + strings.TrimPrefix(serverURL, "http")
}

func TestWebSocketReadyStateConstants(t *testing.T) {
	GlobalConnManager.CloseAll()
	t.Cleanup(func() { GlobalConnManager.CloseAll() })

	loop := startLoop(t)

	var got string
	var runErr error
	runOnLoopSync(loop, func(vm *goja.Runtime) {
		if err := Enable(vm, loop); err != nil {
			t.Error(err)
			return
		}
		v, err := vm.RunString(`JSON.stringify([WebSocket.CONNECTING, WebSocket.OPEN, WebSocket.CLOSING, WebSocket.CLOSED])`)
		if err != nil {
			runErr = err
			return
		}
		got = v.String()
	})
	if runErr != nil {
		t.Fatalf("run failed: %v", runErr)
	}
	if got != "[0,1,2,3]" {
		t.Fatalf("expected [0,1,2,3], got %s", got)
	}
}

func TestWebSocketBinaryMessage(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer closeTestConnection(t, conn)
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2, 3})
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	loop := startLoop(t)
	GlobalConnManager.CloseAll()
	t.Cleanup(func() { GlobalConnManager.CloseAll() })

	var binErr, binVal, binKind string
	runOnLoopSync(loop, func(vm *goja.Runtime) {
		if err := Enable(vm, loop); err != nil {
			t.Error(err)
			return
		}
		_, _ = vm.RunString(`globalThis.__bin = ""; globalThis.__kind = ""; globalThis.__err = "";`)
		_, _ = vm.RunString(`
			const ws = new WebSocket("` + wsURL(srv.URL) + `");
			ws.onmessage = (ev) => {
				var d = ev.data;
				globalThis.__kind = Object.prototype.toString.call(d);
				var arr = [];
				if (d && typeof d.length === "number") {
					for (var i = 0; i < d.length; i++) arr.push(d[i]);
				} else if (d && typeof d.byteLength === "number") {
					var u = new Uint8Array(d);
					for (var j = 0; j < u.length; j++) arr.push(u[j]);
				}
			globalThis.__bin = JSON.stringify(arr);
		};
			ws.onerror = (ev) => { globalThis.__err = String(ev.error || "err"); };
		`)
	})

	waitForCondition(t, 3*time.Second, func() bool {
		finished := false
		runOnLoopSync(loop, func(vm *goja.Runtime) {
			binVal = vm.Get("__bin").String()
			if binVal == "[1,2,3]" {
				finished = true
				return
			}
			binErr = vm.Get("__err").String()
			binKind = vm.Get("__kind").String()
			if binErr != "" {
				finished = true
			}
		})
		return finished
	})
	if binVal != "[1,2,3]" {
		t.Fatalf("binary message mismatch: got %s (kind=%s), want [1,2,3], err=%q", binVal, binKind, binErr)
	}
}

func TestWebSocketAddRemoveEventListener(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer closeTestConnection(t, conn)
		_ = conn.WriteMessage(websocket.TextMessage, []byte("ping"))
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	url := wsURL(srv.URL)
	loop := startLoop(t)
	GlobalConnManager.CloseAll()
	t.Cleanup(func() { GlobalConnManager.CloseAll() })

	runOnLoopSync(loop, func(vm *goja.Runtime) {
		if err := Enable(vm, loop); err != nil {
			t.Error(err)
			return
		}
		_, _ = vm.RunString(`
			globalThis.__c1 = 0; globalThis.__c2 = 0; globalThis.__err1 = "";
			const ws = new WebSocket("` + url + `");
			ws.addEventListener("message", () => { globalThis.__c1++; });
			ws.addEventListener("message", () => { globalThis.__c2++; });
			ws.onerror = (ev) => { globalThis.__err1 = String(ev.error || ""); };
		`)
	})
	var errText string
	waitForCondition(t, 3*time.Second, func() bool {
		fin := false
		runOnLoopSync(loop, func(vm *goja.Runtime) {
			errText = vm.Get("__err1").String()
			if errText != "" {
				fin = true
				return
			}
			fin = vm.Get("__c1").ToInteger() >= 1 && vm.Get("__c2").ToInteger() >= 1
		})
		return fin
	})
	if errText != "" {
		t.Fatalf("two-listeners scenario error: %s", errText)
	}

	runOnLoopSync(loop, func(vm *goja.Runtime) {
		_, _ = vm.RunString(`
			globalThis.__dup = 0; globalThis.__err2 = "";
			const ws2 = new WebSocket("` + url + `");
			const f = () => { globalThis.__dup++; };
			ws2.addEventListener("message", f);
			ws2.addEventListener("message", f);
			ws2.onerror = (ev) => { globalThis.__err2 = String(ev.error || ""); };
		`)
	})
	waitForCondition(t, 3*time.Second, func() bool {
		fin := false
		runOnLoopSync(loop, func(vm *goja.Runtime) {
			errText = vm.Get("__err2").String()
			if errText != "" {
				fin = true
				return
			}
			fin = vm.Get("__dup").ToInteger() >= 1
		})
		return fin
	})
	if errText != "" {
		t.Fatalf("dedup scenario error: %s", errText)
	}
	var dup int64
	runOnLoopSync(loop, func(vm *goja.Runtime) {
		dup = vm.Get("__dup").ToInteger()
	})
	if dup != 1 {
		t.Fatalf("dedup scenario: listener fired %d times, want 1", dup)
	}

	runOnLoopSync(loop, func(vm *goja.Runtime) {
		_, _ = vm.RunString(`
			globalThis.__rem = 0; globalThis.__got = 0; globalThis.__err3 = "";
			const ws3 = new WebSocket("` + url + `");
			const g = () => { globalThis.__rem++; };
			ws3.addEventListener("message", g);
			ws3.removeEventListener("message", g);
			ws3.onmessage = () => { globalThis.__got++; };
			ws3.onerror = (ev) => { globalThis.__err3 = String(ev.error || ""); };
		`)
	})
	waitForCondition(t, 3*time.Second, func() bool {
		fin := false
		runOnLoopSync(loop, func(vm *goja.Runtime) {
			errText = vm.Get("__err3").String()
			if errText != "" {
				fin = true
				return
			}
			fin = vm.Get("__got").ToInteger() >= 1
		})
		return fin
	})
	if errText != "" {
		t.Fatalf("remove scenario error: %s", errText)
	}
	var rem int64
	runOnLoopSync(loop, func(vm *goja.Runtime) {
		rem = vm.Get("__rem").ToInteger()
	})
	if rem != 0 {
		t.Fatalf("removeEventListener scenario: removed listener fired %d times, want 0", rem)
	}
}

func TestWebSocketCloseWithCodeReason(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer closeTestConnection(t, conn)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	loop := startLoop(t)
	GlobalConnManager.CloseAll()
	t.Cleanup(func() { GlobalConnManager.CloseAll() })

	runOnLoopSync(loop, func(vm *goja.Runtime) {
		if err := Enable(vm, loop); err != nil {
			t.Error(err)
			return
		}
		_, _ = vm.RunString(`
			globalThis.__hasExpected = false;
			globalThis.__closeCount = 0;
			globalThis.__err = "";
			const ws = new WebSocket("` + wsURL(srv.URL) + `");
			ws.onopen = () => { ws.close(1000, "bye"); };
			ws.onclose = (ev) => {
				globalThis.__closeCount++;
				if (ev.code === 1000 && ev.reason === "bye" && ev.wasClean === true) {
					globalThis.__hasExpected = true;
				}
			};
			ws.onerror = (ev) => { globalThis.__err = String(ev.error || ""); };
		`)
	})

	var errText string
	waitForCondition(t, 3*time.Second, func() bool {
		fin := false
		runOnLoopSync(loop, func(vm *goja.Runtime) {
			errText = vm.Get("__err").String()
			fin = vm.Get("__hasExpected").ToBoolean()
		})
		return fin
	})
	var closeCount int64
	var hasExpected bool
	runOnLoopSync(loop, func(vm *goja.Runtime) {
		closeCount = vm.Get("__closeCount").ToInteger()
		hasExpected = vm.Get("__hasExpected").ToBoolean()
		errText = vm.Get("__err").String()
	})
	// After the close-dispatch fix: close fires exactly once (code 1000) and the
	// normal 1000 close no longer mis-triggers onerror.
	if closeCount != 1 {
		t.Fatalf("onclose should fire exactly once, got closeCount=%d", closeCount)
	}
	if !hasExpected {
		t.Fatalf("expected a single close event with code=1000 reason=\"bye\" wasClean=true")
	}
	if errText != "" {
		t.Fatalf("normal close should not trigger onerror, got err=%q", errText)
	}
}

func TestWebSocketConnectError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("no upgrade"))
	}))
	defer srv.Close()

	loop := startLoop(t)
	GlobalConnManager.CloseAll()
	t.Cleanup(func() { GlobalConnManager.CloseAll() })

	runOnLoopSync(loop, func(vm *goja.Runtime) {
		if err := Enable(vm, loop); err != nil {
			t.Error(err)
			return
		}
		_, _ = vm.RunString(`
			globalThis.__err = ""; globalThis.__opened = false;
			const ws = new WebSocket("` + wsURL(srv.URL) + `");
			globalThis.__ws = ws;
			ws.onerror = (ev) => { globalThis.__err = String(ev.error || "err"); };
			ws.onopen = () => { globalThis.__opened = true; };
		`)
	})

	var errText string
	waitForCondition(t, 3*time.Second, func() bool {
		fin := false
		runOnLoopSync(loop, func(vm *goja.Runtime) {
			errText = vm.Get("__err").String()
			fin = errText != ""
		})
		return fin
	})
	if errText == "" {
		t.Fatal("expected error on failed connection, got none")
	}

	var opened bool
	var rs int64
	runOnLoopSync(loop, func(vm *goja.Runtime) {
		opened = vm.Get("__opened").ToBoolean()
		rs = vm.Get("__ws").ToObject(vm).Get("readyState").ToInteger()
	})
	if opened {
		t.Fatal("onopen must not fire on failed connection")
	}
	if rs != 3 {
		t.Fatalf("readyState=%d, want 3 (CLOSED) after failed connect", rs)
	}
}

func TestWebSocketSendNotOpen(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer closeTestConnection(t, conn)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	loop := startLoop(t)
	GlobalConnManager.CloseAll()
	t.Cleanup(func() { GlobalConnManager.CloseAll() })

	runOnLoopSync(loop, func(vm *goja.Runtime) {
		if err := Enable(vm, loop); err != nil {
			t.Error(err)
			return
		}
		_, _ = vm.RunString(`
			globalThis.__err = "";
			const ws = new WebSocket("` + wsURL(srv.URL) + `");
			globalThis.__ws = ws;
			ws.onerror = (ev) => { if (!globalThis.__err) globalThis.__err = String(ev.error || ""); };
			ws.send("x");
		`)
	})

	var errText string
	waitForCondition(t, 3*time.Second, func() bool {
		fin := false
		runOnLoopSync(loop, func(vm *goja.Runtime) {
			errText = vm.Get("__err").String()
			fin = strings.Contains(errText, "connection is not open")
		})
		return fin
	})

	waitForCondition(t, 3*time.Second, func() bool {
		stable := false
		runOnLoopSync(loop, func(vm *goja.Runtime) {
			rs := vm.Get("__ws").ToObject(vm).Get("readyState").ToInteger()
			stable = rs != 0
		})
		return stable
	})
}

func TestWebSocketProtocolNegotiation(t *testing.T) {
	upgrader := websocket.Upgrader{
		Subprotocols: []string{"chat", "json"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer closeTestConnection(t, conn)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	url := wsURL(srv.URL)
	loop := startLoop(t)
	GlobalConnManager.CloseAll()
	t.Cleanup(func() { GlobalConnManager.CloseAll() })

	runOnLoopSync(loop, func(vm *goja.Runtime) {
		if err := Enable(vm, loop); err != nil {
			t.Error(err)
			return
		}
		_, _ = vm.RunString(`
			globalThis.__proto1 = ""; globalThis.__err1 = "";
			const ws = new WebSocket("` + url + `", ["chat", "json"]);
			ws.onopen = () => { globalThis.__proto1 = ws.protocol; };
			ws.onerror = (ev) => { globalThis.__err1 = String(ev.error || ""); };
		`)
	})
	var errText string
	waitForCondition(t, 3*time.Second, func() bool {
		fin := false
		runOnLoopSync(loop, func(vm *goja.Runtime) {
			errText = vm.Get("__err1").String()
			if errText != "" {
				fin = true
				return
			}
			fin = vm.Get("__proto1").String() == "chat"
		})
		return fin
	})
	if errText != "" {
		t.Fatalf("array-form protocol error: %s", errText)
	}

	runOnLoopSync(loop, func(vm *goja.Runtime) {
		_, _ = vm.RunString(`
			globalThis.__proto2 = ""; globalThis.__err2 = "";
			const ws2 = new WebSocket("` + url + `", "chat");
			ws2.onopen = () => { globalThis.__proto2 = ws2.protocol; };
			ws2.onerror = (ev) => { globalThis.__err2 = String(ev.error || ""); };
		`)
	})
	var proto2 string
	waitForCondition(t, 3*time.Second, func() bool {
		fin := false
		runOnLoopSync(loop, func(vm *goja.Runtime) {
			errText = vm.Get("__err2").String()
			if errText != "" {
				fin = true
				return
			}
			proto2 = vm.Get("__proto2").String()
			fin = proto2 == "chat"
		})
		return fin
	})
	if errText != "" {
		t.Fatalf("string-form protocol error: %s", errText)
	}
	if proto2 != "chat" {
		t.Fatalf("string-form protocol=%q, want chat", proto2)
	}
}

func TestWebSocketConnManagerLifecycle(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer closeTestConnection(t, conn)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	url := wsURL(srv.URL)
	loop := startLoop(t)
	GlobalConnManager.CloseAll()
	t.Cleanup(func() { GlobalConnManager.CloseAll() })

	runOnLoopSync(loop, func(vm *goja.Runtime) {
		if err := Enable(vm, loop); err != nil {
			t.Error(err)
			return
		}
		_, _ = vm.RunString(`
			globalThis.__opened = 0; globalThis.__err = "";
			const mk = () => {
				const ws = new WebSocket("` + url + `");
				ws.onopen = () => { globalThis.__opened++; };
				ws.onerror = (ev) => { globalThis.__err = String(ev.error || ""); };
				return ws;
			};
			globalThis.__a = mk();
			globalThis.__b = mk();
			globalThis.__c = mk();
		`)
	})

	var errText string
	waitForCondition(t, 3*time.Second, func() bool {
		fin := false
		runOnLoopSync(loop, func(vm *goja.Runtime) {
			errText = vm.Get("__err").String()
			if errText != "" {
				fin = true
				return
			}
			fin = vm.Get("__opened").ToInteger() == 3
		})
		return fin
	})
	if errText != "" {
		t.Fatalf("lifecycle open error: %s", errText)
	}

	GlobalConnManager.mutex.Lock()
	n := len(GlobalConnManager.connections)
	GlobalConnManager.mutex.Unlock()
	if n != 3 {
		t.Fatalf("expected 3 managed connections, got %d", n)
	}

	GlobalConnManager.CloseAll()

	GlobalConnManager.mutex.Lock()
	nAfter := len(GlobalConnManager.connections)
	GlobalConnManager.mutex.Unlock()
	if nAfter != 0 {
		t.Fatalf("expected 0 managed connections after CloseAll, got %d", nAfter)
	}

	states := make(map[string]int64)
	runOnLoopSync(loop, func(vm *goja.Runtime) {
		for _, name := range []string{"__a", "__b", "__c"} {
			obj := vm.Get(name).ToObject(vm)
			if obj == nil {
				continue
			}
			states[name] = obj.Get("readyState").ToInteger()
		}
	})
	for _, name := range []string{"__a", "__b", "__c"} {
		if states[name] != 3 {
			t.Fatalf("%s readyState=%d, want 3 (CLOSED)", name, states[name])
		}
	}
}
