package fs

import (
	"errors"

	"github.com/dop251/goja"
)

type abortSubscription struct {
	object   *goja.Object
	listener goja.Value
	remove   goja.Callable
}

func (s *abortSubscription) cleanup(rt *goja.Runtime) {
	if s == nil || s.remove == nil {
		return
	}
	_, _ = s.remove(s.object, rt.ToValue("abort"), s.listener)
}

func (m *moduleInstance) promiseCallWithSignal(
	signal goja.Value,
	op func() (any, error),
	convert func(*goja.Runtime, any) goja.Value,
) goja.Value {
	promise, resolve, reject := m.rt.NewPromise()
	settled := false
	var subscription *abortSubscription

	settleAbort := func(reason goja.Value) {
		if settled {
			return
		}
		settled = true
		subscription.cleanup(m.rt)
		_ = reject(valueOrUndefined(reason))
	}

	var validationError goja.Value
	subscription, validationError = subscribeAbortSignal(m.rt, signal, settleAbort)
	if validationError != nil {
		settled = true
		_ = reject(validationError)
		return m.rt.ToValue(promise)
	}
	if settled {
		return m.rt.ToValue(promise)
	}

	run := func() {
		value, err := op()
		settle := func(rt *goja.Runtime) {
			if settled {
				return
			}
			settled = true
			subscription.cleanup(rt)
			if err != nil {
				_ = reject(jsErrorValue(rt, err))
				return
			}
			if convert == nil {
				_ = resolve(goja.Undefined())
				return
			}
			_ = resolve(convert(rt, value))
		}
		if m.scheduler != nil {
			_ = m.scheduler.RunOnLoop(settle)
		} else {
			settle(m.rt)
		}
	}
	if m.scheduler == nil {
		run()
	} else {
		go run()
	}
	return m.rt.ToValue(promise)
}

func subscribeAbortSignal(
	rt *goja.Runtime,
	value goja.Value,
	onAbort func(goja.Value),
) (*abortSubscription, goja.Value) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, nil
	}

	object := value.ToObject(rt)
	aborted, ok := object.Get("aborted").Export().(bool)
	if !ok {
		return nil, rt.NewTypeError("signal.aborted must be a boolean")
	}
	add, ok := goja.AssertFunction(object.Get("addEventListener"))
	if !ok {
		return nil, rt.NewTypeError("signal.addEventListener must be callable")
	}
	remove, _ := goja.AssertFunction(object.Get("removeEventListener"))
	if aborted {
		onAbort(object.Get("reason"))
		return nil, nil
	}

	listener := rt.ToValue(func(goja.FunctionCall) goja.Value {
		onAbort(object.Get("reason"))
		return goja.Undefined()
	})
	subscription := &abortSubscription{object: object, listener: listener, remove: remove}
	if _, err := add(object, rt.ToValue("abort"), listener); err != nil {
		var exception *goja.Exception
		if errors.As(err, &exception) {
			return nil, exception.Value()
		}
		return nil, rt.NewGoError(err)
	}
	return subscription, nil
}

func valueOrUndefined(value goja.Value) goja.Value {
	if value == nil {
		return goja.Undefined()
	}
	return value
}
