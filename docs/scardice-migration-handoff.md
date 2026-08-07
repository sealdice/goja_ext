# Scardice goja 模块迁移 — 交接文档（剩余任务）

> 本文接续 `feat/migrate-scardice-modules` 分支上已完成的工作。已完成的任务在末尾“已完成工作”一节。**请按顺序完成本文剩余任务（Task 3 websocket、Final 验证，可选 Task 2 代码质量复核）。**

## 仓库与分支

- 仓库：`/home/pinenut/GolandProjects/goja_ext`，Go module `github.com/sealdice/goja_ext`
- 分支：`feat/migrate-scardice-modules`（从 `8eb67e9` 切出）
- 迁移来源：`https://github.com/Scardice/Scardice-core`，模块位于 `utils/plugin/{abort,structuredclone,utilinspect,httpextra,websocket}`
- 原始计划边界（用户已确认）：
  - 第一阶段：abort、structuredclone 独立成包；util.inspect 并入现有 `util`（✅ 已完成）
  - 第二阶段：Headers/Request/Response/FormData + fetch，**Go 层执行器用 go-resty/resty/v2，JS 侧保持标准 `fetch(url, init) -> Promise<Response>`，不把 Resty 语义暴露给脚本**（✅ 已完成）
  - websocket：单独迁移（⬇️ Task 3）
  - fs：暂不迁移（不在范围内）

---

## 关键事实（已验证，无需再推导）

接手者请直接采信以下结论，都已在本会话中实证：

1. **goja `Callable` 签名**：`func(this Value, args ...Value) (Value, error)`——**第一个参数是 JS 的 `this`**。调用实例方法时必须把实例作为第一个参数传入，否则静默作用于 `undefined`。
2. **goja `Constructor` 签名**：`func(newTarget *Object, args ...Value) (*Object, error)`，`newTarget` 传 `nil` 即可（等价于 `new Foo()`）。用 `goja.AssertConstructor(rt.Get("Map"))` 取得。
3. **goja `ClassName()` 对 Map/Set 实例返回 `"Object"`**（不是 "Map"/"Set"）。Date 返回 `"Date"`。因此识别 Map/Set 需用构造器身份：`obj.Get("constructor").SameAs(rt.Get("Map"))`。
4. **resty 版本**：必须钉在 **`v2.16.5`**（`go 1.20`）。`v2.17.0+` 要求 `go 1.23`，会把本仓库 `go.mod` 的 `go 1.23` 顶上去——**禁止升级**。
5. **resty v2.16.5 没有 `SetHTTPClient` setter**（v2.17.0 才加）。提供自定义 `*http.Client` 只能在构造期用 `resty.NewWithClient(client)`。`SetTimeout`/`SetProxy`/`SetTransport` 均存在。
6. **resty 请求头**：`req := client.R().SetContext(ctx); req.Header = http.Header{...}; req.SetBody([]byte); resp, err := req.Execute(method, url)`——`req.Header` 直接赋值会被 `Execute` 尊重；`SetContext` 取消后 `Execute` 立即返回 `context canceled` 错误。
7. **本仓库 `eventloop` 包**对外提供 `NewEventLoop(opts...)`、`Start()`/`StartInForeground()`、`Stop()`（返回 `int`）、`RunOnLoop(func(*goja.Runtime))`、`EnableConsole(bool)`。`eventloop.Logger` 接口签名：
   ```go
   type Logger interface {
       Debug(args ...interface{})
       Debugf(format string, args ...interface{})
       Info(args ...interface{})
       Infof(format string, args ...interface{})
       Warn(args ...interface{})
       Warnf(format string, args ...interface{})
       Error(args ...interface{})
       Errorf(format string, args ...interface{})
   }
   ```
8. **模块约定**（参考已完成的 `abort/module.go`、`url/module.go`）：每个模块目录 `package <name>`，含 `ModuleName` 常量、`Enable(rt)`（注册全局）、`Require(rt, module)`（设置 exports）、`init()` 调 `require.RegisterCoreModule(ModuleName, Require)`。
9. **go.mod 当前 `go 1.23`，`toolchain go1.24.5`**。任何 `go get` 不得把 `go` 指令顶到 1.21 以上。

---

# Task 3：迁移 websocket 包（解耦 logger）

## 目标

把 Scardice-core 的 `utils/plugin/websocket/websocket.go` 迁移为 `github.com/sealdice/goja_ext/websocket` 包：

- `package sealws` → `package websocket`
- 移除 `Scardice-core/logger` 依赖；日志类型改为复用 `eventloop.Logger`（类型别名），默认 no-op，可通过 `SetLogger` 注入
- 保留 `gorilla/websocket`（与上游一致，生态成熟）
- 其余逻辑（全局连接管理器、连接生命周期、读循环、事件派发、readyState、关闭码常量）**逐字保留**

## 依赖

```
go get github.com/gorilla/websocket
```
确认 `grep '^go' go.mod` 仍为 `go 1.23`。

## 上游源文件

- 源：`utils/plugin/websocket/websocket.go`
  - 原始 URL：`https://raw.githubusercontent.com/Scardice/Scardice-core/master/utils/plugin/websocket/websocket.go`
- 测试：`utils/plugin/websocket/websocket_test.go`
  - 原始 URL：`https://raw.githubusercontent.com/Scardice/Scardice-core/master/utils/plugin/websocket/websocket_test.go`

> 建议先 `curl` 上述两个 URL 取原文，再按下述规则做精确替换。本文末尾附录 A/B 也贴了原文便于离线。

## 文件

- 新建 `websocket/websocket.go`
- 新建 `websocket/websocket_test.go`

### `websocket/websocket.go` 的精确改动

对上游 `websocket.go` 应用以下改动，其余**逐字不变**：

1. **包名**：`package sealws` → `package websocket`。

2. **导入**：
   - 删除 `"Scardice-core/logger"`
   - 保留 `"github.com/dop251/goja"`、`"github.com/sealdice/goja_ext/eventloop"`、`"github.com/gorilla/websocket"`，以及标准库 `"context"`、`"crypto/tls"`、`"errors"`、`"net/http"`、`"reflect"`、`"sync"`、`"time"`
   - **新增 `"fmt"` 和 `"os"`**（见下方 `wsDefaultLogger` 实现）

3. **替换 Logger 相关定义**。删除上游的 `type Logger interface { ... }` 与 `type defaultLogger struct{}` 及其方法，替换为：

   ```go
   // Logger 复用 eventloop 的日志接口，调用方可共用同一实例。
   type Logger = eventloop.Logger

   // wsDefaultLogger 是未注入日志器时的默认实现：全部丢弃。
   type wsDefaultLogger struct{}

   func (wsDefaultLogger) Debug(...interface{})          {}
   func (wsDefaultLogger) Debugf(string, ...interface{})  {}
   func (wsDefaultLogger) Info(...interface{})           {}
   func (wsDefaultLogger) Infof(string, ...interface{})   {}
   func (wsDefaultLogger) Warn(...interface{})           {}
   func (wsDefaultLogger) Warnf(string, ...interface{})   {}
   func (wsDefaultLogger) Error(args ...interface{}) {
       fmt.Fprintln(os.Stderr, args...)
   }
   func (wsDefaultLogger) Errorf(format string, args ...interface{}) {
       fmt.Fprintf(os.Stderr, format+"\n", args...)
   }
   ```

   > 说明：用户选择“复用 eventloop.Logger 接口”。用类型别名 `type Logger = eventloop.Logger` 即可让现有 `WebSocketLogger Logger` 字段与 `SetLogger(logger Logger)` 签名自动匹配。`Error/ErrorF` 默认打到 stderr（无第三方依赖），其余级别 no-op。如你更想要完全静默的 no-op，把 Error/Errorf 也改成空函数体即可。

4. **全局变量**保持：`var WebSocketLogger Logger = wsDefaultLogger{}`、`var GlobalConnManager = &WebSocketManager{}`、`func SetLogger(logger Logger) { WebSocketLogger = logger }`。

5. **其余全部保留**：`WebSocketModule`/`WebSocket`/`WebSocketManager`（Register/Unregister/CloseAll）、`New`/`NewInstance`/`Exports`、`WebSocketConnection` 与所有字段、`WebSocketReadyState` 及常量（Connecting/Open/Closing/Closed）、关闭码常量（CloseNormalClosure…CloseTLSHandshake）、`webSocketOptions`、`writeWait`、`NewWebSocketConnection`、`bindWebSocketMethods`、`parseWebSocketOptions`、`connect`、`triggerOpen/triggerMessage/triggerClose/triggerError`、`Send`、`Close`、`closeWithoutUnregister`、`closeInternal`、`readPump`、`webSocketCloseConnection`、`Enable`、`addEventListener`/`removeEventListener`/`snapshotListeners`/`dispatchEvent`。
   - 保留 `connect()` 内 `InsecureSkipVerify: true` 与其 `//nolint:gosec` 注释（与上游一致）。

### `websocket/websocket_test.go` 的精确改动

对上游 `websocket_test.go` 应用：

1. `package sealws` → `package websocket`
2. 导入保持 `net/http`、`net/http/httptest`、`strings`、`testing`、`time`、`github.com/dop251/goja`、`github.com/sealdice/goja_ext/eventloop`、`github.com/gorilla/websocket`
3. 测试体（`startLoop`、`runOnLoopSync`、`waitForCondition`、`TestWebSocketBasicMessage`）**逐字不变**

## TDD 步骤

1. [ ] `go get github.com/gorilla/websocket`；确认 `go.mod` 仍 `go 1.23`
2. [ ] 创建 `websocket/websocket.go`（上游 + 上述改动）
3. [ ] 创建 `websocket/websocket_test.go`（上游 + 包名改）
4. [ ] `go build ./websocket/` 通过
5. [ ] `go test ./websocket/ -count=1 -race -v` 通过（`TestWebSocketBasicMessage` 起本地 `httptest.NewServer`，无外网）
6. [ ] `go vet ./websocket/`、`gofmt -l websocket/`（空）、`go build ./...`
7. [ ] 提交：`git add websocket go.mod go.sum && git commit -m "feat(websocket): migrate WebSocket client from Scardice-core (decoupled logger)"`

## 自检清单

- `package websocket`；无任何 `"Scardice-core/..."` 残留（`grep -rn Scardice websocket/` 应为空）
- `Logger` 是 `eventloop.Logger` 别名；`WebSocketLogger` 默认 `wsDefaultLogger{}`
- 保留 `GlobalConnManager`（用于优雅关闭所有连接）与 `SetLogger`
- `Enable(rt, loop)` 签名不变；测试与上游一致

## 风险与提示

- `dispatchEvent` 通过 `conn.loop.RunOnLoop` 在事件循环线程派发，避免并发进 goja——保留即可。
- 如日后想替换 ws 库（如 `coder/websocket`），需重写 dialer/读写/关闭逻辑，超出本次迁移范围。
- `connect()` 的 `InsecureSkipVerify: true` 是上游行为；若需可配置化（例如加 `WithTLSConfig` 选项），作为后续增强，不在本任务内。

---

# Final：全量构建与校验

完成 Task 3 后：

1. [ ] `go build ./...`
2. [ ] `go test ./... -count=1`（全部包通过）
3. [ ] `go vet ./...`
4. [ ] 若环境装了 staticcheck：`staticcheck ./...`（仓库有 `staticcheck.conf`）；没有就跳过
5. [ ] `go mod tidy`，确认 `go.mod` 仍 `go 1.23`、`resty v2.16.5`、含 `gorilla/websocket`
6. [ ] 如有 tidy 产生改动：`git add -A && git commit -m "chore: tidy modules after Scardice migration"`

## 收尾建议

- 完成后用 `git log --oneline 8eb67e9..HEAD` 查看本分支提交序列。
- 若要发 PR，基线 `master`（或上游 `dop251/goja_nodejs` master），PR 描述列出：abort / structuredclone / util.inspect / fetch(resty) / websocket 五个模块。
- 可选后续（不在本次范围）：为各模块补 TypeScript 类型定义（`@dop251/types-goja_nodejs-*` 约定，类似 `url/types`、`buffer/types`）。

---

# 可选：Task 2（fetch）代码质量复核

Task 2（`fetch/` 包）已**通过 spec 合规复核**，`go test ./fetch/ -race -v` 全绿，提交 `5040fb5`。若希望走完整的两阶段评审，可按下述做一次代码质量复核（非阻塞）：

- 范围：`git diff 5b666c3..5040fb5`
- 重点检查：
  - `fetch/fetch.go::doFetchRequest` 的 resty 用法与错误归一（abort 与普通网络错误的区分）
  - `attachFetchAbortSignal` 的并发顺序（先 `data.abort.set(reason)` 再 `data.cancel()`，读在 `RunOnLoop` 回调中，二者均在 loop 线程，无 race）
  - `fetch/options.go` 的选项优先级（WithRestyClient > WithHTTPClient > 默认）与 v2.16.5 适配
  - `classes.go` 是否彻底去除 `Enable`/`Require` 与 `seal*` 命名（`grep seal fetch/` 应为空）
  - Resty 类型是否泄漏到 JS 侧（应只在 `fetch.go`/`options.go`）
- 如发现问题，按既有节奏：实现者修复 → 复核 → 通过后 `git commit --amend` 或追加 fixup 提交。

---

# 附录 A：上游 `utils/plugin/websocket/websocket.go` 原文

> 包名 `sealws`。迁移时改 `package websocket`，并按上文替换 Logger 部分、删 `Scardice-core/logger` 导入。

```go
package sealws

// Package websocket 提供了一个与goja兼容的WebSocket客户端实现
// 这个包提供了标准的WebSocket API，可以在任何goja环境中使用

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"reflect"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/eventloop"
	"github.com/gorilla/websocket"

	"Scardice-core/logger"
)

// Logger 日志接口，与helper.go中的Helper方法签名一致
type Logger interface {
	Debug(a ...interface{})
	Debugf(format string, a ...interface{})
	Info(a ...interface{})
	Infof(format string, a ...interface{})
	Warn(a ...interface{})
	Warnf(format string, a ...interface{})
	Error(a ...interface{})
	Errorf(format string, a ...interface{})
}

// defaultLogger 默认日志实现（无操作）
type defaultLogger struct{}

func (l *defaultLogger) Debug(_ ...interface{})            {}
func (l *defaultLogger) Debugf(_ string, _ ...interface{}) {}
func (l *defaultLogger) Info(_ ...interface{})             {}
func (l *defaultLogger) Infof(_ string, _ ...interface{})  {}
func (l *defaultLogger) Warn(_ ...interface{})             {}
func (l *defaultLogger) Warnf(_ string, _ ...interface{})  {}
func (l *defaultLogger) Error(args ...interface{}) {
	logger.M().Errorf("[WebSocket ERROR] %v", args...)
}
func (l *defaultLogger) Errorf(format string, args ...interface{}) {
	logger.M().Errorf("[WebSocket ERROR] "+format, args...)
}

// WebSocketLogger 全局日志实例，用户可以通过SetLogger函数替换
var WebSocketLogger Logger = &defaultLogger{}

// GlobalConnManager 是一个全局的WebSocket管理器 用来最后优雅销毁的
var GlobalConnManager = &WebSocketManager{}

// SetLogger 设置全局日志实例
func SetLogger(logger Logger) {
	WebSocketLogger = logger
}

type (
	// WebSocketModule 是WebSocket模块的根实例
	WebSocketModule struct{}

	// WebSocket 表示WebSocket模块的一个实例
	WebSocket struct {
		rt   *goja.Runtime
		loop *eventloop.EventLoop
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
	return &WebSocket{
		rt:   rt,
		loop: loop,
	}
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
	loop         *eventloop.EventLoop
	ctx          context.Context
	conn         *websocket.Conn
	scheduled    chan goja.Callable
	done         chan struct{}
	shutdownOnce sync.Once
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

	if ws.loop == nil {
		panic(rt.NewTypeError("WebSocket requires event loop. Please provide eventloop when calling Enable()."))
	}

	conn := &WebSocketConnection{
		rt:             rt,
		loop:           ws.loop,
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

	WebSocketLogger.Debugf("开始建立WebSocket连接 url=%s protocols=%v", conn.url, options.protocols)

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
		WebSocketLogger.Errorf("WebSocket连接失败 url=%s error=%s", conn.url, err.Error())
		conn.setReadyState(Closed)
		conn.triggerError(err)
		return
	}

	WebSocketLogger.Infof("WebSocket连接建立成功 url=%s", conn.url)

	if resp != nil {
		conn.protocol = resp.Header.Get("Sec-WebSocket-Protocol")
		if conn.protocol != "" {
			WebSocketLogger.Debugf("选择的子协议 protocol=%s", conn.protocol)
		}
		_ = resp.Body.Close()
	}

	conn.conn = wsConn
	conn.setReadyState(Open)

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
	conn.setReadyState(Closed)
	conn.dispatchEvent("close", func(vm *goja.Runtime, event *goja.Object) {
		_ = event.Set("code", code)
		_ = event.Set("reason", reason)
		_ = event.Set("wasClean", code == CloseNormalClosure)
		_ = event.Set("type", "close")
	})
}

func (conn *WebSocketConnection) triggerError(err error) {
	WebSocketLogger.Errorf("触发WebSocket错误事件 url=%s error=%s", conn.url, err.Error())

	conn.dispatchEvent("error", func(vm *goja.Runtime, event *goja.Object) {
		_ = event.Set("error", err.Error())
		_ = event.Set("type", "error")
	})
}

func (conn *WebSocketConnection) Send(message string) error {
	readyState := conn.getReadyState()
	if readyState != Open {
		err := errors.New("connection is not open")
		WebSocketLogger.Warnf("尝试在非开放连接上发送消息 readyState=%d url=%s", readyState, conn.url)
		return err
	}

	WebSocketLogger.Debugf("发送WebSocket消息 url=%s messageLength=%d", conn.url, len(message))

	err := conn.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err != nil {
		WebSocketLogger.Errorf("设置WebSocket写入超时失败 url=%s error=%s", conn.url, err.Error())
		return err
	}
	err = conn.conn.WriteMessage(websocket.TextMessage, []byte(message))
	if err != nil {
		WebSocketLogger.Errorf("发送WebSocket消息失败 url=%s error=%s", conn.url, err.Error())
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
	WebSocketLogger.Infof("关闭WebSocket连接 url=%s code=%d reason=%s", conn.url, code, reason)
	conn.closeInternal(true, args...)
}

func (conn *WebSocketConnection) closeWithoutUnregister(args ...interface{}) {
	conn.closeInternal(false, args...)
}

func (conn *WebSocketConnection) closeInternal(shouldUnregister bool, args ...interface{}) {
	readyState, started := conn.startClosing()
	if !started {
		WebSocketLogger.Debugf("连接已经关闭或正在关闭 readyState=%d url=%s", readyState, conn.url)
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

	WebSocketLogger.Infof("主动关闭WebSocket连接 url=%s code=%d reason=%s", conn.url, code, reason)

	if conn.conn != nil {
		err := conn.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason))
		if err != nil {
			WebSocketLogger.Warnf("发送关闭消息失败 url=%s error=%s", conn.url, err.Error())
		}
		_ = conn.conn.Close()
	}

	conn.triggerClose(code, reason)
	if shouldUnregister {
		conn.webSocketCloseConnection()
	} else {
		conn.shutdownOnce.Do(func() {
			close(conn.done)
			if conn.conn != nil {
				_ = conn.conn.Close()
			}
		})
	}
}

func (conn *WebSocketConnection) readPump() {
	defer conn.webSocketCloseConnection()

	WebSocketLogger.Debugf("开始WebSocket消息读取循环 url=%s", conn.url)

	for {
		select {
		case <-conn.done:
			WebSocketLogger.Debugf("WebSocket消息读取循环结束 url=%s", conn.url)
			return
		default:
			messageType, message, err := conn.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					WebSocketLogger.Errorf("WebSocket意外关闭错误 url=%s error=%s", conn.url, err.Error())
					conn.triggerError(err)
				} else {
					WebSocketLogger.Infof("WebSocket连接正常关闭 url=%s error=%s", conn.url, err.Error())
				}
				conn.triggerClose(CloseAbnormalClosure, "connection lost")
				return
			}

			switch messageType {
			case websocket.TextMessage:
				WebSocketLogger.Debugf("接收到文本消息 url=%s messageLength=%d", conn.url, len(message))
				conn.triggerMessage(string(message))
			case websocket.BinaryMessage:
				WebSocketLogger.Debugf("接收到二进制消息 url=%s messageLength=%d", conn.url, len(message))
				conn.triggerMessage(message)
			default:
				WebSocketLogger.Warnf("接收到未知类型消息 url=%s messageType=%d", conn.url, messageType)
			}
		}
	}
}

func (conn *WebSocketConnection) webSocketCloseConnection() {
	conn.shutdownOnce.Do(func() {
		WebSocketLogger.Debugf("清理WebSocket连接资源 url=%s", conn.url)
		GlobalConnManager.Unregister(conn)
		close(conn.done)
		if conn.conn != nil {
			_ = conn.conn.Close()
		}
		WebSocketLogger.Debugf("WebSocket连接资源清理完成 url=%s", conn.url)
	})
}

func Enable(rt *goja.Runtime, loop *eventloop.EventLoop) {
	module := New()
	instance := module.NewInstance(rt, loop)
	_ = rt.Set("WebSocket", instance.Exports())
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
	if conn.loop == nil {
		return
	}
	conn.loop.RunOnLoop(func(vm *goja.Runtime) {
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
					WebSocketLogger.Errorf("处理WebSocket%s事件失败 url=%s error=%s", eventType, conn.url, err.Error())
				}
			}
		}

		listeners := conn.snapshotListeners(eventType)
		for _, l := range listeners {
			if fn, ok := goja.AssertFunction(l); ok {
				if _, err := fn(conn.jsObject, event); err != nil {
					WebSocketLogger.Errorf("处理WebSocket%s事件监听失败 url=%s error=%s", eventType, conn.url, err.Error())
				}
			}
		}
	})
}
```

# 附录 B：上游 `utils/plugin/websocket/websocket_test.go` 原文

> 包名改 `websocket`，其余逐字保留。

```go
package sealws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/eventloop"
	"github.com/gorilla/websocket"
)

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

func TestWebSocketBasicMessage(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte("hello-ws"))
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	loop := startLoop(t)
	GlobalConnManager.CloseAll()
	t.Cleanup(func() { GlobalConnManager.CloseAll() })

	runOnLoopSync(loop, func(vm *goja.Runtime) {
		Enable(vm, loop)
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
```

---

# 已完成工作（仅供参考）

分支 `feat/migrate-scardice-modules`，自 `8eb67e9` 起：

| 任务 | 提交 | 说明 |
|---|---|---|
| Task 1.1 abort | `43a4cc7` | `abort/`：AbortController/AbortSignal（含 `removeEventListener`、timeout、reason 等修正）；7 测试 `-race` 通过 |
| Task 1.2 structuredclone | `a09280d` | `structuredclone/`：修复上游 Map/Set/Date 克隆缺陷（goja `ClassName`=="Object"、Callable `this` 绑定、`AssertConstructor(nil)`）；7 测试通过 |
| Task 1.3 util.inspect | `5b666c3` | `util/inspect.go` 并入 `util`；修复 `inspect(v, undefined/null)` 崩溃；`require("util").inspect` |
| Task 2 fetch | `5040fb5` | `fetch/`：Headers/Request/Response/FormData + `fetch`（go-resty v2.16.5 执行器，`EnableFetch(rt, loop, opts...)`）；3 测试（GET/POST/abort）通过；spec 复核通过 |

新增依赖：`github.com/go-resty/resty/v2 v2.16.5`。

## 迁移中对上游做的已确认修正（接手者知悉）

- abort：`AbortSignal.abort()` 静态方法改为先 `buildSignalObj` 再 `doAbort`（原上游顺序导致返回的 signal `aborted=false`）；`removeEventListener` 实现为按身份移除（原上游为空操作）。
- structuredclone：Map/Set 用 `AssertConstructor` 构造真 Map/Set，迭代 `entries()`/`values()` 与写入 `set`/`add` 均以实例为 `this`；Date 用 `new Date(getTime())` 返回独立实例；因 goja 对 Map/Set 的 `ClassName()=="Object"`，按构造器身份 `SameAs` 路由。
- util.inspect：`inspect(value, undefined|null)` 不再抛 `TypeError`（`ToObject` 前先判空）。
- fetch：Go 层执行器从上游的 `http.Handler`+`httptest.NewRecorder` 改为 `go-resty`；UA 改 `goja_nodejs-fetch/1.0`，去掉默认 `connection: close`；resty 不暴露给 JS。
