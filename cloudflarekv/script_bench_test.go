package cloudflarekv_test

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/cloudflarekv"
	"github.com/sealdice/goja_ext/eventloop"
)

func BenchmarkSyncKVScript(b *testing.B) {
	for _, scenario := range benchmarkScenarios() {
		b.Run(scenario.name, func(b *testing.B) {
			vm, runScenario := newScriptBenchmarkVM(b, scenario)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				result, err := runScenario(goja.Undefined())
				if err != nil {
					b.Fatalf("run scenario: %v", err)
				}
				if got := result.ToInteger(); got != scenario.expectedResult {
					b.Fatalf("unexpected scenario result: got %d want %d", got, scenario.expectedResult)
				}
			}
			_ = vm
		})
	}
}

func TestAsyncKVScriptScenarioRuns(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	scenario := benchmarkScenarios()[0]
	runScenario := newAsyncScriptRunner(t, loop, scenario)

	got, err := runScenario()
	if err != nil {
		t.Fatalf("run async scenario: %v", err)
	}
	if got != scenario.expectedResult {
		t.Fatalf("unexpected async scenario result: got %d want %d", got, scenario.expectedResult)
	}
}

func BenchmarkAsyncKVScript(b *testing.B) {
	for _, scenario := range benchmarkScenarios() {
		b.Run(scenario.name, func(b *testing.B) {
			loop := eventloop.NewEventLoop()
			loop.Start()
			defer loop.Stop()

			runScenario := newAsyncScriptRunner(b, loop, scenario)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				got, err := runScenario()
				if err != nil {
					b.Fatalf("run async scenario: %v", err)
				}
				if got != scenario.expectedResult {
					b.Fatalf("unexpected async scenario result: got %d want %d", got, scenario.expectedResult)
				}
			}
		})
	}
}

type scriptScenario struct {
	name           string
	payload        string
	script         string
	asyncScript    string
	expectedResult int64
}

func benchmarkScenarios() []scriptScenario {
	return []scriptScenario{
		{
			name:           "json_heavy",
			payload:        benchmarkJSONPayload(),
			expectedResult: 96,
			script: `
				function runScenario() {
					for (var i = 0; i < 12; i++) {
						SyncKV.put("bench:json:" + i, payloadText, {
							metadata: {kind: "json", slot: i},
							expirationTtl: 3600
						});
					}

					var total = 0;
					for (var i = 0; i < 12; i++) {
						var value = SyncKV.get("bench:json:" + i, "json");
						if (value === null || value.records === undefined) {
							throw new Error("missing json value");
						}
						total += value.records.length;
					}

					var page = SyncKV.list({prefix: "bench:json:", limit: 32});
					if (page.keys.length !== 12) {
						throw new Error("unexpected json key count: " + page.keys.length);
					}
					return total;
				}
			`,
			asyncScript: `
				function runScenarioAsync() {
					var writes = [];
					for (var i = 0; i < 12; i++) {
						writes.push(KV.put("bench:json:" + i, payloadText, {
							metadata: {kind: "json", slot: i},
							expirationTtl: 3600
						}));
					}

					return Promise.all(writes)
						.then(function () {
							var reads = [];
							for (var i = 0; i < 12; i++) {
								reads.push(KV.get("bench:json:" + i, "json"));
							}
							return Promise.all(reads);
						})
						.then(function (values) {
							var total = 0;
							for (var i = 0; i < values.length; i++) {
								if (values[i] === null || values[i].records === undefined) {
									throw new Error("missing json value");
								}
								total += values[i].records.length;
							}
							return KV.list({prefix: "bench:json:", limit: 32}).then(function (page) {
								if (page.keys.length !== 12) {
									throw new Error("unexpected json key count: " + page.keys.length);
								}
								return total;
							});
						});
				}
			`,
		},
		{
			name:           "base64_text",
			payload:        benchmarkBase64Payload(),
			expectedResult: 12,
			script: `
				function runScenario() {
					for (var i = 0; i < 12; i++) {
						SyncKV.put("bench:b64:" + i, payloadText, {
							metadata: {kind: "base64", slot: i},
							expirationTtl: 3600
						});
					}

					var total = 0;
					for (var i = 0; i < 12; i++) {
						var value = SyncKV.get("bench:b64:" + i);
						if (value === null || value.length !== payloadText.length) {
							throw new Error("missing base64 value");
						}
						total++;
					}

					var page = SyncKV.list({prefix: "bench:b64:", limit: 32});
					if (page.keys.length !== 12) {
						throw new Error("unexpected base64 key count: " + page.keys.length);
					}
					return total;
				}
			`,
			asyncScript: `
				function runScenarioAsync() {
					var writes = [];
					for (var i = 0; i < 12; i++) {
						writes.push(KV.put("bench:b64:" + i, payloadText, {
							metadata: {kind: "base64", slot: i},
							expirationTtl: 3600
						}));
					}

					return Promise.all(writes)
						.then(function () {
							var reads = [];
							for (var i = 0; i < 12; i++) {
								reads.push(KV.get("bench:b64:" + i));
							}
							return Promise.all(reads);
						})
						.then(function (values) {
							var total = 0;
							for (var i = 0; i < values.length; i++) {
								if (values[i] === null || values[i].length !== payloadText.length) {
									throw new Error("missing base64 value");
								}
								total++;
							}
							return KV.list({prefix: "bench:b64:", limit: 32}).then(function (page) {
								if (page.keys.length !== 12) {
									throw new Error("unexpected base64 key count: " + page.keys.length);
								}
								return total;
							});
						});
				}
			`,
		},
	}
}

func newScriptBenchmarkVM(b *testing.B, scenario scriptScenario) (*goja.Runtime, func(goja.Value, ...goja.Value) (goja.Value, error)) {
	b.Helper()

	mem := newMemStore()
	fixed := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	mem.now = func() time.Time { return fixed }

	vm := goja.New()
	if err := cloudflarekv.BindSyncNamespace(vm, "SyncKV", mem); err != nil {
		b.Fatalf("bind SyncKV: %v", err)
	}
	if err := vm.Set("payloadText", scenario.payload); err != nil {
		b.Fatalf("set payloadText: %v", err)
	}
	if _, err := vm.RunString(scenario.script); err != nil {
		b.Fatalf("compile benchmark script: %v", err)
	}

	runValue := vm.Get("runScenario")
	runScenario, ok := goja.AssertFunction(runValue)
	if !ok {
		b.Fatal("runScenario is not callable")
	}

	return vm, runScenario
}

func newAsyncScriptRunner(tb testing.TB, loop *eventloop.EventLoop, scenario scriptScenario) func() (int64, error) {
	tb.Helper()

	mem := newMemStore()
	fixed := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	mem.now = func() time.Time { return fixed }

	var runScenario func(goja.Value, ...goja.Value) (goja.Value, error)
	readyCh := make(chan struct{}, 1)
	errCh := make(chan error, 1)

	loop.RunOnLoop(func(vm *goja.Runtime) {
		if err := cloudflarekv.BindNamespace(vm, loop, "KV", mem); err != nil {
			errCh <- err
			return
		}
		if err := vm.Set("payloadText", scenario.payload); err != nil {
			errCh <- err
			return
		}
		if _, err := vm.RunString(scenario.asyncScript); err != nil {
			errCh <- err
			return
		}

		value := vm.Get("runScenarioAsync")
		fn, ok := goja.AssertFunction(value)
		if !ok {
			errCh <- errors.New("runScenarioAsync is not callable")
			return
		}
		runScenario = fn
		readyCh <- struct{}{}
	})

	select {
	case <-readyCh:
	case err := <-errCh:
		tb.Fatalf("prepare async scenario: %v", err)
	case <-time.After(5 * time.Second):
		tb.Fatal("timed out preparing async scenario")
	}

	return func() (int64, error) {
		doneCh := make(chan int64, 1)
		runErrCh := make(chan error, 1)

		loop.RunOnLoop(func(vm *goja.Runtime) {
			value, err := runScenario(goja.Undefined())
			if err != nil {
				runErrCh <- err
				return
			}

			promise := value.ToObject(vm)
			thenValue := promise.Get("then")
			thenFn, ok := goja.AssertFunction(thenValue)
			if !ok {
				runErrCh <- errors.New("promise.then is not callable")
				return
			}

			resolve := func(call goja.FunctionCall) goja.Value {
				doneCh <- call.Argument(0).ToInteger()
				return goja.Undefined()
			}
			reject := func(call goja.FunctionCall) goja.Value {
				runErrCh <- errors.New(call.Argument(0).String())
				return goja.Undefined()
			}

			if _, err := thenFn(promise, vm.ToValue(resolve), vm.ToValue(reject)); err != nil {
				runErrCh <- err
			}
		})

		select {
		case result := <-doneCh:
			return result, nil
		case err := <-runErrCh:
			return 0, err
		case <-time.After(10 * time.Second):
			return 0, errors.New("timed out waiting for async scenario")
		}
	}
}

func benchmarkJSONPayload() string {
	item := `{"name":"goja-kv","role":"worker","status":"active","description":"Compression benchmark payload for repetitive JSON text."}`
	parts := make([]string, 8)
	for i := range parts {
		parts[i] = item
	}
	records := strings.Join(parts, ",")
	return `{"namespace":"bench","enabled":true,"records":[` + records + `],"summary":"` + strings.Repeat("structured-json-", 40) + `"}`
}

func benchmarkBase64Payload() string {
	raw := make([]byte, 16*1024)
	x := uint64(0x9E3779B97F4A7C15)
	for i := range raw {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		raw[i] = byte(x)
	}

	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(raw)))
	base64.StdEncoding.Encode(encoded, raw)
	return string(encoded)
}
