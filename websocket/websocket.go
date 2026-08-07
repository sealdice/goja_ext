package websocket

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/gorilla/websocket"
	"github.com/sealdice/goja_ext/eventloop"
	"github.com/sealdice/goja_ext/runtimehost"
)

// Logger 复用 eventloop 的日志接口，调用方可共用同一实例。
type Logger = eventloop.Logger

// wsDefaultLogger 是未注入日志器时的默认实现：Error/ErrorF 打到 stderr，其余丢弃。
type wsDefaultLogger struct{}

func (wsDefaultLogger) Debug(...interface{})          {}
func (wsDefaultLogger) Debugf(string, ...interface{}) {}
func (wsDefaultLogger) Info(...interface{})           {}
func (wsDefaultLogger) Infof(string, ...interface{})  {}
func (wsDefaultLogger) Warn(...interface{})           {}
func (wsDefaultLogger) Warnf(string, ...interface{})  {}
func (wsDefaultLogger) Error(args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
}
func (wsDefaultLogger) Errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// WebSocketLogger 全局日志实例，用户可以通过SetLogger函数替换。
// 并发读写由 loggerMu 保护；内部 goroutine 应通过 getLogger() 读取。
var (
	loggerMu        sync.RWMutex
	WebSocketLogger Logger = wsDefaultLogger{}
)

// GlobalConnManager 是一个全局的WebSocket管理器 用来最后优雅销毁的
var GlobalConnManager = &WebSocketManager{}

// getLogger 返回当前日志实例（并发安全）。
func getLogger() Logger {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	return WebSocketLogger
}

// SetLogger 设置全局日志实例（并发安全）。
func SetLogger(logger Logger) {
	loggerMu.Lock()
	WebSocketLogger = logger
	loggerMu.Unlock()
}

type (
	// WebSocketModule 是WebSocket模块的根实例
	WebSocketModule struct{}

	// WebSocket 表示WebSocket模块的一个实例
	WebSocket struct {
		rt        *goja.Runtime
		scheduler runtimehost.Scheduler
	}
	WebSocketManager struct {
		connections []*WebSocketConnection
		mutex       sync.Mutex
	}
)

// Register 注册连接
func (m *WebSocketManager) Register(conn *WebSocketConnection) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.connections = append(m.connections, conn)
}

// CloseAll 关闭所有连接
func (m *WebSocketManager) CloseAll() {
	m.mutex.Lock()
	connsCopy := make([]*WebSocketConnection, len(m.connections))
	copy(connsCopy, m.connections)
	m.connections = m.connections[:0]
	m.mutex.Unlock()

	for _, conn := range connsCopy {
		if conn != nil {
			conn.closeWithoutUnregister()
		}
	}
}

// Unregister 移除已关闭的连接
func (m *WebSocketManager) Unregister(conn *WebSocketConnection) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for i, c := range m.connections {
		if c == conn {
			m.connections = append(m.connections[:i], m.connections[i+1:]...)
			break
		}
	}
}

// New 返回一个新的WebSocketModule实例
func New() *WebSocketModule {
	return &WebSocketModule{}
}

// NewInstance 为给定的goja运行时创建一个新的WebSocket实例
func (m *WebSocketModule) NewInstance(rt *goja.Runtime, loop *eventloop.EventLoop) *WebSocket {
	instance, err := m.NewInstanceChecked(rt, loop)
	if err != nil {
		panic(err)
	}
	return instance
}

// NewInstanceChecked creates an instance after verifying that loop owns rt.
func (m *WebSocketModule) NewInstanceChecked(rt *goja.Runtime, loop *eventloop.EventLoop) (*WebSocket, error) {
	if loop == nil {
		return nil, errors.New("websocket: event loop is required")
	}
	if err := runtimehost.ValidateScheduler(rt, loop); err != nil {
		return nil, fmt.Errorf("websocket: %w", err)
	}
	if err := runtimehost.BindScheduler(rt, loop); err != nil {
		return nil, fmt.Errorf("websocket: %w", err)
	}
	return &WebSocket{
		rt:        rt,
		scheduler: loop,
	}, nil
}

// Exports 返回模块的导出对象 - 直接返回WebSocket构造函数
func (ws *WebSocket) Exports() goja.Value {
	constructorFunc := func(call goja.ConstructorCall) *goja.Object {
		funcCall := goja.FunctionCall{
			This:      call.This,
			Arguments: call.Arguments,
		}
		result := ws.NewWebSocketConnection(funcCall)
		return result.ToObject(ws.rt)
	}

	webSocketConstructor := ws.rt.ToValue(constructorFunc)
	constructorObj := webSocketConstructor.ToObject(ws.rt)

	_ = constructorObj.Set("CONNECTING", Connecting)
	_ = constructorObj.Set("OPEN", Open)
	_ = constructorObj.Set("CLOSING", Closing)
	_ = constructorObj.Set("CLOSED", Closed)

	prototypeObj := ws.rt.NewObject()
	_ = constructorObj.Set("prototype", prototypeObj)

	return webSocketConstructor
}

// WebSocketConnection 是返回给JavaScript的WebSocket连接表示
type WebSocketConnection struct {
	rt           *goja.Runtime
	scheduler    runtimehost.Scheduler
	ctx          context.Context
	conn         *websocket.Conn
	connMu       sync.RWMutex
	scheduled    chan goja.Callable
	done         chan struct{}
	shutdownOnce sync.Once
	closeFired   bool
	jsObject     *goja.Object

	url          string
	protocol     string
	readyState   WebSocketReadyState
	readyStateMu sync.RWMutex

	onopen    goja.Value
	onmessage goja.Value
	onclose   goja.Value
	onerror   goja.Value

	eventListeners map[string][]goja.Value
	listenerMutex  sync.RWMutex
}

type WebSocketReadyState int

const (
	Connecting WebSocketReadyState = 0
	Open       WebSocketReadyState = 1
	Closing    WebSocketReadyState = 2
	Closed     WebSocketReadyState = 3
)

func (conn *WebSocketConnection) getReadyState() WebSocketReadyState {
	conn.readyStateMu.RLock()
	defer conn.readyStateMu.RUnlock()
	return conn.readyState
}

func (conn *WebSocketConnection) setReadyState(state WebSocketReadyState) {
	conn.readyStateMu.Lock()
	defer conn.readyStateMu.Unlock()
	conn.readyState = state
}

func (conn *WebSocketConnection) startClosing() (WebSocketReadyState, bool) {
	conn.readyStateMu.Lock()
	defer conn.readyStateMu.Unlock()
	if conn.readyState == Closed || conn.readyState == Closing {
		return conn.readyState, false
	}
	conn.readyState = Closing
	return conn.readyState, true
}

func (conn *WebSocketConnection) getConn() *websocket.Conn {
	conn.connMu.RLock()
	defer conn.connMu.RUnlock()
	return conn.conn
}

func (conn *WebSocketConnection) setConn(c *websocket.Conn) {
	conn.connMu.Lock()
	conn.conn = c
	conn.connMu.Unlock()
}

// tryOpen 将 Connecting 原子地切换为 Open；若期间已被关闭（Closing/Closed）则返回 false。
func (conn *WebSocketConnection) tryOpen() bool {
	conn.readyStateMu.Lock()
	defer conn.readyStateMu.Unlock()
	if conn.readyState != Connecting {
		return false
	}
	conn.readyState = Open
	return true
}

const (
	CloseNormalClosure           = 1000
	CloseGoingAway               = 1001
	CloseProtocolError           = 1002
	CloseUnsupportedData         = 1003
	CloseNoStatusRcvd            = 1005
	CloseAbnormalClosure         = 1006
	CloseInvalidFramePayloadData = 1007
	ClosePolicyViolation         = 1008
	CloseMessageTooBig           = 1009
	CloseMandatoryExtension      = 1010
	CloseInternalServerErr       = 1011
	CloseTLSHandshake            = 1015
)

type webSocketOptions struct {
	headers           http.Header
	enableCompression bool
	protocols         []string
}

const writeWait = 10 * time.Second

// NewWebSocketConnection 创建一个新的WebSocket连接 (构造函数)
func (ws *WebSocket) NewWebSocketConnection(call goja.FunctionCall) goja.Value {
	rt := ws.rt
	args := call.Arguments

	if len(args) == 0 {
		panic(rt.NewTypeError("WebSocket constructor requires at least 1 argument (url)"))
	}

	url := args[0].String()
	var options *webSocketOptions

	if len(args) > 1 && !goja.IsUndefined(args[1]) && !goja.IsNull(args[1]) {
		options = parseWebSocketOptions(args[1])
	} else {
		options = &webSocketOptions{
			headers: make(http.Header),
		}
	}

	if ws.scheduler == nil {
		panic(rt.NewTypeError("WebSocket requires event loop. Please provide eventloop when calling Enable()."))
	}

	conn := &WebSocketConnection{
		rt:             rt,
		scheduler:      ws.scheduler,
		url:            url,
		protocol:       "",
		readyState:     Connecting,
		scheduled:      make(chan goja.Callable),
		done:           make(chan struct{}),
		onopen:         goja.Undefined(),
		onmessage:      goja.Undefined(),
		onclose:        goja.Undefined(),
		onerror:        goja.Undefined(),
		eventListeners: make(map[string][]goja.Value),
	}
	GlobalConnManager.Register(conn)

	conn.bindWebSocketMethods()

	go conn.connect(options)

	return rt.ToValue(conn.jsObject)
}

func (conn *WebSocketConnection) bindWebSocketMethods() {
	rt := conn.rt
	obj := rt.NewObject()

	_ = obj.DefineAccessorProperty("readyState", rt.ToValue(func() int {
		return int(conn.getReadyState())
	}), goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)

	_ = obj.DefineAccessorProperty("url", rt.ToValue(func() string {
		return conn.url
	}), goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)

	_ = obj.DefineAccessorProperty("protocol", rt.ToValue(func() string {
		return conn.protocol
	}), goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)

	_ = obj.DefineAccessorProperty("onopen", rt.ToValue(func() goja.Value {
		return conn.onopen
	}), rt.ToValue(func(val goja.Value) {
		conn.onopen = val
	}), goja.FLAG_FALSE, goja.FLAG_TRUE)

	_ = obj.DefineAccessorProperty("onmessage", rt.ToValue(func() goja.Value {
		return conn.onmessage
	}), rt.ToValue(func(val goja.Value) {
		conn.onmessage = val
	}), goja.FLAG_FALSE, goja.FLAG_TRUE)

	_ = obj.DefineAccessorProperty("onclose", rt.ToValue(func() goja.Value {
		return conn.onclose
	}), rt.ToValue(func(val goja.Value) {
		conn.onclose = val
	}), goja.FLAG_FALSE, goja.FLAG_TRUE)

	_ = obj.DefineAccessorProperty("onerror", rt.ToValue(func() goja.Value {
		return conn.onerror
	}), rt.ToValue(func(val goja.Value) {
		conn.onerror = val
	}), goja.FLAG_FALSE, goja.FLAG_TRUE)

	_ = obj.Set("send", rt.ToValue(func(message string) {
		if err := conn.Send(message); err != nil {
			conn.triggerError(err)
		}
	}))
	_ = obj.Set("close", rt.ToValue(func(args ...interface{}) {
		conn.Close(args...)
	}))

	_ = obj.Set("addEventListener", rt.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		eventType := call.Argument(0).String()
		listener := call.Argument(1)
		conn.addEventListener(eventType, listener)
		return goja.Undefined()
	}))
	_ = obj.Set("removeEventListener", rt.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		eventType := call.Argument(0).String()
		listener := call.Argument(1)
		conn.removeEventListener(eventType, listener)
		return goja.Undefined()
	}))

	conn.jsObject = obj
}

func parseWebSocketOptions(protocolsVal goja.Value) *webSocketOptions {
	options := &webSocketOptions{
		headers: make(http.Header),
	}

	if !goja.IsUndefined(protocolsVal) && !goja.IsNull(protocolsVal) {
		if protocolsVal.ExportType().Kind() == reflect.String {
			options.protocols = []string{protocolsVal.String()}
		} else {
			if protocolsArray := protocolsVal.Export(); protocolsArray != nil {
				if protocolSlice, ok := protocolsArray.([]interface{}); ok {
					for _, p := range protocolSlice {
						if str, ok2 := p.(string); ok2 {
							options.protocols = append(options.protocols, str)
						}
					}
				}
			}
		}
	}

	return options
}

func (conn *WebSocketConnection) connect(options *webSocketOptions) {
	ctx := context.Background()
	conn.ctx = ctx

	getLogger().Debugf("开始建立WebSocket连接 url=%s protocols=%v", conn.url, options.protocols)

	dialer := &websocket.Dialer{
		HandshakeTimeout:  5 * time.Second,
		EnableCompression: options.enableCompression,
		Subprotocols:      options.protocols,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec
		},
	}

	wsConn, resp, err := dialer.Dial(conn.url, options.headers)
	if err != nil {
		getLogger().Errorf("WebSocket连接失败 url=%s error=%s", conn.url, err.Error())
		conn.setReadyState(Closed)
		conn.triggerError(err)
		return
	}

	getLogger().Infof("WebSocket连接建立成功 url=%s", conn.url)

	if resp != nil {
		conn.protocol = resp.Header.Get("Sec-WebSocket-Protocol")
		if conn.protocol != "" {
			getLogger().Debugf("选择的子协议 protocol=%s", conn.protocol)
		}
		_ = resp.Body.Close()
	}

	conn.setConn(wsConn)
	if !conn.tryOpen() {
		// 在拨号期间连接已被关闭（例如 CloseAll），静默放弃。
		_ = wsConn.Close()
		conn.setConn(nil)
		return
	}

	conn.triggerOpen()

	go conn.readPump()
}

func (conn *WebSocketConnection) triggerOpen() {
	conn.dispatchEvent("open", func(vm *goja.Runtime, event *goja.Object) {
		_ = event.Set("type", "open")
	})
}

func (conn *WebSocketConnection) triggerMessage(data interface{}) {
	conn.dispatchEvent("message", func(vm *goja.Runtime, event *goja.Object) {
		_ = event.Set("data", data)
		_ = event.Set("type", "message")
	})
}

func (conn *WebSocketConnection) triggerClose(code int, reason string) {
	conn.readyStateMu.Lock()
	if conn.closeFired {
		conn.readyStateMu.Unlock()
		return
	}
	conn.closeFired = true
	conn.readyState = Closed
	conn.readyStateMu.Unlock()
	conn.dispatchEvent("close", func(vm *goja.Runtime, event *goja.Object) {
		_ = event.Set("code", code)
		_ = event.Set("reason", reason)
		_ = event.Set("wasClean", code == CloseNormalClosure)
		_ = event.Set("type", "close")
	})
}

func (conn *WebSocketConnection) triggerError(err error) {
	getLogger().Errorf("触发WebSocket错误事件 url=%s error=%s", conn.url, err.Error())

	conn.dispatchEvent("error", func(vm *goja.Runtime, event *goja.Object) {
		_ = event.Set("error", err.Error())
		_ = event.Set("type", "error")
	})
}

func (conn *WebSocketConnection) Send(message string) error {
	readyState := conn.getReadyState()
	if readyState != Open {
		err := errors.New("connection is not open")
		getLogger().Warnf("尝试在非开放连接上发送消息 readyState=%d url=%s", readyState, conn.url)
		return err
	}

	getLogger().Debugf("发送WebSocket消息 url=%s messageLength=%d", conn.url, len(message))

	c := conn.getConn()
	err := c.SetWriteDeadline(time.Now().Add(writeWait))
	if err != nil {
		getLogger().Errorf("设置WebSocket写入超时失败 url=%s error=%s", conn.url, err.Error())
		return err
	}
	err = c.WriteMessage(websocket.TextMessage, []byte(message))
	if err != nil {
		getLogger().Errorf("发送WebSocket消息失败 url=%s error=%s", conn.url, err.Error())
	}
	return err
}

func (conn *WebSocketConnection) Close(args ...interface{}) {
	code := CloseNormalClosure
	reason := ""
	if len(args) > 0 {
		if c, ok := args[0].(int); ok {
			code = c
		}
	}
	if len(args) > 1 {
		if r, ok := args[1].(string); ok {
			reason = r
		}
	}
	getLogger().Infof("关闭WebSocket连接 url=%s code=%d reason=%s", conn.url, code, reason)
	conn.closeInternal(true, args...)
}

func (conn *WebSocketConnection) closeWithoutUnregister(args ...interface{}) {
	conn.closeInternal(false, args...)
}

func (conn *WebSocketConnection) closeInternal(shouldUnregister bool, args ...interface{}) {
	readyState, started := conn.startClosing()
	if !started {
		getLogger().Debugf("连接已经关闭或正在关闭 readyState=%d url=%s", readyState, conn.url)
		return
	}

	code := CloseNormalClosure
	reason := ""

	if len(args) > 0 {
		if c, ok := args[0].(int); ok {
			code = c
		}
	}

	if len(args) > 1 {
		if r, ok := args[1].(string); ok {
			reason = r
		}
	}

	getLogger().Infof("主动关闭WebSocket连接 url=%s code=%d reason=%s", conn.url, code, reason)

	if c := conn.getConn(); c != nil {
		err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason))
		if err != nil {
			getLogger().Warnf("发送关闭消息失败 url=%s error=%s", conn.url, err.Error())
		}
		_ = c.Close()
	}

	conn.triggerClose(code, reason)
	if shouldUnregister {
		conn.webSocketCloseConnection()
	} else {
		conn.shutdownOnce.Do(func() {
			close(conn.done)
			if c := conn.getConn(); c != nil {
				_ = c.Close()
			}
		})
	}
}

func (conn *WebSocketConnection) readPump() {
	defer conn.webSocketCloseConnection()

	c := conn.getConn()
	getLogger().Debugf("开始WebSocket消息读取循环 url=%s", conn.url)

	for {
		select {
		case <-conn.done:
			getLogger().Debugf("WebSocket消息读取循环结束 url=%s", conn.url)
			return
		default:
			messageType, message, err := c.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					getLogger().Errorf("WebSocket意外关闭错误 url=%s error=%s", conn.url, err.Error())
					conn.triggerError(err)
				} else {
					getLogger().Infof("WebSocket连接正常关闭 url=%s error=%s", conn.url, err.Error())
				}
				// 仅在非主动关闭时派发被动 close 事件。主动关闭路径（closeInternal）
				// 会先把 readyState 置为 Closing，故 readPump 此处总能观察到并让出派发权。
				if state := conn.getReadyState(); state != Closing && state != Closed {
					conn.triggerClose(CloseAbnormalClosure, "connection lost")
				}
				return
			}

			switch messageType {
			case websocket.TextMessage:
				getLogger().Debugf("接收到文本消息 url=%s messageLength=%d", conn.url, len(message))
				conn.triggerMessage(string(message))
			case websocket.BinaryMessage:
				getLogger().Debugf("接收到二进制消息 url=%s messageLength=%d", conn.url, len(message))
				conn.triggerMessage(message)
			default:
				getLogger().Warnf("接收到未知类型消息 url=%s messageType=%d", conn.url, messageType)
			}
		}
	}
}

func (conn *WebSocketConnection) webSocketCloseConnection() {
	conn.shutdownOnce.Do(func() {
		getLogger().Debugf("清理WebSocket连接资源 url=%s", conn.url)
		GlobalConnManager.Unregister(conn)
		close(conn.done)
		if c := conn.getConn(); c != nil {
			_ = c.Close()
		}
		getLogger().Debugf("WebSocket连接资源清理完成 url=%s", conn.url)
	})
}

func Enable(rt *goja.Runtime, loop *eventloop.EventLoop) error {
	module := New()
	instance, err := module.NewInstanceChecked(rt, loop)
	if err != nil {
		return err
	}
	return rt.Set("WebSocket", instance.Exports())
}

func (conn *WebSocketConnection) addEventListener(eventType string, listener goja.Value) {
	if goja.IsUndefined(listener) || goja.IsNull(listener) {
		return
	}
	if _, ok := goja.AssertFunction(listener); !ok {
		return
	}
	conn.listenerMutex.Lock()
	defer conn.listenerMutex.Unlock()
	lst := conn.eventListeners[eventType]
	for _, l := range lst {
		if l == listener {
			return
		}
	}
	conn.eventListeners[eventType] = append(lst, listener)
}

func (conn *WebSocketConnection) removeEventListener(eventType string, listener goja.Value) {
	if goja.IsUndefined(listener) || goja.IsNull(listener) {
		return
	}
	conn.listenerMutex.Lock()
	defer conn.listenerMutex.Unlock()
	lst := conn.eventListeners[eventType]
	for i, l := range lst {
		if l == listener {
			conn.eventListeners[eventType] = append(lst[:i], lst[i+1:]...)
			break
		}
	}
}

func (conn *WebSocketConnection) snapshotListeners(eventType string) []goja.Value {
	conn.listenerMutex.RLock()
	defer conn.listenerMutex.RUnlock()
	lst := conn.eventListeners[eventType]
	if len(lst) == 0 {
		return nil
	}
	cp := make([]goja.Value, len(lst))
	copy(cp, lst)
	return cp
}

func (conn *WebSocketConnection) dispatchEvent(eventType string, populate func(vm *goja.Runtime, event *goja.Object)) {
	if conn.scheduler == nil {
		return
	}
	conn.scheduler.RunOnLoop(func(vm *goja.Runtime) {
		event := vm.NewObject()
		_ = event.Set("target", conn.jsObject)
		_ = event.Set("currentTarget", conn.jsObject)
		populate(vm, event)

		var handler goja.Value
		switch eventType {
		case "open":
			handler = conn.onopen
		case "message":
			handler = conn.onmessage
		case "close":
			handler = conn.onclose
		case "error":
			handler = conn.onerror
		}
		if !goja.IsUndefined(handler) && !goja.IsNull(handler) {
			if fn, ok := goja.AssertFunction(handler); ok {
				if _, err := fn(conn.jsObject, event); err != nil {
					getLogger().Errorf("处理WebSocket%s事件失败 url=%s error=%s", eventType, conn.url, err.Error())
				}
			}
		}

		listeners := conn.snapshotListeners(eventType)
		for _, l := range listeners {
			if fn, ok := goja.AssertFunction(l); ok {
				if _, err := fn(conn.jsObject, event); err != nil {
					getLogger().Errorf("处理WebSocket%s事件监听失败 url=%s error=%s", eventType, conn.url, err.Error())
				}
			}
		}
	})
}
