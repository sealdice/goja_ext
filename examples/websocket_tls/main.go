package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/gorilla/websocket"
	"github.com/sealdice/goja_ext/eventloop"
	jswebsocket "github.com/sealdice/goja_ext/websocket"
)

func main() {
	upgrader := websocket.Upgrader{}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			return
		}
		defer func() {
			if err := connection.Close(); err != nil {
				log.Printf("close websocket: %v", err)
			}
		}()
		_ = connection.WriteMessage(websocket.TextMessage, []byte("verified websocket"))
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()

	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	manager := &jswebsocket.WebSocketManager{}
	loop := eventloop.NewEventLoop(eventloop.EnableConsole(false))
	loop.Start()
	defer loop.Stop()
	defer manager.CloseAll()

	result := make(chan string, 1)
	setup := make(chan error, 1)
	if !loop.RunOnLoop(func(rt *goja.Runtime) {
		err := jswebsocket.EnableWithOptions(
			rt,
			loop,
			jswebsocket.WithConnectionManager(manager),
			jswebsocket.WithTLSConfig(&tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    roots,
			}),
		)
		if err != nil {
			setup <- err
			return
		}
		if err := rt.Set("__url", "wss"+strings.TrimPrefix(server.URL, "https")); err != nil {
			setup <- err
			return
		}
		if err := rt.Set("__finish", func(value string) { result <- value }); err != nil {
			setup <- err
			return
		}
		_, err = rt.RunString(`
			const socket = new WebSocket(__url);
			socket.onmessage = (event) => {
				__finish(String(event.data));
				socket.close();
			};
			socket.onerror = (event) => __finish("error: " + event.error);
		`)
		setup <- err
	}) {
		panic("event loop rejected setup")
	}
	if err := <-setup; err != nil {
		panic(err)
	}

	select {
	case value := <-result:
		if value != "verified websocket" {
			panic("unexpected websocket result: " + value)
		}
		fmt.Println(value)
	case <-time.After(3 * time.Second):
		panic("timed out waiting for websocket message")
	}
}
