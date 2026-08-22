// Package store defines the KV domain contract consumed by the
// cloudflarekv bridge and implemented by storage adapters.
//
// It is a leaf package: it depends only on the standard library, so that
// cloudflarekv can be used without pulling in any storage backend.
package store

import (
	"context"
	"encoding/json"
	"io"
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

// StreamGetter is an optional capability for stores that can expose values
// without first materializing them as a byte slice.
type StreamGetter interface {
	GetStream(ctx context.Context, key string) (StreamRecord, bool, error)
}

// StreamPutter is an optional capability for stores that can consume values
// incrementally. Implementations must not make a partial value visible: a
// value is committed only after body reaches EOF and the method returns nil.
type StreamPutter interface {
	PutStream(ctx context.Context, key string, body io.Reader, options PutOptions) error
}

// BulkGetter is an optional capability for stores that can retrieve multiple
// complete records more efficiently than repeated Get calls. Missing keys are
// omitted from the returned map.
type BulkGetter interface {
	GetMany(ctx context.Context, keys []string) (map[string]Record, error)
}

// StreamRecord describes one immutable value snapshot. Body belongs to the
// caller and must be closed. Size is -1 when the length is unknown.
type StreamRecord struct {
	Key        string
	Body       io.ReadCloser
	Size       int64
	Metadata   json.RawMessage
	Expiration *time.Time
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
