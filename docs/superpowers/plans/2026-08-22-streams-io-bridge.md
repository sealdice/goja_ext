# streams 统一 io 桥实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 streams 库内提供唯一的规范 io↔Stream 桥（`NewReadableStreamFromReader` / `NewReadableStreamFromBytes` / `NewWritableStreamToWriter`），并把 fs、cloudflarekv、fetch 各自手写的 4 份生产者收敛为薄委托，从根上消除 fetch 的突发交付/超前读取 bug。

**Architecture:** 桥采用标准异步 underlying-source 模式：每个 pull 在后台 goroutine 做一次 `Read`，经 `runtimehost.Scheduler.RunOnLoop` 回到事件循环后 enqueue；EOF 时 close、读失败时 error；取消/外部终止用同一个 `settled` 终态门闸防止对已关闭 controller 的竞态操作。消费者只传 reader/writer + scheduler + 差异选项（chunk 形态、错误映射、取消/收尾钩子）。

**Tech Stack:** Go 1.25、goja、web-streams-polyfill（已内嵌于 streams 包）、`runtimehost.Scheduler`（`*eventloop.EventLoop` 天然满足该接口）。

**验证命令:** `go test ./...`、`gofmt -l .`、`go vet ./...`（对齐 .github/workflows/main.yml）。

---

## 背景与现状（为什么要做）

仓库里有 4 个各自手写的 ReadableStream 生产者：

| 文件 | 源 | 状况 |
| --- | --- | --- |
| `fs/streams.go:16` | `FileHandle.Read` | 正确（pull 内异步读） |
| `cloudflarekv/stream_bridge.go:131` | `io.ReadCloser` | 正确（pull 内异步读） |
| `fetch/stream.go:335` | `io.ReadCloser` | **有 bug**：`Start` 即启动 pump，超前读取最多 16×64KiB；EOF 时 `finishOnLoop` 把积压 chunk 一次性突发 enqueue，违反背压契约 |
| `cloudflarekv/bridge.go:817` | `[]byte` | 正确（同步分块） |

streams 库只提供了低层钩子 `NewReadableStream(rt, ReadableStreamSource{Start,Pull,Cancel})`，消费者被迫各自实现"异步读 + 线程跳转 + 入队 + 背压 + EOF/error/cancel"整套编排，导致漂移。

## 目标接口（streams 包新增）

```go
// streams/reader.go
func NewReadableStreamFromReader(rt *goja.Runtime, scheduler runtimehost.Scheduler,
    reader io.ReadCloser, opts ...ReaderStreamOption) (*ReaderStream, error)
func NewReadableStreamFromBytes(rt *goja.Runtime, data []byte,
    chunkSize int, opts ...ReaderStreamOption) (*goja.Object, error)

// streams/writer.go
func NewWritableStreamToWriter(rt *goja.Runtime, scheduler runtimehost.Scheduler,
    writer io.WriteCloser, opts ...WriterOption) (*goja.Object, error)
```

选项：`WithChunkSize`、`WithHighWaterMark`、`WithChunkValue`（默认 `Uint8ArrayChunk`；cloudflarekv 传 `ArrayBufferChunk` 保持既有对外形态）、`WithMapError`、`WithOnCancel`（提供后该钩子拥有取消路径的关闭职责）、`WithOnSettled`（EOF/读错/取消/Error 后在 loop 上恰好调用一次；EOF 时 err 为 `io.EOF`，取消与外部 Error 为 nil）、`WithOnSettledOffLoop`（loop 已死时的替代钩子）。

生命周期约定（桥负责，恰好一次）：
- EOF/读错：在后台 goroutine 关闭 reader（EOF 时 close 失败替换 EOF 上报）；
- 构造失败：关闭 reader 并调用 `OnSettled(rt, nil)`；
- 取消：未提供 `WithOnCancel` 时由桥在 loop 上关闭 reader；提供后由钩子负责；
- `ReaderStream.Error(rt, reason)`（外部终止，必须在 loop 上调用）：settle 挂起的 pull、关闭 reader、`controller.error(reason)`、`OnSettled`。
- loop 已死（`RunOnLoop` 返回 false）：标记终态、关闭 reader、调用 `OnSettledOffLoop`。

消费者行为保持不变的部分：
- fs：chunk 为 Uint8Array、`WithStreamChunkSize` 可配、EOF/出错/取消都要 `closeAndWait`（读侧经包装 ReadCloser 的 Close 在 goroutine 完成；取消经 `WithOnCancel` 返回异步 promise）。
- cloudflarekv：chunk 为 ArrayBuffer、读错映射为字符串。
- fetch：读错映射为 `_FetchError NETWORK_ERROR`、abort 保留精确 reason、cleanup/cleanupOffLoop 恰好一次。

## 文件结构

- 新建 `streams/reader.go`：读侧桥（ReaderStream、选项、FromReader、FromBytes）。
- 新建 `streams/writer.go`：写侧桥。
- 新建 `streams/reader_test.go`、`streams/writer_test.go`：桥的唯一时序测试（含背压回归）。
- 改 `fs/streams.go`：两个构造器改为委托；删 `fileStreamReadResult`。
- 改 `cloudflarekv/stream_bridge.go`：删 `storageStreamSource`；`streamRecordToReadableStream` 委托。
- 改 `cloudflarekv/bridge.go`：`bytesToReadableStreamValue` 委托。
- 改 `fetch/stream.go`：删 `streamingBody` 全套（queue/waiters/wake/more/finishOnLoop/池）；`fetchReadableStream` 变薄委托。
- 改 `fetch/stream_regression_test.go`：删 streamingBody 内部机制测试，保留全部公共行为测试，新增"不超前读取"回归测试。
- 改 `fetch/stream_benchmark_test.go`：删 `BenchmarkStreamingBodyPump1MiB`（机制已删），吞吐基准移植到 `streams/reader_test.go`。

---

### Task 0: 确认绿色基线

- [x] **Step 1: 运行全量测试确认基线为绿**

Run: `go test ./...`
Expected: 全部 PASS（若有既有失败，停下来向用户确认，不要继续）。

- [x] **Step 2: 确认工作区干净**

Run: `git status --short`
Expected: 无输出。

### Task 1: streams 读侧桥（TDD）

**Files:**
- Create: `streams/reader.go`
- Test: `streams/reader_test.go`

- [x] **Step 1: 写失败测试（核心交付语义）**

创建 `streams/reader_test.go`，内容如下（完整文件）：

```go
package streams_test

import (
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/streams"
)

func newTestLoop(t *testing.T) *eventloop.EventLoop {
	t.Helper()
	loop := eventloop.NewEventLoop()
	loop.Start()
	t.Cleanup(func() { loop.Stop() })
	return loop
}

// runStreamsScript 注册 done/fail 后在 loop 上执行 script，返回 done 的值。
func runStreamsScript(
	t *testing.T,
	loop *eventloop.EventLoop,
	setup func(vm *goja.Runtime) error,
	script string,
) string {
	t.Helper()
	result := make(chan string, 1)
	setupErr := make(chan error, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		_ = vm.Set("done", func(call goja.FunctionCall) goja.Value {
			select {
			case result <- call.Argument(0).String():
			default:
			}
			return goja.Undefined()
		})
		_ = vm.Set("fail", func(call goja.FunctionCall) goja.Value {
			select {
			case result <- "FAIL:" + call.Argument(0).String():
			default:
			}
			return goja.Undefined()
		})
		setupErr <- setup(vm)
	})
	if err := <-setupErr; err != nil {
		t.Fatal(err)
	}
	runErr := make(chan error, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		_, err := vm.RunString(script)
		runErr <- err
	})
	if err := <-runErr; err != nil {
		t.Fatal(err)
	}
	select {
	case out := <-result:
		return out
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for script result")
		return ""
	}
}

type countingReadCloser struct {
	reader     io.Reader
	readCalls  atomic.Int32
	closeCount atomic.Int32
	closeOnce  sync.Once
}

func (c *countingReadCloser) Read(p []byte) (int, error) {
	c.readCalls.Add(1)
	return c.reader.Read(p)
}

func (c *countingReadCloser) Close() error {
	c.closeCount.Add(1)
	c.closeOnce.Do(func() {})
	return nil
}

type stepRead struct {
	data []byte
	err  error
}

// gatedReadCloser 每次读都先上报 readStarted，再等待 steps 投喂。
type gatedReadCloser struct {
	readStarted chan struct{}
	steps       chan stepRead
	closed      chan struct{}
	closeOnce   sync.Once
}

func newGatedReadCloser() *gatedReadCloser {
	return &gatedReadCloser{
		readStarted: make(chan struct{}, 8),
		steps:       make(chan stepRead),
		closed:      make(chan struct{}),
	}
}

func (g *gatedReadCloser) Read(p []byte) (int, error) {
	select {
	case g.readStarted <- struct{}{}:
	case <-g.closed:
		return 0, io.ErrClosedPipe
	}
	select {
	case step := <-g.steps:
		return copy(p, step.data), step.err
	case <-g.closed:
		return 0, io.ErrClosedPipe
	}
}

func (g *gatedReadCloser) Close() error {
	g.closeOnce.Do(func() { close(g.closed) })
	return nil
}

// eofWithDataReadCloser 单次 Read 同时返回数据与 EOF。
type eofWithDataReadCloser struct {
	data []byte
	done bool
}

func (r *eofWithDataReadCloser) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	return copy(p, r.data), io.EOF
}

func (r *eofWithDataReadCloser) Close() error { return nil }

func TestReaderStreamDeliversChunksAndCloses(t *testing.T) {
	loop := newTestLoop(t)
	body := &countingReadCloser{reader: strings.NewReader("hello")}
	settleErrs := make(chan error, 1)
	var settleCount atomic.Int32
	setup := func(vm *goja.Runtime) error {
		source, err := streams.NewReadableStreamFromReader(vm, loop, body,
			streams.WithOnSettled(func(_ *goja.Runtime, err error) {
				settleCount.Add(1)
				select {
				case settleErrs <- err:
				default:
				}
			}),
		)
		if err != nil {
			return err
		}
		return vm.Set("__s", source.Stream())
	}
	result := runStreamsScript(t, loop, setup, `
		(async () => {
			const reader = __s.getReader();
			let text = "";
			while (true) {
				const item = await reader.read();
				if (item.done) break;
				text += String.fromCharCode(...item.value);
			}
			done(text);
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	if result != "hello" {
		t.Fatalf("streamed text = %q", result)
	}
	if got := body.closeCount.Load(); got != 1 {
		t.Fatalf("reader Close calls = %d, want 1", got)
	}
	select {
	case err := <-settleErrs:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("settle err = %v, want io.EOF", err)
		}
	default:
		t.Fatal("OnSettled was not called")
	}
	if got := settleCount.Load(); got != 1 {
		t.Fatalf("OnSettled calls = %d, want 1", got)
	}
}

func TestReaderStreamDeliversDataReturnedWithEOF(t *testing.T) {
	loop := newTestLoop(t)
	body := &eofWithDataReadCloser{data: []byte("last")}
	setup := func(vm *goja.Runtime) error {
		source, err := streams.NewReadableStreamFromReader(vm, loop, body)
		if err != nil {
			return err
		}
		return vm.Set("__s", source.Stream())
	}
	result := runStreamsScript(t, loop, setup, `
		(async () => {
			const reader = __s.getReader();
			const first = await reader.read();
			const second = await reader.read();
			done(JSON.stringify([
				String.fromCharCode(...first.value), first.done, second.done,
			]));
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	if want := `["last",false,true]`; result != want {
		t.Fatalf("read sequence = %s, want %s", result, want)
	}
}

func TestReaderStreamMapsReadFailure(t *testing.T) {
	loop := newTestLoop(t)
	body := &countingReadCloser{reader: io.MultiReader(
		strings.NewReader("x"),
		failureReader{err: errors.New("boom")},
	)}
	setup := func(vm *goja.Runtime) error {
		source, err := streams.NewReadableStreamFromReader(vm, loop, body)
		if err != nil {
			return err
		}
		return vm.Set("__s", source.Stream())
	}
	result := runStreamsScript(t, loop, setup, `
		(async () => {
			const reader = __s.getReader();
			const first = await reader.read();
			try {
				await reader.read();
				done("resolved");
			} catch (error) {
				done(JSON.stringify([
					error instanceof Error,
					String(error.message).includes("boom"),
					String.fromCharCode(...first.value),
				]));
			}
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	if want := `[true,true,"x"]`; result != want {
		t.Fatalf("error result = %s, want %s", result, want)
	}
	if got := body.closeCount.Load(); got != 1 {
		t.Fatalf("reader Close calls = %d, want 1", got)
	}
}

type failureReader struct{ err error }

func (r failureReader) Read([]byte) (int, error) { return 0, r.err }

func TestReaderStreamDoesNotReadAheadOfConsumer(t *testing.T) {
	loop := newTestLoop(t)
	body := newGatedReadCloser()
	setup := func(vm *goja.Runtime) error {
		source, err := streams.NewReadableStreamFromReader(vm, loop, body)
		if err != nil {
			return err
		}
		return vm.Set("__s", source.Stream())
	}
	result := runStreamsScript(t, loop, setup, `
		(async () => {
			const reader = __s.getReader();
			const pending = reader.read();
			globalThis.__readPending = true;
			const item = await pending;
			done(String.fromCharCode(...item.value) + "|" + (await reader.read()).done);
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	_ = result
	select {
	case <-body.readStarted:
	default:
		t.Fatal("first read did not start")
	}
	select {
	case <-body.readStarted:
		t.Fatal("bridge read ahead of consumer demand")
	case <-time.After(30 * time.Millisecond):
	}
	body.steps <- stepRead{data: []byte("a"), err: nil}
	body.steps <- stepRead{data: nil, err: io.EOF}
}
```

- [x] **Step 2: 运行测试确认编译失败**

Run: `go test ./streams/ -run TestReaderStream`
Expected: FAIL，报 `undefined: streams.NewReadableStreamFromReader` 等编译错误。

- [x] **Step 3: 实现 streams/reader.go（读桥主体）**

创建 `streams/reader.go`：

```go
package streams

import (
	"errors"
	"io"
	"sync"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/runtimehost"
)

// DefaultChunkSize is the number of bytes read per chunk when WithChunkSize
// is not supplied.
const DefaultChunkSize = 64 * 1024

// ReaderStreamOption configures the byte-stream constructors in this file.
type ReaderStreamOption func(*readerConfig)

type readerConfig struct {
	chunkSize     int
	highWaterMark int
	chunkValue    func(rt *goja.Runtime, data []byte) goja.Value
	mapError      func(rt *goja.Runtime, err error) goja.Value
	onCancel      func(rt *goja.Runtime, reason goja.Value) goja.Value
	onSettled     func(rt *goja.Runtime, err error)
	onSettledOff  func(err error)
}

func newReaderConfig() readerConfig {
	return readerConfig{
		chunkSize:  DefaultChunkSize,
		chunkValue: Uint8ArrayChunk,
		mapError: func(rt *goja.Runtime, err error) goja.Value {
			return errorValue(rt, err)
		},
	}
}

func (config *readerConfig) apply(opts []ReaderStreamOption) {
	for _, opt := range opts {
		opt(config)
	}
	if config.chunkSize <= 0 {
		config.chunkSize = DefaultChunkSize
	}
}

// WithChunkSize sets the maximum number of bytes delivered per chunk.
func WithChunkSize(size int) ReaderStreamOption {
	return func(config *readerConfig) { config.chunkSize = size }
}

// WithHighWaterMark sets a count-based high water mark on the stream.
func WithHighWaterMark(mark int) ReaderStreamOption {
	return func(config *readerConfig) { config.highWaterMark = mark }
}

// WithChunkValue overrides how a chunk is exposed to JavaScript. The default
// wraps each chunk in a Uint8Array; the callback receives ownership of data.
func WithChunkValue(fn func(rt *goja.Runtime, data []byte) goja.Value) ReaderStreamOption {
	return func(config *readerConfig) { config.chunkValue = fn }
}

// WithMapError overrides how a failed read becomes the stream error reason.
func WithMapError(fn func(rt *goja.Runtime, err error) goja.Value) ReaderStreamOption {
	return func(config *readerConfig) { config.mapError = fn }
}

// WithOnCancel customizes stream cancellation. When set, the hook owns
// closing the reader on the cancel path (the bridge does not close it), and
// its return value becomes the cancel result.
func WithOnCancel(fn func(rt *goja.Runtime, reason goja.Value) goja.Value) ReaderStreamOption {
	return func(config *readerConfig) { config.onCancel = fn }
}

// WithOnSettled registers a callback invoked exactly once on the loop after
// the stream settles: end of input (err is io.EOF), read failure (err set),
// cancellation, or Error (err nil). It runs after the controller transition.
func WithOnSettled(fn func(rt *goja.Runtime, err error)) ReaderStreamOption {
	return func(config *readerConfig) { config.onSettled = fn }
}

// WithOnSettledOffLoop registers the fallback invoked instead of OnSettled
// when the scheduler can no longer deliver loop callbacks.
func WithOnSettledOffLoop(fn func(err error)) ReaderStreamOption {
	return func(config *readerConfig) { config.onSettledOff = fn }
}

// Uint8ArrayChunk wraps data in a Uint8Array backed by a new ArrayBuffer. It
// takes ownership of data.
func Uint8ArrayChunk(rt *goja.Runtime, data []byte) goja.Value {
	arrayBuffer := rt.NewArrayBuffer(data)
	typed, err := rt.New(rt.Get("Uint8Array"), rt.ToValue(arrayBuffer))
	if err != nil {
		panic(err)
	}
	return typed
}

// ArrayBufferChunk wraps data in an ArrayBuffer. It takes ownership of data.
func ArrayBufferChunk(rt *goja.Runtime, data []byte) goja.Value {
	return rt.ToValue(rt.NewArrayBuffer(data))
}

// ReaderStream is a ReadableStream that streams an io.ReadCloser. Each pull
// performs one read on a background goroutine (inline when the scheduler is
// nil) and delivers the result on the scheduler's loop.
type ReaderStream struct {
	rt     *goja.Runtime
	sched  runtimehost.Scheduler
	reader io.ReadCloser
	config readerConfig
	stream *goja.Object

	mu         sync.Mutex
	settled    bool
	settle     func(goja.Value)
	controller *goja.Object
}

// Stream returns the ReadableStream object.
func (s *ReaderStream) Stream() *goja.Object { return s.stream }

// Error terminates the stream with reason, closing the reader and running
// the settle hooks. It must be called on the loop goroutine.
func (s *ReaderStream) Error(rt *goja.Runtime, reason goja.Value) {
	s.mu.Lock()
	if s.settled {
		s.mu.Unlock()
		return
	}
	s.settled = true
	settle := s.settle
	s.settle = nil
	controller := s.controller
	s.mu.Unlock()

	if settle != nil {
		_ = settle(goja.Undefined())
	}
	_ = s.reader.Close()
	if controller != nil {
		callStreamController(rt, controller, "error", valueOrUndefined(reason))
	}
	if s.config.onSettled != nil {
		s.config.onSettled(rt, nil)
	}
}

// NewReadableStreamFromReader returns a ReadableStream that delivers reader
// in chunks of WithChunkSize bytes. The bridge closes reader exactly once:
// on end of input, on Error, on construction failure, and on cancellation
// unless WithOnCancel takes over that path.
func NewReadableStreamFromReader(
	rt *goja.Runtime,
	scheduler runtimehost.Scheduler,
	reader io.ReadCloser,
	opts ...ReaderStreamOption,
) (*ReaderStream, error) {
	if reader == nil {
		return nil, errors.New("streams: reader is required")
	}
	config := newReaderConfig()
	config.apply(opts)

	source := &ReaderStream{rt: rt, sched: scheduler, reader: reader, config: config}
	var strategy goja.Value
	if config.highWaterMark > 0 {
		strategyObject := rt.NewObject()
		_ = strategyObject.Set("highWaterMark", config.highWaterMark)
		_ = strategyObject.Set("size", rt.ToValue(func(goja.FunctionCall) goja.Value {
			return rt.ToValue(1)
		}))
		strategy = strategyObject
	}
	stream, err := NewReadableStream(rt, ReadableStreamSource{
		Start: func(controller *goja.Object) goja.Value {
			source.mu.Lock()
			source.controller = controller
			source.mu.Unlock()
			return goja.Undefined()
		},
		Pull:   func(*goja.Object) goja.Value { return source.pull() },
		Cancel: func(reason goja.Value) goja.Value { return source.cancel(reason) },
	}, strategy)
	if err != nil {
		_ = reader.Close()
		if config.onSettled != nil {
			config.onSettled(rt, nil)
		}
		return nil, err
	}
	source.stream = stream
	return source, nil
}

func (s *ReaderStream) pull() goja.Value {
	promise, resolve, _ := s.rt.NewPromise()
	s.mu.Lock()
	if s.settled {
		s.mu.Unlock()
		_ = resolve(goja.Undefined())
		return s.rt.ToValue(promise)
	}
	s.settle = resolve
	s.mu.Unlock()

	if s.sched == nil {
		s.readOnce()
	} else {
		go s.readOnce()
	}
	return s.rt.ToValue(promise)
}

func (s *ReaderStream) cancel(reason goja.Value) goja.Value {
	s.mu.Lock()
	if s.settled {
		s.mu.Unlock()
		return goja.Undefined()
	}
	s.settled = true
	settle := s.settle
	s.settle = nil
	s.mu.Unlock()

	if settle != nil {
		_ = settle(goja.Undefined())
	}
	if s.config.onCancel != nil {
		if result := s.config.onCancel(s.rt, reason); result != nil {
			if s.config.onSettled != nil {
				s.config.onSettled(s.rt, nil)
			}
			return result
		}
	} else {
		_ = s.reader.Close()
	}
	if s.config.onSettled != nil {
		s.config.onSettled(s.rt, nil)
	}
	return goja.Undefined()
}

func (s *ReaderStream) readOnce() {
	buffer := make([]byte, s.config.chunkSize)
	n, readErr := s.reader.Read(buffer)
	if readErr != nil {
		// Terminal read: close off the loop. A close failure at end of input
		// replaces EOF so it surfaces to the consumer.
		if closeErr := s.reader.Close(); closeErr != nil && errors.Is(readErr, io.EOF) {
			readErr = closeErr
		}
	}
	if s.sched == nil {
		s.deliver(buffer, n, readErr)
		return
	}
	if s.sched.RunOnLoop(func(*goja.Runtime) {
		s.deliver(buffer, n, readErr)
	}) {
		return
	}
	s.abandon(readErr)
}

// deliver runs on the loop (or inline for nil schedulers) with one result.
func (s *ReaderStream) deliver(buffer []byte, n int, readErr error) {
	s.mu.Lock()
	if s.settled {
		s.mu.Unlock()
		return
	}
	terminal := readErr != nil
	if terminal {
		s.settled = true
	}
	settle := s.settle
	s.settle = nil
	controller := s.controller
	s.mu.Unlock()

	if controller == nil {
		if settle != nil {
			_ = settle(goja.Undefined())
		}
		return
	}
	rt := s.rt
	if n > 0 {
		chunk := make([]byte, n)
		copy(chunk, buffer[:n])
		callStreamController(rt, controller, "enqueue", s.config.chunkValue(rt, chunk))
	}
	switch {
	case terminal && errors.Is(readErr, io.EOF):
		callStreamController(rt, controller, "close")
	case terminal:
		callStreamController(rt, controller, "error", s.config.mapError(rt, readErr))
	}
	if settle != nil {
		_ = settle(goja.Undefined())
	}
	if terminal && s.config.onSettled != nil {
		s.config.onSettled(rt, readErr)
	}
}

// abandon runs when the loop is gone: mark settled, close the reader, and
// run the off-loop settle hook.
func (s *ReaderStream) abandon(readErr error) {
	s.mu.Lock()
	if s.settled {
		s.mu.Unlock()
		return
	}
	s.settled = true
	s.settle = nil
	s.mu.Unlock()

	_ = s.reader.Close()
	if s.config.onSettledOff != nil {
		s.config.onSettledOff(readErr)
	}
}

func callStreamController(rt *goja.Runtime, controller *goja.Object, name string, args ...goja.Value) {
	method, ok := goja.AssertFunction(controller.Get(name))
	if !ok {
		panic(rt.NewTypeError("ReadableStream controller method %s is not callable", name))
	}
	if _, err := method(controller, args...); err != nil {
		panic(err)
	}
}
```

- [x] **Step 4: 运行核心测试确认通过**

Run: `go test ./streams/ -run TestReaderStream -v`
Expected: 4 个测试 PASS（无 race：可加 `-race` 再跑一遍）。

Run: `go test -race ./streams/ -run TestReaderStream`
Expected: PASS。

- [x] **Step 5: 追加边界测试（取消、外部 Error、chunk 选项、nil 调度器、loop 死亡、构造失败）**

在 `streams/reader_test.go` 末尾追加：

```go
func TestReaderStreamCancelClosesReader(t *testing.T) {
	loop := newTestLoop(t)
	body := newGatedReadCloser()
	setup := func(vm *goja.Runtime) error {
		source, err := streams.NewReadableStreamFromReader(vm, loop, body)
		if err != nil {
			return err
		}
		return vm.Set("__s", source.Stream())
	}
	result := runStreamsScript(t, loop, setup, `
		(async () => {
			const reader = __s.getReader();
			const pending = reader.read();
			await reader.cancel("stop");
			done("cancelled");
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	if result != "cancelled" {
		t.Fatalf("cancel result = %q", result)
	}
	select {
	case <-body.closed:
	case <-time.After(time.Second):
		t.Fatal("cancel did not close the reader")
	}
}

func TestReaderStreamErrorRejectsPendingReadWithExactReason(t *testing.T) {
	loop := newTestLoop(t)
	body := newGatedReadCloser()
	settledCount := atomic.Int32{}
	setup := func(vm *goja.Runtime) error {
		source, err := streams.NewReadableStreamFromReader(vm, loop, body,
			streams.WithOnSettled(func(*goja.Runtime, error) { settledCount.Add(1) }),
		)
		if err != nil {
			return err
		}
		if err := vm.Set("__error", func(call goja.FunctionCall) goja.Value {
			source.Error(vm, call.Argument(0))
			return goja.Undefined()
		}); err != nil {
			return err
		}
		return vm.Set("__s", source.Stream())
	}
	result := runStreamsScript(t, loop, setup, `
		(async () => {
			const reason = { marker: "exact" };
			const pending = __s.getReader().read();
			globalThis.__readPending = true;
			__error(reason);
			try {
				await pending;
				done("resolved");
			} catch (error) {
				done(String(error === reason));
			}
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	if result != "true" {
		t.Fatalf("abort identity = %q", result)
	}
	select {
	case <-body.closed:
	case <-time.After(time.Second):
		t.Fatal("Error did not close the reader")
	}
	if got := settledCount.Load(); got != 1 {
		t.Fatalf("OnSettled calls = %d, want 1", got)
	}
}

func TestReaderStreamChunkSizeAndValueOptions(t *testing.T) {
	loop := newTestLoop(t)
	setup := func(vm *goja.Runtime) error {
		source, err := streams.NewReadableStreamFromReader(
			vm, loop, io.NopCloser(strings.NewReader("hello")),
			streams.WithChunkSize(2),
			streams.WithChunkValue(streams.ArrayBufferChunk),
		)
		if err != nil {
			return err
		}
		return vm.Set("__s", source.Stream())
	}
	result := runStreamsScript(t, loop, setup, `
		(async () => {
			const reader = __s.getReader();
			const lengths = [];
			let isBuffer = true;
			while (true) {
				const item = await reader.read();
				if (item.done) break;
				isBuffer = isBuffer && item.value instanceof ArrayBuffer;
				lengths.push(item.value.byteLength);
			}
			done(lengths.join(",") + "|" + isBuffer);
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	if result != "2,2,1|true" {
		t.Fatalf("chunk result = %q", result)
	}
}

func TestReaderStreamNilSchedulerDeliversSynchronously(t *testing.T) {
	rt := goja.New()
	body := &countingReadCloser{reader: strings.NewReader("sync")}
	source, err := streams.NewReadableStreamFromReader(rt, nil, body)
	if err != nil {
		t.Fatal(err)
	}
	var consumed strings.Builder
	promise, err := streams.ConsumeReadableStream(rt, source.Stream(), func(chunk goja.Value) goja.Value {
		consumed.WriteString(string(chunk.Export().([]byte)))
		return goja.Undefined()
	})
	if err != nil {
		t.Fatal(err)
	}
	if promise.State() != goja.PromiseStateFulfilled {
		t.Fatalf("consume state = %v, result = %v", promise.State(), promise.Result())
	}
	if consumed.String() != "sync" {
		t.Fatalf("consumed = %q", consumed.String())
	}
	if got := body.closeCount.Load(); got != 1 {
		t.Fatalf("reader Close calls = %d, want 1", got)
	}
}

func TestReaderStreamSettlesOffLoopWhenLoopIsGone(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	body := newGatedReadCloser()
	offLoopErrs := make(chan error, 1)
	setupDone := make(chan error, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		_, err := streams.NewReadableStreamFromReader(vm, loop, body,
			streams.WithOnSettledOffLoop(func(err error) {
				select {
				case offLoopErrs <- err:
				default:
				}
			}),
		)
		_, _ = vm.RunString(`__s.getReader().read()`)
		setupDone <- err
	})
	if err := <-setupDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-body.readStarted:
	case <-time.After(time.Second):
		t.Fatal("read did not start")
	}

	loop.Stop()
	body.steps <- stepRead{err: io.EOF}

	select {
	case err := <-offLoopErrs:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("off-loop settle err = %v, want io.EOF", err)
		}
	case <-time.After(time.Second):
		t.Fatal("off-loop settle hook was not called")
	}
	select {
	case <-body.closed:
	case <-time.After(time.Second):
		t.Fatal("loop-dead path did not close the reader")
	}
}

func TestReaderStreamConstructionFailureClosesReader(t *testing.T) {
	rt := goja.New()
	if err := streams.Exports(rt).Set("ReadableStream", 1); err != nil {
		t.Fatal(err)
	}
	body := &countingReadCloser{reader: strings.NewReader("x")}
	settled := make(chan struct{}, 1)
	_, err := streams.NewReadableStreamFromReader(rt, nil, body,
		streams.WithOnSettled(func(*goja.Runtime, error) {
			select {
			case settled <- struct{}{}:
			default:
			}
		}),
	)
	if err == nil {
		t.Fatal("construction unexpectedly succeeded")
	}
	if got := body.closeCount.Load(); got != 1 {
		t.Fatalf("reader Close calls = %d, want 1", got)
	}
	select {
	case <-settled:
	default:
		t.Fatal("OnSettled was not called on construction failure")
	}
}
```

- [x] **Step 6: 运行全部读桥测试**

Run: `go test -race ./streams/ -run TestReaderStream`
Expected: 全部 PASS。

- [x] **Step 7: 提交**

```bash
git add streams/reader.go streams/reader_test.go
git commit -m "feat(streams): 新增 io.ReadCloser 到 ReadableStream 的规范桥"
```

### Task 2: streams 字节流桥 NewReadableStreamFromBytes（TDD）

**Files:**
- Modify: `streams/reader.go`
- Test: `streams/reader_test.go`

- [x] **Step 1: 写失败测试**

在 `streams/reader_test.go` 追加：

```go
func TestReaderFromBytesChunksCloseAndCancel(t *testing.T) {
	rt := goja.New()
	stream, err := streams.NewReadableStreamFromBytes(rt, []byte("hello"), 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Set("__s", stream); err != nil {
		t.Fatal(err)
	}
	value, err := rt.RunString(`
		(async () => {
			const reader = __s.getReader();
			const first = await reader.read();
			await reader.cancel("stop");
			return JSON.stringify([
				String.fromCharCode(...first.value),
				first.value instanceof Uint8Array,
				first.value.byteLength,
			]);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	promise := value.Export().(*goja.Promise)
	if promise.State() != goja.PromiseStateFulfilled {
		t.Fatalf("state = %v, result = %v", promise.State(), promise.Result())
	}
	if got, want := promise.Result().String(), `["he",true,2]`; got != want {
		t.Fatalf("result = %s, want %s", got, want)
	}
}
```

- [x] **Step 2: 运行确认编译失败**

Run: `go test ./streams/ -run TestReaderFromBytes`
Expected: FAIL，`undefined: streams.NewReadableStreamFromBytes`。

- [x] **Step 3: 实现**

在 `streams/reader.go` 末尾追加：

```go
// NewReadableStreamFromBytes returns a ReadableStream that delivers data in
// fixed-size chunks. Pulls are satisfied synchronously, so no scheduler is
// required; data is copied per chunk.
func NewReadableStreamFromBytes(
	rt *goja.Runtime,
	data []byte,
	chunkSize int,
	opts ...ReaderStreamOption,
) (*goja.Object, error) {
	config := newReaderConfig()
	config.chunkSize = chunkSize
	config.apply(opts)

	offset := 0
	return NewReadableStream(rt, ReadableStreamSource{
		Pull: func(controller *goja.Object) goja.Value {
			if offset >= len(data) {
				callStreamController(rt, controller, "close")
				return goja.Undefined()
			}
			end := min(offset+config.chunkSize, len(data))
			chunk := append([]byte(nil), data[offset:end]...)
			offset = end
			callStreamController(rt, controller, "enqueue", config.chunkValue(rt, chunk))
			return goja.Undefined()
		},
		Cancel: func(goja.Value) goja.Value {
			offset = len(data)
			return goja.Undefined()
		},
	})
}
```

- [x] **Step 4: 运行确认通过**

Run: `go test -race ./streams/`
Expected: 全部 PASS。

- [x] **Step 5: 提交**

```bash
git add streams/reader.go streams/reader_test.go
git commit -m "feat(streams): 新增 []byte 分块 ReadableStream 桥"
```

### Task 3: streams 写侧桥 NewWritableStreamToWriter（TDD）

**Files:**
- Create: `streams/writer.go`
- Test: `streams/writer_test.go`

- [x] **Step 1: 写失败测试**

创建 `streams/writer_test.go`：

```go
package streams_test

import (
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/streams"
)

type recordingWriter struct {
	mu         sync.Mutex
	builder    strings.Builder
	failWrites bool
	closeCount atomic.Int32
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failWrites {
		return 0, errors.New("write exploded")
	}
	return w.builder.Write(p)
}

func (w *recordingWriter) Close() error {
	w.closeCount.Add(1)
	return nil
}

func (w *recordingWriter) written() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.builder.String()
}

func TestWriterStreamWritesChunksAndCloses(t *testing.T) {
	loop := newTestLoop(t)
	writer := &recordingWriter{}
	setup := func(vm *goja.Runtime) error {
		stream, err := streams.NewWritableStreamToWriter(vm, loop, writer)
		if err != nil {
			return err
		}
		return vm.Set("__w", stream)
	}
	result := runStreamsScript(t, loop, setup, `
		(async () => {
			const writer = __w.getWriter();
			await writer.write(new Uint8Array([65]));
			await writer.write(new Uint8Array([66]));
			await writer.close();
			done("closed");
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	if result != "closed" {
		t.Fatalf("write result = %q", result)
	}
	if got := writer.written(); got != "AB" {
		t.Fatalf("written = %q", got)
	}
	if got := writer.closeCount.Load(); got != 1 {
		t.Fatalf("writer Close calls = %d, want 1", got)
	}
}

func TestWriterStreamRejectsInvalidChunkAndCloses(t *testing.T) {
	loop := newTestLoop(t)
	writer := &recordingWriter{}
	setup := func(vm *goja.Runtime) error {
		stream, err := streams.NewWritableStreamToWriter(vm, loop, writer)
		if err != nil {
			return err
		}
		return vm.Set("__w", stream)
	}
	result := runStreamsScript(t, loop, setup, `
		(async () => {
			try {
				await __w.getWriter().write(42);
				done("resolved");
			} catch (error) {
				done(String(error instanceof TypeError));
			}
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	if result != "true" {
		t.Fatalf("invalid chunk result = %q", result)
	}
	if got := writer.closeCount.Load(); got != 1 {
		t.Fatalf("writer Close calls = %d, want 1", got)
	}
}

func TestWriterStreamRejectsWriteFailureAndCloses(t *testing.T) {
	loop := newTestLoop(t)
	writer := &recordingWriter{failWrites: true}
	setup := func(vm *goja.Runtime) error {
		stream, err := streams.NewWritableStreamToWriter(vm, loop, writer)
		if err != nil {
			return err
		}
		return vm.Set("__w", stream)
	}
	result := runStreamsScript(t, loop, setup, `
		(async () => {
			try {
				await __w.getWriter().write(new Uint8Array([65]));
				done("resolved");
			} catch (error) {
				done(String(error.message).includes("write exploded"));
			}
		})().catch((error) => fail(String(error && error.stack || error)));
	`)
	if result != "true" {
		t.Fatalf("write failure result = %q", result)
	}
	if got := writer.closeCount.Load(); got != 1 {
		t.Fatalf("writer Close calls = %d, want 1", got)
	}
}

func TestWriterStreamAbortCloses(t *testing.T) {
	loop := newTestLoop(t)
	writer := &recordingWriter{}
	setup := func(vm *goja.Runtime) error {
		stream, err := streams.NewWritableStreamToWriter(vm, loop, writer)
		if err != nil {
			return err
		}
		return vm.Set("__w", stream)
	}
	result := runStreamsScript(t, loop, setup, `
		__w.getWriter().abort("stop").then(() => done("aborted"));
	`)
	if result != "aborted" {
		t.Fatalf("abort result = %q", result)
	}
	if got := writer.closeCount.Load(); got != 1 {
		t.Fatalf("writer Close calls = %d, want 1", got)
	}
}

func TestWriterStreamNilScheduler(t *testing.T) {
	rt := goja.New()
	writer := &recordingWriter{}
	stream, err := streams.NewWritableStreamToWriter(rt, nil, writer)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Set("__w", stream); err != nil {
		t.Fatal(err)
	}
	value, err := rt.RunString(`
		(function () {
			const writer = __w.getWriter();
			return writer.write(new Uint8Array([65]))
				.then(() => writer.write(new Uint8Array([66])))
				.then(() => writer.close());
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	promise := value.Export().(*goja.Promise)
	if promise.State() != goja.PromiseStateFulfilled {
		t.Fatalf("state = %v, result = %v", promise.State(), promise.Result())
	}
	if got := writer.written(); got != "AB" {
		t.Fatalf("written = %q", got)
	}
	var _ io.WriteCloser = writer
}
```

- [x] **Step 2: 运行确认编译失败**

Run: `go test ./streams/ -run TestWriterStream`
Expected: FAIL，`undefined: streams.NewWritableStreamToWriter`。

- [x] **Step 3: 实现 streams/writer.go**

```go
package streams

import (
	"errors"
	"io"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/runtimehost"
)

// WriterOption configures NewWritableStreamToWriter.
type WriterOption func(*writerConfig)

type writerConfig struct {
	decodeChunk func(rt *goja.Runtime, chunk goja.Value) ([]byte, error)
	mapError    func(rt *goja.Runtime, err error) goja.Value
}

// WithDecodeChunk overrides how JavaScript chunks are decoded to bytes. The
// default accepts ArrayBuffer and ArrayBufferView values.
func WithDecodeChunk(fn func(rt *goja.Runtime, chunk goja.Value) ([]byte, error)) WriterOption {
	return func(config *writerConfig) { config.decodeChunk = fn }
}

// WithMapWriteError overrides how decode and write failures become the
// rejected write reason.
func WithMapWriteError(fn func(rt *goja.Runtime, err error) goja.Value) WriterOption {
	return func(config *writerConfig) { config.mapError = fn }
}

// NewWritableStreamToWriter returns a WritableStream that decodes chunks on
// the loop, writes them on a background goroutine (inline when scheduler is
// nil), and closes the writer on close, abort, and any failure.
func NewWritableStreamToWriter(
	rt *goja.Runtime,
	scheduler runtimehost.Scheduler,
	writer io.WriteCloser,
	opts ...WriterOption,
) (*goja.Object, error) {
	if writer == nil {
		return nil, errors.New("streams: writer is required")
	}
	config := writerConfig{
		decodeChunk: decodeChunkBytes,
		mapError: func(rt *goja.Runtime, err error) goja.Value {
			return errorValue(rt, err)
		},
	}
	for _, opt := range opts {
		opt(&config)
	}

	run := func(op func() error) goja.Value {
		promise, resolve, reject := rt.NewPromise()
		exec := func() {
			err := op()
			settle := func(rt *goja.Runtime) {
				if err != nil {
					_ = reject(config.mapError(rt, err))
					return
				}
				_ = resolve(goja.Undefined())
			}
			if scheduler == nil {
				settle(rt)
				return
			}
			_ = scheduler.RunOnLoop(settle)
		}
		if scheduler == nil {
			exec()
		} else {
			go exec()
		}
		return rt.ToValue(promise)
	}

	return NewWritableStream(rt, WritableStreamSink{
		Write: func(chunk goja.Value, _ *goja.Object) goja.Value {
			data, decodeErr := config.decodeChunk(rt, chunk)
			if decodeErr != nil {
				return run(func() error {
					_ = writer.Close()
					return decodeErr
				})
			}
			return run(func() error {
				if err := writeAll(writer, data); err != nil {
					_ = writer.Close()
					return err
				}
				return nil
			})
		},
		Close: func() goja.Value {
			return run(writer.Close)
		},
		Abort: func(reason goja.Value) goja.Value {
			_ = reason
			return run(writer.Close)
		},
	})
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func decodeChunkBytes(rt *goja.Runtime, chunk goja.Value) ([]byte, error) {
	if chunk == nil || goja.IsUndefined(chunk) || goja.IsNull(chunk) {
		return nil, rt.NewTypeError("streams: chunk must be an ArrayBuffer or ArrayBufferView")
	}
	switch data := chunk.Export().(type) {
	case []byte:
		return append([]byte(nil), data...), nil
	case goja.ArrayBuffer:
		return append([]byte(nil), data.Bytes()...), nil
	}
	return nil, rt.NewTypeError("streams: chunk must be an ArrayBuffer or ArrayBufferView")
}
```

- [x] **Step 4: 运行确认通过**

Run: `go test -race ./streams/`
Expected: 全部 PASS。

- [x] **Step 5: 提交**

```bash
git add streams/writer.go streams/writer_test.go
git commit -m "feat(streams): 新增 WritableStream 到 io.WriteCloser 的规范桥"
```

### Task 4: cloudflarekv 迁移到读桥

**Files:**
- Modify: `cloudflarekv/stream_bridge.go`（删 `storageStreamSource` 及其 `close`；重写 `streamRecordToReadableStream`）
- Modify: `cloudflarekv/bridge.go:817-846`（`bytesToReadableStreamValue` 委托）

- [x] **Step 1: 重写 streamRecordToReadableStream**

`cloudflarekv/stream_bridge.go`：删除 `storageStreamSource` 结构体（第 17-21 行）与 `(s *storageStreamSource) close()`（第 127-129 行），并把 `streamRecordToReadableStream`（第 131-196 行）替换为：

```go
func streamRecordToReadableStream(
	vm *goja.Runtime,
	loop *eventloop.EventLoop,
	record store.StreamRecord,
	maximumBytes int64,
) (goja.Value, error) {
	if record.Body == nil {
		return nil, errors.New("cloudflarekv: stream record body is nil")
	}
	if maximumBytes > 0 && record.Size >= 0 && record.Size > maximumBytes {
		_ = record.Body.Close()
		return nil, fmt.Errorf("KV value exceeds the maximum size of %d bytes", maximumBytes)
	}
	body := record.Body
	if maximumBytes > 0 {
		body = &maximumReadCloser{
			maximumReader: &maximumReader{reader: record.Body, remaining: maximumBytes, maximum: maximumBytes},
			closer:        record.Body,
		}
	}
	stream, err := streams.NewReadableStreamFromReader(
		vm,
		loop,
		body,
		streams.WithChunkSize(readableStreamChunkSize),
		streams.WithChunkValue(streams.ArrayBufferChunk),
		streams.WithMapError(func(rt *goja.Runtime, err error) goja.Value {
			return rt.ToValue(err.Error())
		}),
	)
	if err != nil {
		return nil, err
	}
	return stream.Stream(), nil
}
```

随后检查 import：`sync` 若仅剩 `storageStreamSource` 使用则移除。

- [x] **Step 2: 重写 bytesToReadableStreamValue**

`cloudflarekv/bridge.go:817-846` 替换为：

```go
func bytesToReadableStreamValue(vm *goja.Runtime, value []byte) (goja.Value, error) {
	stream, err := streams.NewReadableStreamFromBytes(
		vm,
		value,
		readableStreamChunkSize,
		streams.WithChunkValue(streams.ArrayBufferChunk),
	)
	if err != nil {
		return nil, err
	}
	return stream, nil
}
```

- [x] **Step 3: 编译并跑 kv 全部测试**

Run: `go test -race ./cloudflarekv/`
Expected: 全部 PASS（ArrayBuffer chunk 形态由 `WithChunkValue` 保持，测试断言 `new Uint8Array(result.value)` 不受影响）。

- [x] **Step 4: 提交**

```bash
git add cloudflarekv/stream_bridge.go cloudflarekv/bridge.go
git commit -m "refactor(cloudflarekv): KV 流生产者委托 streams 规范读桥"
```

### Task 5: fs 迁移到读/写桥

**Files:**
- Modify: `fs/streams.go:11-96`（重写两个构造器；删 `fileStreamReadResult`）

- [x] **Step 1: 重写 fs/streams.go 头部构造器**

把 `fs/streams.go` 第 11-96 行（`fileStreamReadResult`、`newFileReadableStream`、`newFileWritableStream`）替换为：

```go
type fileHandleReadCloser struct {
	handle *FileHandle
}

func (r *fileHandleReadCloser) Read(p []byte) (int, error) { return r.handle.Read(p) }

// Close 阻塞等待在途读完成后物理关闭，只能在后台 goroutine 调用。
func (r *fileHandleReadCloser) Close() error { return r.handle.closeAndWait() }

type fileHandleWriteCloser struct {
	handle *FileHandle
}

func (w *fileHandleWriteCloser) Write(p []byte) (int, error) {
	if err := w.handle.WriteAll(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Close 阻塞等待在途写完成后物理关闭，只能在后台 goroutine 调用。
func (w *fileHandleWriteCloser) Close() error { return w.handle.closeAndWait() }

func newFileReadableStream(instance *moduleInstance, handle *FileHandle) goja.Value {
	rt := instance.rt
	stream, err := streams.NewReadableStreamFromReader(
		rt,
		instance.scheduler,
		&fileHandleReadCloser{handle: handle},
		streams.WithChunkSize(instance.core.ChunkSize()),
		streams.WithMapError(jsErrorValue),
		streams.WithOnCancel(func(rt *goja.Runtime, reason goja.Value) goja.Value {
			_ = reason
			return instance.promiseCall(func() (any, error) {
				return nil, handle.closeAndWait()
			}, nil)
		}),
	)
	if err != nil {
		panicJSError(rt, err)
	}
	return stream.Stream()
}

func newFileWritableStream(instance *moduleInstance, handle *FileHandle) goja.Value {
	rt := instance.rt
	stream, err := streams.NewWritableStreamToWriter(
		rt,
		instance.scheduler,
		&fileHandleWriteCloser{handle: handle},
		streams.WithDecodeChunk(bytesFromValue),
		streams.WithMapWriteError(jsErrorValue),
	)
	if err != nil {
		panicJSError(rt, err)
	}
	return stream
}
```

注意：`WithOnCancel` 返回异步 promise（`closeAndWait` 在 goroutine 完成），桥的默认 on-loop 关闭被跳过，`FsFile.close` 不被阻塞；EOF/读错路径的 `closeAndWait` 由桥在后台 goroutine 调 `fileHandleReadCloser.Close` 完成。

- [x] **Step 2: 清理孤儿符号**

检查 `fs/streams.go` 中 `callController` 与 `fs/api.go` 中 `bytesValue` 是否仍被其他位置引用：

Run: `rg -n "callController\(|bytesValue\(" fs/`
若已无引用则删除对应函数（`fs/streams.go:234-242` 的 `callController`；`fs/api.go:754-761` 的 `bytesValue`），有引用则保留。

- [x] **Step 3: 跑 fs 全部测试**

Run: `go test -race ./fs/`
Expected: 全部 PASS（重点：`TestFsFileReadableStreamReadsFile`、`TestFsFileReadableClosesOnEOF/OnCancel`、`TestFsFileReadableUsesConfiguredChunkSize`（2,2,1）、`TestFsFileWritable*`、`TestFsFileCloseDoesNotBlockOnInFlightRead`）。

- [x] **Step 4: 提交**

```bash
git add fs/
git commit -m "refactor(fs): 文件读写流委托 streams 规范 io 桥"
```

### Task 6: fetch 迁移并删除 streamingBody

**Files:**
- Modify: `fetch/stream.go`（整文件重写为薄委托）
- Modify: `fetch/stream_regression_test.go`（删机制测试、加回归测试）
- Modify: `fetch/stream_benchmark_test.go`（删 `BenchmarkStreamingBodyPump1MiB`）

- [x] **Step 1: 重写 fetch/stream.go**

将 `fetch/stream.go` 全文替换为（保留仍被引用的辅助函数；先 `rg -n "bytesValue\(|callFetchController\(" fetch/` 确认引用情况，`bytesValue` 被 benchmark 与其他文件引用则保留，`callFetchController` 若仅旧实现使用则删）：

```go
package fetch

import (
	"io"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/runtimehost"
	"github.com/dop251/goja_nodejs/streams"
)

// fetchReadableStream builds a canonical ReadableStream that streams the given
// HTTP body. Reads run one per pull on background goroutines, so the loop
// thread never blocks and backpressure follows the stream contract exactly.
func fetchReadableStream(
	rt *goja.Runtime,
	scheduler runtimehost.Scheduler,
	body io.ReadCloser,
	cleanup func(),
	cleanupOffLoop func(),
	abort *dispatchAbortState,
) (*goja.Object, error) {
	reader, err := streams.NewReadableStreamFromReader(
		rt,
		scheduler,
		body,
		streams.WithMapError(func(rt *goja.Runtime, err error) goja.Value {
			return fetchNetworkError(rt, rt.NewGoError(err))
		}),
		streams.WithOnSettled(func(*goja.Runtime, error) { cleanup() }),
		streams.WithOnSettledOffLoop(func(error) { cleanupOffLoop() }),
	)
	if err != nil {
		return nil, err
	}
	if abort != nil {
		abort.setHandler(func(reason goja.Value) {
			reader.Error(rt, reason)
		})
	}
	return reader.Stream(), nil
}

func fetchNetworkError(rt *goja.Runtime, cause goja.Value) goja.Value {
	fetchError := Exports(rt).Get("_FetchError").ToObject(rt)
	networkError, ok := goja.AssertFunction(fetchError.Get("NETWORK_ERROR"))
	if !ok {
		return cause
	}
	value, err := networkError(fetchError, rt.ToValue("Network error"), cause)
	if err != nil {
		return cause
	}
	return value
}

func bytesValue(rt *goja.Runtime, data []byte) goja.Value {
	// NewArrayBuffer keeps data as its backing store; callers transfer ownership.
	arrayBuffer := rt.NewArrayBuffer(data)
	typed, err := rt.New(rt.Get("Uint8Array"), rt.ToValue(arrayBuffer))
	if err != nil {
		panic(err)
	}
	return typed
}
```

若 `bytesValue` 无其他引用则一并删除（benchmark 删除后仍被 `fetch_test.go` 等引用时必须保留，以 Step 1 开头的 rg 结果为准）。

- [x] **Step 2: 清理 fetch/stream_regression_test.go**

删除以下仅测 streamingBody 内部机制的测试与辅助（行号以当前文件为准）：
- `TestStreamingBodySchedulerRejectionStopsPump`（约 83-118 行）
- `TestStreamingBodyTerminalSchedulerRejectionRunsOffLoopCleanup`（约 120-149 行）
- `TestStreamingBodyBackpressureAndCancellation` 及其全部子测试（约 355-478 行）
- `waitQueueLength`、`takeQueuedChunk`（约 705-735 行）
- `immediateScheduler`（约 79-82、151-158 行）——先 `rg -n "immediateScheduler" fetch/` 确认仅被将删除的 benchmark 引用后再删。

保留：`controlledReadCloser`、`contextReadCloser`、`errControlledBodyClosed`、`responseClient`、`awaitSignal` 以及所有 `TestFetchStream*` 公共行为测试（DeliversBytesReturnedWithEOF…、DeliversBytesBeforeSeparateEOF、WrapsBodyReadFailureAsNetworkError、AbortPreservesExactReason、PullDoesNotBlockLoop、ConstructionFailureRejectsAndClosesBody 等）与信号测试。

在文件末尾追加新的回归测试（针对本次修复的超前读取/突发交付 bug）：

```go
func TestFetchStreamDoesNotReadAheadOfConsumer(t *testing.T) {
	body := newControlledReadCloser(0)
	loop := startFetchLoop(t)
	runSync(t, loop, func(rt *goja.Runtime) {
		Enable(rt)
		if err := EnableFetch(rt, loop, WithHTTPClient(responseClient(body))); err != nil {
			t.Fatal(err)
		}
		_, err := rt.RunString(`
			globalThis.__done = false
			;(async () => {
				const reader = (await fetch("http://stream.test/backpressure")).body.getReader()
				const pending = reader.read()
				globalThis.__readPending = true
				const item = await pending
				globalThis.__chunk = String.fromCharCode(...item.value)
				await reader.cancel("test complete")
			})().catch((error) => {
				globalThis.__err = String(error && error.stack || error)
			}).finally(() => { globalThis.__done = true })
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
	waitBool(t, loop, "__readPending")
	awaitSignal(t, body.readStarted, "first body Read")
	select {
	case <-body.readStarted:
		t.Fatal("body was read ahead of consumer demand")
	case <-time.After(30 * time.Millisecond):
	}
	body.reads <- controlledRead{data: []byte("x")}
	waitBool(t, loop, "__done")
	if got := gstr(t, loop, "__err"); got != "" {
		t.Fatalf("stream rejected: %s", got)
	}
	if got := gstr(t, loop, "__chunk"); got != "x" {
		t.Fatalf("chunk = %q, want %q", got, "x")
	}
	if got := body.closeCount.Load(); got != 1 {
		t.Fatalf("body Close called %d times", got)
	}
}
```

- [x] **Step 3: 清理 fetch/stream_benchmark_test.go**

删除 `BenchmarkStreamingBodyPump1MiB`（引用已删除的 `newStreamingBody`）；保留 `BenchmarkResponseBytesValue64KiB`（`bytesValue` 保留的依据）。若删除后 `immediateScheduler`、`io`、`bytes` import 无引用则一并清理。

- [x] **Step 4: 在 streams 包补吞吐基准（替代被删基准）**

在 `streams/reader_test.go` 追加：

```go
func BenchmarkReaderStreamPump1MiB(b *testing.B) {
	payload := make([]byte, 1024*1024)
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		finished := make(chan struct{})
		loop.RunOnLoop(func(vm *goja.Runtime) {
			_ = vm.Set("__benchDone", func(goja.FunctionCall) goja.Value {
				close(finished)
				return goja.Undefined()
			})
			source, err := streams.NewReadableStreamFromReader(vm, loop, io.NopCloser(bytes.NewReader(payload)))
			if err != nil {
				b.Fatal(err)
			}
			_ = vm.Set("__s", source.Stream())
			_, err = vm.RunString(`
				(async () => {
					const reader = __s.getReader();
					while (!(await reader.read()).done) {}
				})().then(() => __benchDone());
			`)
			if err != nil {
				b.Fatal(err)
			}
		})
		select {
		case <-finished:
		case <-time.After(30 * time.Second):
			b.Fatal("timeout waiting for pump completion")
		}
	}
}
```

并在 `streams/reader_test.go` 的 import 中加入 `"bytes"`。

- [x] **Step 5: 跑 fetch 全部测试**

Run: `go test -race ./fetch/`
Expected: 全部 PASS。若 `TestFetchStreamConstructionFailureRejectsAndClosesBody` 失败，核对桥构造失败路径是否调用了 `OnSettled`（应调用 `cleanup` → 移除 JS abort listener）。

- [x] **Step 6: 提交**

```bash
git add fetch/ streams/reader_test.go
git commit -m "fix(fetch): 消除响应体超前读取与突发交付，委托 streams 规范读桥"
```

### Task 7: 全量验证与收尾

- [x] **Step 1: 全量测试（含 race）**

Run: `go test ./... && go test -race ./streams/ ./fetch/ ./fs/ ./cloudflarekv/`
Expected: 全部 PASS。

- [x] **Step 2: 格式与静态检查（对齐 CI）**

Run: `gofmt -l . && go vet ./...`
Expected: `gofmt` 无输出，`vet` 无告警。

- [x] **Step 3: 确认生产者已收敛**

Run: `rg -n "NewReadableStream\(" --glob '!streams/*' -g '*.go' .`
Expected: 除 `streams/` 包自身与测试外无匹配（消费者全部经由 `NewReadableStreamFromReader`/`NewReadableStreamFromBytes`）。

- [x] **Step 4: 若前三步产生改动则提交**

```bash
git add -A
git commit -m "chore(streams): io 桥收敛后的静态检查清理"
```
（无改动则跳过。）

---

## Self-Review 结论

- 覆盖检查：4 个生产者全部迁移（fs/cloudflarekv×2/fetch）+ 写侧 `NewWritableStreamToWriter`（fs 使用）；fetch 的 abort/cleanupOffLoop/构造失败/精确 reason 等 6 个公共行为测试保留；新增超前读取回归测试（streams 层 + fetch 层各一）。
- 类型一致性：`ReaderStreamOption`/`WriterOption` 命名不冲突；`WithMapError`（读）与 `WithMapWriteError`（写）区分；`Uint8ArrayChunk`/`ArrayBufferChunk` 为导出辅助。
- 风险与回退：每个任务独立提交，任一步测试红可 `git revert` 单个提交；cloudflarekv 对外 chunk 形态（ArrayBuffer）与错误字符串映射通过选项原样保留。
