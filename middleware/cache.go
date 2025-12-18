package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/grpc-guardian/grpc-guardian/pkg/cache"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CacheConfig holds configuration for caching middleware
type CacheConfig struct {
	Backend      cache.Backend      // Cache backend
	KeyGenerator cache.KeyGenerator // Key generation strategy
	TTL          time.Duration      // Default TTL for cache entries
	MethodTTLs   map[string]time.Duration // Per-method TTL overrides
	SkipMethods  map[string]bool    // Methods to skip caching
	OnlyMethods  map[string]bool    // Only cache these methods (if set)
	CacheErrors  bool               // Whether to cache error responses
	SkipAuth     bool               // Skip caching for authenticated requests
}

// CacheOption is a functional option for cache configuration
type CacheOption func(*CacheConfig)

// WithBackend sets the cache backend
func WithCacheBackend(backend cache.Backend) CacheOption {
	return func(c *CacheConfig) {
		c.Backend = backend
	}
}

// WithKeyGenerator sets the key generation strategy
func WithKeyGenerator(gen cache.KeyGenerator) CacheOption {
	return func(c *CacheConfig) {
		c.KeyGenerator = gen
	}
}

// WithTTL sets the default TTL for cached responses
func WithTTL(ttl time.Duration) CacheOption {
	return func(c *CacheConfig) {
		c.TTL = ttl
	}
}

// WithMethodTTL sets a custom TTL for a specific method
func WithMethodTTL(method string, ttl time.Duration) CacheOption {
	return func(c *CacheConfig) {
		if c.MethodTTLs == nil {
			c.MethodTTLs = make(map[string]time.Duration)
		}
		c.MethodTTLs[method] = ttl
	}
}

// WithSkipMethod skips caching for a specific method
func WithSkipMethod(method string) CacheOption {
	return func(c *CacheConfig) {
		if c.SkipMethods == nil {
			c.SkipMethods = make(map[string]bool)
		}
		c.SkipMethods[method] = true
	}
}

// WithOnlyMethod only caches specific methods
func WithOnlyMethod(method string) CacheOption {
	return func(c *CacheConfig) {
		if c.OnlyMethods == nil {
			c.OnlyMethods = make(map[string]bool)
		}
		c.OnlyMethods[method] = true
	}
}

// WithCacheErrors enables caching of error responses
func WithCacheErrors() CacheOption {
	return func(c *CacheConfig) {
		c.CacheErrors = true
	}
}

// WithSkipAuth skips caching for authenticated requests
func WithSkipAuth() CacheOption {
	return func(c *CacheConfig) {
		c.SkipAuth = true
	}
}

// cachedResponse wraps a response for caching
type cachedResponse struct {
	Response interface{} `json:"response,omitempty"`
	Error    *cachedError `json:"error,omitempty"`
}

// cachedError represents a cached error
type cachedError struct {
	Code    codes.Code `json:"code"`
	Message string     `json:"message"`
}

// Cache creates a caching middleware
func Cache(opts ...CacheOption) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// Default configuration
	config := &CacheConfig{
		Backend:      cache.NewMemoryBackend(cache.DefaultMemoryConfig()),
		KeyGenerator: cache.NewDefaultKeyGenerator(),
		TTL:          5 * time.Minute, // Default 5 minute TTL
		SkipMethods:  make(map[string]bool),
		OnlyMethods:  make(map[string]bool),
		CacheErrors:  false,
		SkipAuth:     true,
	}

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		method := info.FullMethod

		// Check if method should be cached
		if !shouldCache(method, config) {
			return handler(ctx, req)
		}

		// Check if authenticated and skip auth is enabled
		if config.SkipAuth {
			if _, ok := GetUserID(ctx); ok {
				// Skip caching for authenticated requests
				return handler(ctx, req)
			}
		}

		// Generate cache key
		cacheKey, err := config.KeyGenerator.GenerateKey(method, req)
		if err != nil {
			// If key generation fails, skip caching
			return handler(ctx, req)
		}

		// Try to get from cache
		cached, found, err := config.Backend.Get(ctx, cacheKey)
		if err == nil && found {
			// Cache hit - deserialize and return
			var cachedResp cachedResponse
			if err := json.Unmarshal(cached, &cachedResp); err == nil {
				if cachedResp.Error != nil {
					// Return cached error
					return nil, status.Error(cachedResp.Error.Code, cachedResp.Error.Message)
				}
				// Return cached response
				return cachedResp.Response, nil
			}
		}

		// Cache miss - call handler
		resp, err := handler(ctx, req)

		// Determine if we should cache this response
		shouldCacheResp := true
		if err != nil && !config.CacheErrors {
			shouldCacheResp = false
		}

		if shouldCacheResp {
			// Prepare cached response
			cachedResp := cachedResponse{
				Response: resp,
			}

			if err != nil {
				st := status.Convert(err)
				cachedResp.Error = &cachedError{
					Code:    st.Code(),
					Message: st.Message(),
				}
			}

			// Serialize response
			data, marshalErr := json.Marshal(cachedResp)
			if marshalErr == nil {
				// Get TTL for this method
				ttl := config.TTL
				if methodTTL, ok := config.MethodTTLs[method]; ok {
					ttl = methodTTL
				}

				// Store in cache
				_ = config.Backend.Set(ctx, cacheKey, data, ttl)
			}
		}

		return resp, err
	}
}

// shouldCache determines if a method should be cached
func shouldCache(method string, config *CacheConfig) bool {
	// If OnlyMethods is set, only cache those methods
	if len(config.OnlyMethods) > 0 {
		return config.OnlyMethods[method]
	}

	// Otherwise, cache everything except skip methods
	return !config.SkipMethods[method]
}

// InvalidateCache invalidates a specific cache entry
func InvalidateCache(ctx context.Context, backend cache.Backend, method string, req interface{}) error {
	gen := cache.NewDefaultKeyGenerator()
	key, err := gen.GenerateKey(method, req)
	if err != nil {
		return fmt.Errorf("failed to generate cache key: %w", err)
	}

	return backend.Delete(ctx, key)
}

// ClearCache clears all cache entries
func ClearCache(ctx context.Context, backend cache.Backend) error {
	return backend.Clear(ctx)
}

// GetCacheStats returns cache statistics
func GetCacheStats(backend cache.Backend) cache.Stats {
	return backend.Stats()
}
