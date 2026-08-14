// Package store defines the KV domain contract consumed by the
// cloudflarekv bridge and implemented by storage adapters.
//
// It is a leaf package: it depends only on the standard library, so that
// cloudflarekv can be used without pulling in any storage backend.
package store

import (
	"context"
	"encoding/json"
	"time"
)

// NamespaceStore is the storage port consumed by the cloudflarekv bridge and
// implemented by storage adapters (e.g. an in-memory or database-backed store).
type NamespaceStore interface {
	Put(ctx context.Context, key string, value []byte, options PutOptions) error
	Get(ctx context.Context, key string) (Record, bool, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, options ListOptions) (ListResult, error)
}

type PutOptions struct {
	Metadata      any
	Expiration    *time.Time
	ExpirationTTL time.Duration
	ValueKind     ValueKind
}

type Record struct {
	Key        string
	Value      []byte
	Metadata   json.RawMessage
	Expiration *time.Time
}

type ListOptions struct {
	Prefix string
	Limit  int
	Cursor string
}

type ListKey struct {
	Name       string
	Expiration *time.Time
	Metadata   json.RawMessage
}

type ListResult struct {
	Keys         []ListKey
	ListComplete bool
	Cursor       string
}
