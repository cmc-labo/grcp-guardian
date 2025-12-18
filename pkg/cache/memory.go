package cache

import (
	"context"
	"sync"
	"time"
)

// MemoryBackend is an in-memory cache implementation
type MemoryBackend struct {
	mu         sync.RWMutex
	data       map[string]*Entry
	maxSize    int
	stats      Stats
	cleanupInterval time.Duration
	stopCleanup chan struct{}
}

// MemoryConfig holds configuration for memory cache
type MemoryConfig struct {
	MaxSize         int           // Maximum number of entries (0 = unlimited)
	CleanupInterval time.Duration // How often to clean expired entries
}

// DefaultMemoryConfig returns default memory cache configuration
func DefaultMemoryConfig() *MemoryConfig {
	return &MemoryConfig{
		MaxSize:         1000,
		CleanupInterval: 1 * time.Minute,
	}
}

// NewMemoryBackend creates a new in-memory cache backend
func NewMemoryBackend(config *MemoryConfig) *MemoryBackend {
	if config == nil {
		config = DefaultMemoryConfig()
	}

	mb := &MemoryBackend{
		data:            make(map[string]*Entry),
		maxSize:         config.MaxSize,
		cleanupInterval: config.CleanupInterval,
		stopCleanup:     make(chan struct{}),
	}

	mb.stats.MaxSize = config.MaxSize

	// Start background cleanup goroutine
	go mb.startCleanup()

	return mb
}

// Get retrieves a value from the cache
func (m *MemoryBackend) Get(ctx context.Context, key string) ([]byte, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, exists := m.data[key]
	if !exists {
		m.stats.Misses++
		m.updateHitRate()
		return nil, false, nil
	}

	// Check if expired
	if entry.IsExpired() {
		m.stats.Misses++
		m.updateHitRate()
		// Note: Actual deletion happens in cleanup goroutine
		return nil, false, nil
	}

	// Update access time
	entry.AccessedAt = time.Now()

	m.stats.Hits++
	m.updateHitRate()

	return entry.Value, true, nil
}

// Set stores a value in the cache with a TTL
func (m *MemoryBackend) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if we need to evict entries
	if m.maxSize > 0 && len(m.data) >= m.maxSize {
		// Evict oldest entry (simple LRU)
		m.evictOldest()
	}

	now := time.Now()
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = now.Add(ttl)
	}

	m.data[key] = &Entry{
		Value:      value,
		ExpiresAt:  expiresAt,
		CreatedAt:  now,
		AccessedAt: now,
	}

	m.stats.Sets++
	m.stats.Size = len(m.data)

	return nil
}

// Delete removes a value from the cache
func (m *MemoryBackend) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.data[key]; exists {
		delete(m.data, key)
		m.stats.Deletes++
		m.stats.Size = len(m.data)
	}

	return nil
}

// Clear removes all values from the cache
func (m *MemoryBackend) Clear(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data = make(map[string]*Entry)
	m.stats.Size = 0

	return nil
}

// Stats returns cache statistics
func (m *MemoryBackend) Stats() Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statsCopy := m.stats
	statsCopy.Size = len(m.data)
	return statsCopy
}

// Close stops the cleanup goroutine
func (m *MemoryBackend) Close() {
	close(m.stopCleanup)
}

// evictOldest removes the oldest accessed entry (simple LRU)
func (m *MemoryBackend) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range m.data {
		if oldestKey == "" || entry.AccessedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.AccessedAt
		}
	}

	if oldestKey != "" {
		delete(m.data, oldestKey)
		m.stats.Evictions++
	}
}

// startCleanup runs a background goroutine to clean expired entries
func (m *MemoryBackend) startCleanup() {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanup()
		case <-m.stopCleanup:
			return
		}
	}
}

// cleanup removes expired entries
func (m *MemoryBackend) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for key, entry := range m.data {
		if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
			delete(m.data, key)
			m.stats.Evictions++
		}
	}

	m.stats.Size = len(m.data)
}

// updateHitRate calculates the cache hit rate
func (m *MemoryBackend) updateHitRate() {
	total := m.stats.Hits + m.stats.Misses
	if total > 0 {
		m.stats.HitRate = float64(m.stats.Hits) / float64(total)
	}
}
