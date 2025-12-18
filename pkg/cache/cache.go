// Package cache provides caching backends for gRPC response caching
package cache

import (
	"context"
	"time"
)

// Backend defines the interface for cache storage backends
type Backend interface {
	// Get retrieves a value from the cache
	Get(ctx context.Context, key string) ([]byte, bool, error)

	// Set stores a value in the cache with a TTL
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes a value from the cache
	Delete(ctx context.Context, key string) error

	// Clear removes all values from the cache
	Clear(ctx context.Context) error

	// Stats returns cache statistics
	Stats() Stats
}

// Stats holds cache statistics
type Stats struct {
	Hits       uint64 // Number of cache hits
	Misses     uint64 // Number of cache misses
	Sets       uint64 // Number of cache sets
	Deletes    uint64 // Number of cache deletes
	Evictions  uint64 // Number of evictions
	Size       int    // Current number of items in cache
	MaxSize    int    // Maximum cache size
	HitRate    float64 // Cache hit rate (0.0 - 1.0)
}

// Entry represents a cached entry
type Entry struct {
	Value      []byte    // Cached value
	ExpiresAt  time.Time // Expiration time
	CreatedAt  time.Time // Creation time
	AccessedAt time.Time // Last access time
}

// IsExpired checks if the entry has expired
func (e *Entry) IsExpired() bool {
	if e.ExpiresAt.IsZero() {
		return false // Never expires
	}
	return time.Now().After(e.ExpiresAt)
}
