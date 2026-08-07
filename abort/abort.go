package abort

import (
	"sync"

	"github.com/dop251/goja"
	"github.com/sealdice/goja_ext/runtimehost"
)

type abortListener struct {
	src goja.Value
	fn  func(goja.Value)
}

type abortSignal struct {
	mu        sync.Mutex
	aborted   bool
	reason    goja.Value
	rt        *goja.Runtime
	listeners []abortListener
	obj       *goja.Object
}

func newSignal(rt *goja.Runtime) *abortSignal {
	return &abortSignal{rt: rt}
}

func (s *abortSignal) bind(obj *goja.Object) {
	s.obj = obj
}

func (s *abortSignal) doAbort(reason goja.Value) bool {
	s.mu.Lock()
	if s.aborted {
		s.mu.Unlock()
		return false
	}
	s.aborted = true
	if reason == nil || goja.IsUndefined(reason) || goja.IsNull(reason) {
		reason = s.rt.ToValue("Aborted")
	}
	s.reason = reason
	listeners := s.listeners
	s.listeners = nil
	obj := s.obj
	s.mu.Unlock()

	if obj != nil {
		_ = obj.Set("aborted", true)
		_ = obj.Set("reason", reason)
	}
	for _, l := range listeners {
		l.fn(reason)
	}
	return true
}

func newAbortControllerCtor(rt *goja.Runtime) func(call goja.ConstructorCall) *goja.Object {
	return func(call goja.ConstructorCall) *goja.Object {
		sig := newSignal(rt)
		obj := call.This

		signalObj := buildSignalObj(rt, sig)
		_ = obj.Set("signal", signalObj)
		_ = obj.Set("abort", func(call goja.FunctionCall) goja.Value {
			sig.doAbort(call.Argument(0))
			return goja.Undefined()
		})
		return obj
	}
}

func newAbortSignalStatic(rt *goja.Runtime) *goja.Object {
	static := rt.NewObject()
	_ = static.Set("abort", func(call goja.FunctionCall) goja.Value {
		sig := newSignal(rt)
		signalObj := buildSignalObj(rt, sig)
		sig.doAbort(call.Argument(0))
		return signalObj
	})
	_ = static.Set("timeout", func(call goja.FunctionCall) goja.Value {
		if _, ok := runtimehost.SchedulerFor(rt); !ok {
			panic(rt.NewTypeError("AbortSignal.timeout requires a runtime scheduler"))
		}
		ms := call.Argument(0).ToInteger()
		sig := newSignal(rt)
		signalObj := buildSignalObj(rt, sig)
		timer := rt.Get("setTimeout")
		fn, ok := goja.AssertFunction(timer)
		if !ok {
			panic(rt.NewTypeError("AbortSignal.timeout requires the setTimeout capability"))
		}
		cb := func(goja.FunctionCall) goja.Value {
			sig.doAbort(rt.ToValue("TimeoutError"))
			return goja.Undefined()
		}
		if _, err := fn(goja.Undefined(), rt.ToValue(cb), rt.ToValue(ms)); err != nil {
			panic(err)
		}
		return signalObj
	})
	return static
}

func buildSignalObj(rt *goja.Runtime, sig *abortSignal) *goja.Object {
	obj := rt.NewObject()
	sig.bind(obj)

	_ = obj.Set("aborted", false)
	_ = obj.Set("reason", goja.Undefined())

	_ = obj.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		evType := call.Argument(0).String()
		if evType != "abort" {
			return goja.Undefined()
		}
		handler, ok := goja.AssertFunction(call.Argument(1))
		if !ok {
			return goja.Undefined()
		}
		fn := func(reason goja.Value) {
			_, _ = handler(goja.Undefined(), rt.ToValue(map[string]interface{}{
				"type":   "abort",
				"target": obj,
				"reason": reason,
			}))
		}
		sig.mu.Lock()
		if !sig.aborted {
			sig.listeners = append(sig.listeners, abortListener{src: call.Argument(1), fn: fn})
		}
		sig.mu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
		evType := call.Argument(0).String()
		if evType != "abort" {
			return goja.Undefined()
		}
		target := call.Argument(1)
		sig.mu.Lock()
		kept := sig.listeners[:0]
		for _, l := range sig.listeners {
			if !l.src.SameAs(target) {
				kept = append(kept, l)
			}
		}
		sig.listeners = kept
		sig.mu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("throwIfAborted", func(call goja.FunctionCall) goja.Value {
		sig.mu.Lock()
		aborted := sig.aborted
		reason := sig.reason
		sig.mu.Unlock()
		if aborted {
			panic(reason)
		}
		return goja.Undefined()
	})

	return obj
}
