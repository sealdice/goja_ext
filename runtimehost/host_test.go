package runtimehost

import (
	"errors"
	"testing"

	"github.com/dop251/goja"
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
	first := For(rt1)
	if first != For(rt1) {
		t.Fatal("same runtime returned different hosts")
	}
	if first == For(rt2) {
		t.Fatal("different runtimes shared a host")
	}
}

func TestHostCanonicalState(t *testing.T) {
	rt := goja.New()
	key := NewKey("test.value")
	calls := 0
	first := GetOrCreate(rt, key, func() any {
		calls++
		return rt.NewObject()
	})
	second := GetOrCreate(rt, key, func() any {
		calls++
		return rt.NewObject()
	})
	if first != second {
		t.Fatal("canonical value was recreated")
	}
	if calls != 1 {
		t.Fatalf("factory called %d times", calls)
	}
	if value, ok := Load(rt, key); !ok || value != first {
		t.Fatal("stored canonical value cannot be loaded")
	}
}

func TestSchedulerOwnership(t *testing.T) {
	rt := goja.New()
	other := goja.New()
	scheduler := &fakeScheduler{rt: rt}
	if err := BindScheduler(rt, scheduler); err != nil {
		t.Fatalf("bind matching scheduler: %v", err)
	}
	if got, ok := SchedulerFor(rt); !ok || got != scheduler {
		t.Fatal("bound scheduler was not returned")
	}
	if err := ValidateScheduler(rt, scheduler); err != nil {
		t.Fatalf("validate matching scheduler: %v", err)
	}
	if err := BindScheduler(other, scheduler); err == nil {
		t.Fatal("expected runtime mismatch")
	}
	if err := ValidateScheduler(other, scheduler); err == nil {
		t.Fatal("expected validation mismatch")
	}
	if err := BindScheduler(rt, &fakeScheduler{rt: rt}); err == nil {
		t.Fatal("expected conflicting scheduler rejection")
	}
}

func TestCwdProviderCanReplaceDefaultOnce(t *testing.T) {
	rt := goja.New()
	provider := &fakeCwdProvider{cwd: "/sandbox"}
	if err := BindCwdProvider(rt, provider); err != nil {
		t.Fatalf("bind cwd provider: %v", err)
	}
	if got := Cwd(rt); got != "/sandbox" {
		t.Fatalf("cwd = %q", got)
	}
	if err := Chdir(rt, "/sandbox/work"); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if got := Cwd(rt); got != "/sandbox/work" {
		t.Fatalf("cwd after chdir = %q", got)
	}
	if err := BindCwdProvider(rt, provider); err != nil {
		t.Fatalf("rebinding same provider: %v", err)
	}
	if err := BindCwdProvider(rt, &fakeCwdProvider{cwd: "/other"}); err == nil {
		t.Fatal("expected conflicting cwd provider rejection")
	}
	want := errors.New("denied")
	provider.err = want
	if err := Chdir(rt, "/denied"); !errors.Is(err, want) {
		t.Fatalf("chdir error = %v, want %v", err, want)
	}
}

func TestDetachDropsHostState(t *testing.T) {
	rt := goja.New()
	key := NewKey("test.detach")
	host := For(rt)
	Store(rt, key, "value")
	if !Detach(rt) {
		t.Fatal("expected existing host to be detached")
	}
	if _, ok := Lookup(rt); ok {
		t.Fatal("detached host still present")
	}
	if For(rt) == host {
		t.Fatal("detached runtime reused old host")
	}
	if _, ok := Load(rt, key); ok {
		t.Fatal("detached canonical state survived")
	}
	if !Detach(rt) {
		t.Fatal("newly-created host should detach")
	}
	if Detach(rt) {
		t.Fatal("detaching missing host should report false")
	}
}

func TestNilArgumentsAreRejected(t *testing.T) {
	key := NewKey("test.nil")
	if For(nil) != nil {
		t.Fatal("nil runtime returned a host")
	}
	if _, ok := Load(nil, key); ok {
		t.Fatal("nil runtime returned state")
	}
	if err := BindScheduler(goja.New(), nil); err == nil {
		t.Fatal("nil scheduler was accepted")
	}
	if err := BindCwdProvider(goja.New(), nil); err == nil {
		t.Fatal("nil cwd provider was accepted")
	}
}
