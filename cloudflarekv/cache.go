package cloudflarekv

import (
	"container/list"
	"context"
	"io"
	"sync"
	"time"

	"github.com/dop251/goja_nodejs/cloudflarekv/store"
)

func (state *bindingState) getRecordCached(ctx context.Context, key string, ttl time.Duration) (store.Record, bool, error) {
	if record, found, cached := state.cache.get(key); cached {
		return record, found, nil
	}
	record, found, err := state.ns.Get(ctx, key)
	if err == nil {
		state.cache.put(key, record, found, ttl)
	}
	return record, found, err
}

type cacheEntry struct {
	key       string
	record    store.Record
	found     bool
	expiresAt time.Time
	size      int64
}

type lruCache struct {
	mu       sync.Mutex
	capacity int64
	size     int64
	entries  map[string]*list.Element
	order    *list.List
}

func newLRUCache(capacity int64) *lruCache {
	if capacity <= 0 {
		return nil
	}
	return &lruCache{capacity: capacity, entries: make(map[string]*list.Element), order: list.New()}
}

func (cache *lruCache) get(key string) (store.Record, bool, bool) {
	if cache == nil {
		return store.Record{}, false, false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	element, ok := cache.entries[key]
	if !ok {
		return store.Record{}, false, false
	}
	entry := element.Value.(*cacheEntry)
	if !time.Now().Before(entry.expiresAt) {
		cache.remove(element)
		return store.Record{}, false, false
	}
	cache.order.MoveToFront(element)
	return cloneRecord(entry.record), entry.found, true
}

func (cache *lruCache) put(key string, record store.Record, found bool, ttl time.Duration) {
	if cache == nil || ttl <= 0 {
		return
	}
	if record.Expiration != nil {
		remaining := time.Until(*record.Expiration)
		if remaining <= 0 {
			return
		}
		if remaining < ttl {
			ttl = remaining
		}
	}
	size := int64(len(key) + len(record.Value) + len(record.Metadata))
	if size > cache.capacity {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if old, ok := cache.entries[key]; ok {
		cache.remove(old)
	}
	entry := &cacheEntry{
		key: key, record: cloneRecord(record), found: found,
		expiresAt: time.Now().Add(ttl), size: size,
	}
	cache.entries[key] = cache.order.PushFront(entry)
	cache.size += size
	for cache.size > cache.capacity {
		cache.remove(cache.order.Back())
	}
}

func (cache *lruCache) delete(key string) {
	if cache == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if element, ok := cache.entries[key]; ok {
		cache.remove(element)
	}
}

func (cache *lruCache) remove(element *list.Element) {
	if element == nil {
		return
	}
	entry := element.Value.(*cacheEntry)
	delete(cache.entries, entry.key)
	cache.order.Remove(element)
	cache.size -= entry.size
}

func cloneRecord(record store.Record) store.Record {
	record.Value = append([]byte(nil), record.Value...)
	record.Metadata = append([]byte(nil), record.Metadata...)
	if record.Expiration != nil {
		expiration := *record.Expiration
		record.Expiration = &expiration
	}
	return record
}

type cachingReadCloser struct {
	body       io.ReadCloser
	cache      *lruCache
	key        string
	metadata   []byte
	expiration *time.Time
	ttl        time.Duration
	buffer     []byte
	maximum    int64
	cacheable  bool
	once       sync.Once
}

func (reader *cachingReadCloser) Read(buffer []byte) (int, error) {
	n, err := reader.body.Read(buffer)
	if n > 0 && reader.cacheable {
		if int64(len(reader.buffer)+n+len(reader.key)+len(reader.metadata)) <= reader.maximum {
			reader.buffer = append(reader.buffer, buffer[:n]...)
		} else {
			reader.cacheable = false
			reader.buffer = nil
		}
	}
	if err == io.EOF && reader.cacheable {
		reader.once.Do(func() {
			reader.cache.put(reader.key, store.Record{
				Key: reader.key, Value: reader.buffer, Metadata: reader.metadata, Expiration: reader.expiration,
			}, true, reader.ttl)
		})
	}
	return n, err
}

func (reader *cachingReadCloser) Close() error { return reader.body.Close() }
