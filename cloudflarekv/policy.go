package cloudflarekv

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dop251/goja_nodejs/cloudflarekv/store"
)

// Limits controls Cloudflare-compatible validation performed by the JS
// binding. It does not constrain direct calls to NamespaceStore.
type Limits struct {
	MaxKeyBytes          int
	MaxValueBytes        int64
	MaxMetadataBytes     int
	MaxBulkKeys          int
	MaxBulkResponseBytes int64
	MaxListKeys          int
	MinCacheTTL          time.Duration
	MinExpirationTTL     time.Duration
}

// CloudflareLimits returns the default limits used by new bindings.
func CloudflareLimits() Limits {
	return Limits{
		MaxKeyBytes:          512,
		MaxValueBytes:        25 * 1024 * 1024,
		MaxMetadataBytes:     1024,
		MaxBulkKeys:          100,
		MaxBulkResponseBytes: 25 * 1024 * 1024,
		MaxListKeys:          1000,
		MinCacheTTL:          30 * time.Second,
		MinExpirationTTL:     60 * time.Second,
	}
}

type bindingConfig struct {
	limits            Limits
	writeRateInterval time.Duration
	cacheCapacity     int64
	cacheDefaultTTL   time.Duration
}

// BindOption customizes validation and local bridge behavior for one binding.
type BindOption func(*bindingConfig)

// WithLimits replaces the Cloudflare-compatible binding limits. A non-positive
// maximum or minimum disables that individual limit.
func WithLimits(limits Limits) BindOption {
	return func(config *bindingConfig) { config.limits = limits }
}

// WithWriteRateLimit sets the minimum interval between writes to the same key.
// A non-positive duration disables local rate limiting.
func WithWriteRateLimit(interval time.Duration) BindOption {
	return func(config *bindingConfig) { config.writeRateInterval = interval }
}

// WithCacheCapacity sets the maximum number of value and metadata bytes kept
// in the binding-local LRU cache. A non-positive value disables the cache.
func WithCacheCapacity(bytes int64) BindOption {
	return func(config *bindingConfig) { config.cacheCapacity = bytes }
}

// WithCacheDefaultTTL sets the cache lifetime used when cacheTtl is omitted.
// A non-positive value disables caching unless a request supplies cacheTtl.
func WithCacheDefaultTTL(ttl time.Duration) BindOption {
	return func(config *bindingConfig) { config.cacheDefaultTTL = ttl }
}

type bindingState struct {
	ns store.NamespaceStore

	config bindingConfig
	mu     sync.Mutex
	last   map[string]time.Time
	cache  *lruCache
}

func newBindingState(ns store.NamespaceStore, options []BindOption) *bindingState {
	config := bindingConfig{
		limits:            CloudflareLimits(),
		writeRateInterval: time.Second,
		cacheCapacity:     64 * 1024 * 1024,
		cacheDefaultTTL:   60 * time.Second,
	}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	return &bindingState{
		ns: ns, config: config, last: make(map[string]time.Time),
		cache: newLRUCache(config.cacheCapacity),
	}
}

func (state *bindingState) cacheTTL(requested time.Duration, supplied bool) (time.Duration, error) {
	if supplied {
		if minimum := state.config.limits.MinCacheTTL; minimum > 0 && requested < minimum {
			return 0, fmt.Errorf("cacheTtl must be at least %d seconds", int64(minimum/time.Second))
		}
		return requested, nil
	}
	return state.config.cacheDefaultTTL, nil
}

func (state *bindingState) validateKey(key string) error {
	if key == "" || key == "." || key == ".." {
		return fmt.Errorf("KV key %q is not allowed", key)
	}
	if maximum := state.config.limits.MaxKeyBytes; maximum > 0 && len([]byte(key)) > maximum {
		return fmt.Errorf("KV key exceeds the maximum length of %d bytes", maximum)
	}
	return nil
}

func (state *bindingState) validateKeys(keys []string, bulk bool) error {
	if bulk {
		if len(keys) == 0 {
			return errors.New("bulk get requires at least one key")
		}
		if maximum := state.config.limits.MaxBulkKeys; maximum > 0 && len(keys) > maximum {
			return fmt.Errorf("bulk get accepts at most %d keys", maximum)
		}
	}
	for _, key := range keys {
		if err := state.validateKey(key); err != nil {
			return err
		}
	}
	return nil
}

func (state *bindingState) validateValueSize(size int64) error {
	if maximum := state.config.limits.MaxValueBytes; maximum > 0 && size > maximum {
		return fmt.Errorf("KV value exceeds the maximum size of %d bytes", maximum)
	}
	return nil
}

func (state *bindingState) validatePutOptions(options store.PutOptions) error {
	if minimum := state.config.limits.MinExpirationTTL; minimum > 0 {
		if options.ExpirationTTL > 0 && options.ExpirationTTL < minimum {
			return fmt.Errorf("expirationTtl must be at least %d seconds", int64(minimum/time.Second))
		}
		if options.Expiration != nil && time.Until(*options.Expiration) < minimum {
			return fmt.Errorf("expiration must be at least %d seconds in the future", int64(minimum/time.Second))
		}
	}
	if options.Metadata != nil {
		encoded, err := json.Marshal(options.Metadata)
		if err != nil {
			return fmt.Errorf("metadata must be JSON serializable: %w", err)
		}
		if maximum := state.config.limits.MaxMetadataBytes; maximum > 0 && len(encoded) > maximum {
			return fmt.Errorf("metadata exceeds the maximum size of %d bytes", maximum)
		}
	}
	return nil
}

func (state *bindingState) validateListOptions(options *store.ListOptions) error {
	maximum := state.config.limits.MaxListKeys
	if options.Limit == 0 && maximum > 0 {
		options.Limit = maximum
	}
	if options.Limit < 0 {
		return errors.New("list limit must be positive")
	}
	if maximum > 0 && options.Limit > maximum {
		return fmt.Errorf("list limit cannot exceed %d", maximum)
	}
	return nil
}

func (state *bindingState) beginWrite(key string) error {
	interval := state.config.writeRateInterval
	if interval <= 0 {
		return nil
	}
	now := time.Now()
	state.mu.Lock()
	defer state.mu.Unlock()
	if previous, found := state.last[key]; found && now.Sub(previous) < interval {
		if interval == time.Second {
			return errors.New("KV permits only one write to a key every 1 second")
		}
		return fmt.Errorf("KV permits only one write to a key every %s", interval)
	}
	state.last[key] = now
	if len(state.last) > 4096 {
		for candidate, writtenAt := range state.last {
			if now.Sub(writtenAt) >= interval {
				delete(state.last, candidate)
			}
		}
	}
	return nil
}
