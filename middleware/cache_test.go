package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/grpc-guardian/grpc-guardian/pkg/cache"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockRequest for testing
type mockRequest struct {
	ID   int
	Data string
}

// mockResponse for testing
type mockResponse struct {
	Result string
}

// mockHandler simulates a gRPC handler
func mockHandler(resp interface{}, err error) grpc.UnaryHandler {
	return func(ctx context.Context, req interface{}) (interface{}, error) {
		return resp, err
	}
}

// mockInfo creates mock UnaryServerInfo
func mockInfo(method string) *grpc.UnaryServerInfo {
	return &grpc.UnaryServerInfo{
		FullMethod: method,
	}
}

func TestCache_BasicCaching(t *testing.T) {
	backend := cache.NewMemoryBackend(cache.DefaultMemoryConfig())
	defer backend.Close()

	middleware := Cache(
		WithCacheBackend(backend),
		WithTTL(1*time.Minute),
	)

	req := &mockRequest{ID: 1, Data: "test"}
	expectedResp := &mockResponse{Result: "success"}
	info := mockInfo("/test.Service/Method")

	callCount := 0
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		callCount++
		return expectedResp, nil
	}

	// First call - cache miss
	resp1, err1 := middleware(context.Background(), req, info, handler)
	assert.NoError(t, err1)
	assert.Equal(t, expectedResp, resp1)
	assert.Equal(t, 1, callCount, "Handler should be called on cache miss")

	// Second call - cache hit
	resp2, err2 := middleware(context.Background(), req, info, handler)
	assert.NoError(t, err2)
	assert.Equal(t, expectedResp, resp2)
	assert.Equal(t, 1, callCount, "Handler should not be called on cache hit")

	// Verify cache statistics
	stats := backend.Stats()
	assert.Equal(t, uint64(1), stats.Hits, "Should have 1 cache hit")
	assert.Equal(t, uint64(1), stats.Misses, "Should have 1 cache miss")
}

func TestCache_TTLExpiration(t *testing.T) {
	backend := cache.NewMemoryBackend(cache.DefaultMemoryConfig())
	defer backend.Close()

	middleware := Cache(
		WithCacheBackend(backend),
		WithTTL(100*time.Millisecond), // Very short TTL for testing
	)

	req := &mockRequest{ID: 1, Data: "test"}
	expectedResp := &mockResponse{Result: "success"}
	info := mockInfo("/test.Service/Method")

	callCount := 0
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		callCount++
		return expectedResp, nil
	}

	// First call
	_, _ = middleware(context.Background(), req, info, handler)
	assert.Equal(t, 1, callCount)

	// Second call immediately - should be cached
	_, _ = middleware(context.Background(), req, info, handler)
	assert.Equal(t, 1, callCount)

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Third call after expiration - should call handler again
	_, _ = middleware(context.Background(), req, info, handler)
	assert.Equal(t, 2, callCount, "Handler should be called after TTL expiration")
}

func TestCache_MethodTTL(t *testing.T) {
	backend := cache.NewMemoryBackend(cache.DefaultMemoryConfig())
	defer backend.Close()

	method1 := "/test.Service/Method1"
	method2 := "/test.Service/Method2"

	middleware := Cache(
		WithCacheBackend(backend),
		WithTTL(1*time.Minute),
		WithMethodTTL(method2, 100*time.Millisecond),
	)

	req := &mockRequest{ID: 1, Data: "test"}
	expectedResp := &mockResponse{Result: "success"}

	callCount1 := 0
	handler1 := func(ctx context.Context, req interface{}) (interface{}, error) {
		callCount1++
		return expectedResp, nil
	}

	callCount2 := 0
	handler2 := func(ctx context.Context, req interface{}) (interface{}, error) {
		callCount2++
		return expectedResp, nil
	}

	// Call both methods
	_, _ = middleware(context.Background(), req, mockInfo(method1), handler1)
	_, _ = middleware(context.Background(), req, mockInfo(method2), handler2)

	// Wait for method2's short TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Call both again
	_, _ = middleware(context.Background(), req, mockInfo(method1), handler1)
	_, _ = middleware(context.Background(), req, mockInfo(method2), handler2)

	assert.Equal(t, 1, callCount1, "Method1 should still be cached")
	assert.Equal(t, 2, callCount2, "Method2 should have expired and called handler again")
}

func TestCache_SkipMethod(t *testing.T) {
	backend := cache.NewMemoryBackend(cache.DefaultMemoryConfig())
	defer backend.Close()

	skippedMethod := "/test.Service/SkipMe"
	cachedMethod := "/test.Service/CacheMe"

	middleware := Cache(
		WithCacheBackend(backend),
		WithTTL(1*time.Minute),
		WithSkipMethod(skippedMethod),
	)

	req := &mockRequest{ID: 1, Data: "test"}
	expectedResp := &mockResponse{Result: "success"}

	callCount := 0
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		callCount++
		return expectedResp, nil
	}

	// Call skipped method twice
	_, _ = middleware(context.Background(), req, mockInfo(skippedMethod), handler)
	_, _ = middleware(context.Background(), req, mockInfo(skippedMethod), handler)
	assert.Equal(t, 2, callCount, "Skipped method should always call handler")

	// Call cached method twice
	callCount = 0
	_, _ = middleware(context.Background(), req, mockInfo(cachedMethod), handler)
	_, _ = middleware(context.Background(), req, mockInfo(cachedMethod), handler)
	assert.Equal(t, 1, callCount, "Cached method should only call handler once")
}

func TestCache_OnlyMethod(t *testing.T) {
	backend := cache.NewMemoryBackend(cache.DefaultMemoryConfig())
	defer backend.Close()

	onlyMethod := "/test.Service/OnlyThis"
	otherMethod := "/test.Service/NotThis"

	middleware := Cache(
		WithCacheBackend(backend),
		WithTTL(1*time.Minute),
		WithOnlyMethod(onlyMethod),
	)

	req := &mockRequest{ID: 1, Data: "test"}
	expectedResp := &mockResponse{Result: "success"}

	callCount := 0
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		callCount++
		return expectedResp, nil
	}

	// Call only method twice
	_, _ = middleware(context.Background(), req, mockInfo(onlyMethod), handler)
	_, _ = middleware(context.Background(), req, mockInfo(onlyMethod), handler)
	assert.Equal(t, 1, callCount, "Only method should be cached")

	// Call other method twice
	callCount = 0
	_, _ = middleware(context.Background(), req, mockInfo(otherMethod), handler)
	_, _ = middleware(context.Background(), req, mockInfo(otherMethod), handler)
	assert.Equal(t, 2, callCount, "Other method should not be cached")
}

func TestCache_ErrorCaching(t *testing.T) {
	backend := cache.NewMemoryBackend(cache.DefaultMemoryConfig())
	defer backend.Close()

	middleware := Cache(
		WithCacheBackend(backend),
		WithTTL(1*time.Minute),
		WithCacheErrors(),
	)

	req := &mockRequest{ID: 1, Data: "test"}
	expectedErr := status.Error(codes.InvalidArgument, "test error")
	info := mockInfo("/test.Service/Method")

	callCount := 0
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		callCount++
		return nil, expectedErr
	}

	// First call - cache miss
	_, err1 := middleware(context.Background(), req, info, handler)
	assert.Error(t, err1)
	assert.Equal(t, codes.InvalidArgument, status.Code(err1))
	assert.Equal(t, 1, callCount)

	// Second call - cache hit (error should be cached)
	_, err2 := middleware(context.Background(), req, info, handler)
	assert.Error(t, err2)
	assert.Equal(t, codes.InvalidArgument, status.Code(err2))
	assert.Equal(t, 1, callCount, "Handler should not be called on cache hit for error")
}

func TestCache_DifferentRequests(t *testing.T) {
	backend := cache.NewMemoryBackend(cache.DefaultMemoryConfig())
	defer backend.Close()

	middleware := Cache(
		WithCacheBackend(backend),
		WithTTL(1*time.Minute),
	)

	req1 := &mockRequest{ID: 1, Data: "test1"}
	req2 := &mockRequest{ID: 2, Data: "test2"}
	info := mockInfo("/test.Service/Method")

	callCount := 0
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		callCount++
		mr := req.(*mockRequest)
		return &mockResponse{Result: mr.Data}, nil
	}

	// Call with req1
	resp1, _ := middleware(context.Background(), req1, info, handler)
	assert.Equal(t, "test1", resp1.(*mockResponse).Result)

	// Call with req2 (different request, should not use cache)
	resp2, _ := middleware(context.Background(), req2, info, handler)
	assert.Equal(t, "test2", resp2.(*mockResponse).Result)

	// Both should have called the handler
	assert.Equal(t, 2, callCount, "Different requests should not share cache")

	// Call req1 again (should be cached)
	resp3, _ := middleware(context.Background(), req1, info, handler)
	assert.Equal(t, "test1", resp3.(*mockResponse).Result)
	assert.Equal(t, 2, callCount, "Same request should use cache")
}

func TestInvalidateCache(t *testing.T) {
	backend := cache.NewMemoryBackend(cache.DefaultMemoryConfig())
	defer backend.Close()

	ctx := context.Background()
	method := "/test.Service/Method"
	req := &mockRequest{ID: 1, Data: "test"}

	// Set a cache entry
	gen := cache.NewDefaultKeyGenerator()
	key, _ := gen.GenerateKey(method, req)
	_ = backend.Set(ctx, key, []byte("test data"), 1*time.Minute)

	// Verify it exists
	_, found, _ := backend.Get(ctx, key)
	assert.True(t, found, "Cache entry should exist")

	// Invalidate
	err := InvalidateCache(ctx, backend, method, req)
	assert.NoError(t, err)

	// Verify it's gone
	_, found, _ = backend.Get(ctx, key)
	assert.False(t, found, "Cache entry should be invalidated")
}

func TestClearCache(t *testing.T) {
	backend := cache.NewMemoryBackend(cache.DefaultMemoryConfig())
	defer backend.Close()

	ctx := context.Background()

	// Set multiple entries
	_ = backend.Set(ctx, "key1", []byte("data1"), 1*time.Minute)
	_ = backend.Set(ctx, "key2", []byte("data2"), 1*time.Minute)
	_ = backend.Set(ctx, "key3", []byte("data3"), 1*time.Minute)

	stats := backend.Stats()
	assert.Equal(t, 3, stats.Size, "Should have 3 entries")

	// Clear cache
	err := ClearCache(ctx, backend)
	assert.NoError(t, err)

	stats = backend.Stats()
	assert.Equal(t, 0, stats.Size, "Cache should be empty")
}
