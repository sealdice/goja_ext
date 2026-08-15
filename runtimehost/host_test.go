package runtimehost_test

import (
	"errors"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/runtimehost"
)

type fakeScheduler struct{ rt *goja.Runtime }

func (s *fakeScheduler) Runtime() *goja.Runtime { return s.rt }
func (s *fakeScheduler) RunOnLoop(fn func(*goja.Runtime)) bool {
	fn(s.rt)
	return true
}

type fakeCwdProvider struct {
	cwd string
	err error
}

func (p *fakeCwdProvider) Cwd() string { return p.cwd }
func (p *fakeCwdProvider) Chdir(cwd string) error {
	if p.err != nil {
		return p.err
	}
	p.cwd = cwd
	return nil
}

func TestHostIsCanonicalPerRuntime(t *testing.T) {
	rt1 := goja.New()
	rt2 := goja.New()
	first := runtimehost.For(rt1)
	if first != runtimehost.For(rt1) {
		t.Fatal("same runtime returned different hosts")
	}
	if first == runtimehost.For(rt2) {
		t.Fatal("different runtimes shared a host")
	}
}

func TestHostCanonicalState(t *testing.T) {
	rt := goja.New()
	key := runtimehost.NewKey("test.value")
	calls := 0
	first := runtimehost.GetOrCreate(rt, key, func() any {
		calls++
		return rt.NewObject()
	})
	second := runtimehost.GetOrCreate(rt, key, func() any {
		calls++
		return rt.NewObject()
	})
	if first != second {
		t.Fatal("canonical value was recreated")
	}
	if calls != 1 {
		t.Fatalf("factory called %d times", calls)
	}
	if value, ok := runtimehost.Load(rt, key); !ok || value != first {
		t.Fatal("stored canonical value cannot be loaded")
	}
}

func TestSchedulerOwnership(t *testing.T) {
	rt := goja.New()
	other := goja.New()
	scheduler := &fakeScheduler{rt: rt}
	if err := runtimehost.BindScheduler(rt, scheduler); err != nil {
		t.Fatalf("bind matching scheduler: %v", err)
	}
	if got, ok := runtimehost.SchedulerFor(rt); !ok || got != scheduler {
		t.Fatal("bound scheduler was not returned")
	}
	if err := runtimehost.ValidateScheduler(rt, scheduler); err != nil {
		t.Fatalf("validate matching scheduler: %v", err)
	}
	if err := runtimehost.BindScheduler(other, scheduler); err == nil {
		t.Fatal("expected runtime mismatch")
	}
	if err := runtimehost.ValidateScheduler(other, scheduler); err == nil {
		t.Fatal("expected validation mismatch")
	}
	if err := runtimehost.BindScheduler(rt, &fakeScheduler{rt: rt}); err == nil {
		t.Fatal("expected conflicting scheduler rejection")
	}
}

func TestCwdProviderCanReplaceDefaultOnce(t *testing.T) {
	rt := goja.New()
	provider := &fakeCwdProvider{cwd: "/sandbox"}
	if err := runtimehost.BindCwdProvider(rt, provider); err != nil {
		t.Fatalf("bind cwd provider: %v", err)
	}
	if got := runtimehost.Cwd(rt); got != "/sandbox" {
		t.Fatalf("cwd = %q", got)
	}
	if err := runtimehost.Chdir(rt, "/sandbox/work"); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if got := runtimehost.Cwd(rt); got != "/sandbox/work" {
		t.Fatalf("cwd after chdir = %q", got)
	}
	if err := runtimehost.BindCwdProvider(rt, provider); err != nil {
		t.Fatalf("rebinding same provider: %v", err)
	}
	if err := runtimehost.BindCwdProvider(rt, &fakeCwdProvider{cwd: "/other"}); err == nil {
		t.Fatal("expected conflicting cwd provider rejection")
	}
	want := errors.New("denied")
	provider.err = want
	if err := runtimehost.Chdir(rt, "/denied"); !errors.Is(err, want) {
		t.Fatalf("chdir error = %v, want %v", err, want)
	}
}

func TestDetachDropsHostState(t *testing.T) {
	rt := goja.New()
	key := runtimehost.NewKey("test.detach")
	host := runtimehost.For(rt)
	runtimehost.Store(rt, key, "value")
	if !runtimehost.Detach(rt) {
		t.Fatal("expected existing host to be detached")
	}
	if _, ok := runtimehost.Lookup(rt); ok {
		t.Fatal("detached host still present")
	}
	if runtimehost.For(rt) == host {
		t.Fatal("detached runtime reused old host")
	}
	if _, ok := runtimehost.Load(rt, key); ok {
		t.Fatal("detached canonical state survived")
	}
	if !runtimehost.Detach(rt) {
		t.Fatal("newly-created host should detach")
	}
	if runtimehost.Detach(rt) {
		t.Fatal("detaching missing host should report false")
	}
}

func TestNilArgumentsAreRejected(t *testing.T) {
	key := runtimehost.NewKey("test.nil")
	if runtimehost.For(nil) != nil {
		t.Fatal("nil runtime returned a host")
	}
	if _, ok := runtimehost.Load(nil, key); ok {
		t.Fatal("nil runtime returned state")
	}
	if err := runtimehost.BindScheduler(goja.New(), nil); err == nil {
		t.Fatal("nil scheduler was accepted")
	}
	if err := runtimehost.BindCwdProvider(goja.New(), nil); err == nil {
		t.Fatal("nil cwd provider was accepted")
	}
}
