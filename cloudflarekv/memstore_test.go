package cloudflarekv_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/sealdice/goja_ext/cloudflarekv/store"
)

// memStore is an in-memory store.NamespaceStore used by the bridge tests.
type memStore struct {
	mu      sync.Mutex
	now     func() time.Time
	records map[string]memRecord
	puts    []memPutCall
}

type memRecord struct {
	value      []byte
	metadata   any
	expiration *time.Time
	valueKind  store.ValueKind
}

type memPutCall struct {
	key     string
	value   []byte
	options store.PutOptions
}

func newMemStore() *memStore {
	return &memStore{
		now:     time.Now,
		records: make(map[string]memRecord),
	}
}

func (m *memStore) Put(ctx context.Context, key string, value []byte, options store.PutOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var expiration *time.Time
	switch {
	case options.Expiration != nil:
		expiration = options.Expiration
	case options.ExpirationTTL > 0:
		exp := m.now().Add(options.ExpirationTTL)
		expiration = &exp
	}

	m.records[key] = memRecord{
		value:      append([]byte(nil), value...),
		metadata:   options.Metadata,
		expiration: expiration,
		valueKind:  options.ValueKind,
	}
	m.puts = append(m.puts, memPutCall{key: key, value: append([]byte(nil), value...), options: options})
	return nil
}

func (m *memStore) Get(ctx context.Context, key string) (store.Record, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	record, ok := m.records[key]
	if !ok {
		return store.Record{}, false, nil
	}
	if record.expiration != nil && m.now().After(*record.expiration) {
		return store.Record{}, false, nil
	}

	return store.Record{
		Key:      key,
		Value:    append([]byte(nil), record.value...),
		Metadata: metadataJSON(record.metadata),
	}, true, nil
}

func (m *memStore) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.records, key)
	return nil
}

func (m *memStore) List(ctx context.Context, options store.ListOptions) (store.ListResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	keys := make([]string, 0, len(m.records))
	for key := range m.records {
		if options.Prefix != "" && !hasPrefix(key, options.Prefix) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	cursorIndex := 0
	if options.Cursor != "" {
		raw, err := base64.StdEncoding.DecodeString(options.Cursor)
		if err != nil {
			return store.ListResult{}, err
		}
		cursorIndex = sort.SearchStrings(keys, string(raw))
		if cursorIndex < len(keys) && keys[cursorIndex] == string(raw) {
			cursorIndex++
		}
	}

	result := store.ListResult{}
	if options.Limit > 0 && len(keys)-cursorIndex > options.Limit {
		keys = keys[cursorIndex : cursorIndex+options.Limit]
		result.ListComplete = false
		result.Cursor = base64.StdEncoding.EncodeToString([]byte(keys[len(keys)-1]))
	} else {
		keys = keys[cursorIndex:]
		result.ListComplete = true
	}

	for _, key := range keys {
		record := m.records[key]
		item := store.ListKey{
			Name:     key,
			Metadata: metadataJSON(record.metadata),
		}
		if record.expiration != nil {
			item.Expiration = record.expiration
		}
		result.Keys = append(result.Keys, item)
	}
	return result, nil
}

func metadataJSON(metadata any) []byte {
	if metadata == nil {
		return nil
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		panic(err)
	}
	return data
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
