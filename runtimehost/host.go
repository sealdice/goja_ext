// Package runtimehost stores host capabilities and canonical module state for
// a goja runtime without exposing private data on the JavaScript global object.
package runtimehost

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"

	"github.com/dop251/goja"
)

// Scheduler serializes JavaScript work onto the goroutine that owns Runtime.
type Scheduler interface {
	Runtime() *goja.Runtime
	RunOnLoop(func(*goja.Runtime)) bool
}

// CwdProvider owns a runtime's logical working directory.
type CwdProvider interface {
	Cwd() string
	Chdir(string) error
}

// Key is an identity-based key for private, per-runtime state.
type Key struct {
	name string
}

// NewKey returns a package-owned key. Keys with the same name remain distinct.
func NewKey(name string) *Key {
	if name == "" {
		panic("runtimehost: key name must not be empty")
	}
	return &Key{name: name}
}

func (k *Key) String() string {
	if k == nil {
		return "<nil>"
	}
	return k.name
}

type stateEntry struct {
	ready chan struct{}
	value any
}

// Host contains the private capabilities associated with one runtime.
type Host struct {
	runtime *goja.Runtime

	mu              sync.RWMutex
	scheduler       Scheduler
	cwd             CwdProvider
	customCwd       bool
	canonicalValues map[*Key]*stateEntry
}

var hosts sync.Map // map[*goja.Runtime]*Host

// For returns the canonical host for rt, creating it when necessary.
func For(rt *goja.Runtime) *Host {
	if rt == nil {
		return nil
	}
	if value, ok := hosts.Load(rt); ok {
		return value.(*Host)
	}
	host := &Host{
		runtime:         rt,
		cwd:             newDefaultCwdProvider(),
		canonicalValues: make(map[*Key]*stateEntry),
	}
	actual, _ := hosts.LoadOrStore(rt, host)
	return actual.(*Host)
}

// Lookup returns an existing host without creating one.
func Lookup(rt *goja.Runtime) (*Host, bool) {
	if rt == nil {
		return nil, false
	}
	value, ok := hosts.Load(rt)
	if !ok {
		return nil, false
	}
	return value.(*Host), true
}

// Detach releases the registry's reference to a runtime and its private state.
// The caller must ensure no module initialization is in progress.
func Detach(rt *goja.Runtime) bool {
	if rt == nil {
		return false
	}
	_, loaded := hosts.LoadAndDelete(rt)
	return loaded
}

// Runtime returns the runtime owned by h.
func (h *Host) Runtime() *goja.Runtime {
	if h == nil {
		return nil
	}
	return h.runtime
}

// GetOrCreate returns the canonical value for key. The factory is run at most
// once successfully for a host and key, even with concurrent callers.
func GetOrCreate(rt *goja.Runtime, key *Key, factory func() any) any {
	host := For(rt)
	if host == nil || key == nil || factory == nil {
		return nil
	}

	host.mu.Lock()
	if entry, ok := host.canonicalValues[key]; ok {
		host.mu.Unlock()
		<-entry.ready
		return entry.value
	}
	entry := &stateEntry{ready: make(chan struct{})}
	host.canonicalValues[key] = entry
	host.mu.Unlock()

	completed := false
	defer func() {
		if completed {
			return
		}
		host.mu.Lock()
		delete(host.canonicalValues, key)
		close(entry.ready)
		host.mu.Unlock()
	}()

	value := factory()
	host.mu.Lock()
	entry.value = value
	completed = true
	close(entry.ready)
	host.mu.Unlock()
	return value
}

// Load returns canonical state previously stored for key.
func Load(rt *goja.Runtime, key *Key) (any, bool) {
	host, ok := Lookup(rt)
	if !ok || key == nil {
		return nil, false
	}
	host.mu.RLock()
	entry, ok := host.canonicalValues[key]
	host.mu.RUnlock()
	if !ok {
		return nil, false
	}
	<-entry.ready
	return entry.value, true
}

// Store records value if key has no canonical value yet.
func Store(rt *goja.Runtime, key *Key, value any) {
	GetOrCreate(rt, key, func() any { return value })
}

// ValidateScheduler verifies that scheduler owns rt.
func ValidateScheduler(rt *goja.Runtime, scheduler Scheduler) error {
	if rt == nil {
		return errors.New("runtimehost: runtime is required")
	}
	if isNilInterface(scheduler) {
		return errors.New("runtimehost: scheduler is required")
	}
	if scheduler.Runtime() != rt {
		return errors.New("runtimehost: scheduler belongs to a different runtime")
	}
	return nil
}

// BindScheduler associates a scheduler with rt. A different scheduler cannot
// replace an existing binding.
func BindScheduler(rt *goja.Runtime, scheduler Scheduler) error {
	if err := ValidateScheduler(rt, scheduler); err != nil {
		return err
	}
	host := For(rt)
	host.mu.Lock()
	defer host.mu.Unlock()
	if host.scheduler == nil {
		host.scheduler = scheduler
		return nil
	}
	if !sameInstance(host.scheduler, scheduler) {
		return errors.New("runtimehost: runtime already has a different scheduler")
	}
	return nil
}

// SchedulerFor returns the scheduler bound to rt.
func SchedulerFor(rt *goja.Runtime) (Scheduler, bool) {
	host, ok := Lookup(rt)
	if !ok {
		return nil, false
	}
	host.mu.RLock()
	defer host.mu.RUnlock()
	return host.scheduler, host.scheduler != nil
}

// BindCwdProvider replaces the default host provider. A custom provider may be
// rebound idempotently but cannot be silently replaced by a different one.
func BindCwdProvider(rt *goja.Runtime, provider CwdProvider) error {
	if rt == nil {
		return errors.New("runtimehost: runtime is required")
	}
	if isNilInterface(provider) {
		return errors.New("runtimehost: cwd provider is required")
	}
	host := For(rt)
	host.mu.Lock()
	defer host.mu.Unlock()
	if !host.customCwd {
		host.cwd = provider
		host.customCwd = true
		return nil
	}
	if !sameInstance(host.cwd, provider) {
		return errors.New("runtimehost: runtime already has a different cwd provider")
	}
	return nil
}

// Cwd returns the runtime's current logical working directory.
func Cwd(rt *goja.Runtime) string {
	host := For(rt)
	if host == nil {
		return ""
	}
	host.mu.RLock()
	provider := host.cwd
	host.mu.RUnlock()
	return provider.Cwd()
}

// Chdir changes the runtime's logical working directory through its provider.
func Chdir(rt *goja.Runtime, directory string) error {
	host := For(rt)
	if host == nil {
		return errors.New("runtimehost: runtime is required")
	}
	host.mu.RLock()
	provider := host.cwd
	host.mu.RUnlock()
	return provider.Chdir(directory)
}

func sameInstance(left, right any) bool {
	if left == nil || right == nil {
		return left == right
	}
	lv := reflect.ValueOf(left)
	rv := reflect.ValueOf(right)
	if lv.Type() != rv.Type() || !lv.Comparable() || !rv.Comparable() {
		return false
	}
	return lv.Interface() == rv.Interface()
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

type defaultCwdProvider struct {
	mu  sync.RWMutex
	cwd string
}

func newDefaultCwdProvider() *defaultCwdProvider {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = string(filepath.Separator)
	}
	return &defaultCwdProvider{cwd: filepath.Clean(cwd)}
}

func (p *defaultCwdProvider) Cwd() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cwd
}

func (p *defaultCwdProvider) Chdir(directory string) error {
	if directory == "" {
		return errors.New("runtimehost: cwd must not be empty")
	}
	if !filepath.IsAbs(directory) {
		directory = filepath.Join(p.Cwd(), directory)
	}
	directory = filepath.Clean(directory)
	info, err := os.Stat(directory)
	if err != nil {
		return fmt.Errorf("runtimehost: chdir %q: %w", directory, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("runtimehost: chdir %q: not a directory", directory)
	}
	p.mu.Lock()
	p.cwd = directory
	p.mu.Unlock()
	return nil
}
