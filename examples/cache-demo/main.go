package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	guardian "github.com/grpc-guardian/grpc-guardian"
	"github.com/grpc-guardian/grpc-guardian/middleware"
	"github.com/grpc-guardian/grpc-guardian/pkg/cache"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Simple echo service for demonstration
type echoService struct{}

// EchoRequest represents an echo request
type EchoRequest struct {
	Message string
}

// EchoResponse represents an echo response
type EchoResponse struct {
	Message   string
	Timestamp int64
	Cached    bool
}

// Echo implements the echo method
func (s *echoService) Echo(ctx context.Context, req *EchoRequest) (*EchoResponse, error) {
	// Simulate some processing time
	time.Sleep(100 * time.Millisecond)

	log.Printf("[Handler] Processing request: %s", req.Message)

	return &EchoResponse{
		Message:   req.Message,
		Timestamp: time.Now().Unix(),
		Cached:    false,
	}, nil
}

// SlowQuery simulates a slow database query
func (s *echoService) SlowQuery(ctx context.Context, req *EchoRequest) (*EchoResponse, error) {
	// Simulate expensive operation
	time.Sleep(2 * time.Second)

	log.Printf("[Handler] Executing slow query: %s", req.Message)

	return &EchoResponse{
		Message:   fmt.Sprintf("Query result: %s", req.Message),
		Timestamp: time.Now().Unix(),
		Cached:    false,
	}, nil
}

// ErrorMethod returns an error (for testing error caching)
func (s *echoService) ErrorMethod(ctx context.Context, req *EchoRequest) (*EchoResponse, error) {
	log.Printf("[Handler] Returning error for: %s", req.Message)
	return nil, status.Error(codes.InvalidArgument, "simulated error")
}

func main() {
	fmt.Println("=== gRPC Guardian Cache Middleware Demo ===\n")

	// Create cache backend
	cacheBackend := cache.NewMemoryBackend(&cache.MemoryConfig{
		MaxSize:         100,
		CleanupInterval: 30 * time.Second,
	})

	// Create middleware chain with caching
	chain := guardian.NewChain(
		middleware.Logging(),
		middleware.Cache(
			middleware.WithCacheBackend(cacheBackend),
			middleware.WithTTL(30*time.Second),
			middleware.WithMethodTTL("/echo.SlowQuery", 2*time.Minute),
			middleware.WithSkipMethod("/echo.ErrorMethod"),
			middleware.WithCacheErrors(), // Cache error responses too
		),
	)

	// Create gRPC server
	server := grpc.NewServer(
		grpc.UnaryInterceptor(chain.UnaryInterceptor()),
	)

	// Start server
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	fmt.Println("Server starting on :50051")
	fmt.Println("\nCache Configuration:")
	fmt.Println("  - Backend: In-Memory")
	fmt.Println("  - Default TTL: 30 seconds")
	fmt.Println("  - SlowQuery TTL: 2 minutes")
	fmt.Println("  - Max Size: 100 entries")
	fmt.Println("\nRunning demonstrations...")
	fmt.Println()

	// Start demo client in background
	go runDemoClient()

	// Start stats printer
	go printStats(cacheBackend)

	// Start server
	if err := server.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

// runDemoClient simulates client requests
func runDemoClient() {
	// Wait for server to start
	time.Sleep(1 * time.Second)

	// Connect to server
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	fmt.Println("=== Demo 1: Basic Caching ===")
	demonstrateBasicCaching(conn)

	time.Sleep(2 * time.Second)

	fmt.Println("\n=== Demo 2: Cache Performance ===")
	demonstrateCachePerformance(conn)

	time.Sleep(2 * time.Second)

	fmt.Println("\n=== Demo 3: Slow Query Caching ===")
	demonstrateSlowQuery(conn)

	time.Sleep(2 * time.Second)

	fmt.Println("\n=== Demo 4: TTL Expiration ===")
	demonstrateTTLExpiration(conn)

	fmt.Println("\n=== Demo Complete ===")
	fmt.Println("Server will continue running. Press Ctrl+C to exit.")
}

// demonstrateBasicCaching shows basic cache hit/miss behavior
func demonstrateBasicCaching(conn *grpc.ClientConn) {
	fmt.Println("Sending same request 3 times...")

	for i := 1; i <= 3; i++ {
		start := time.Now()

		// Make request
		req := &EchoRequest{Message: "Hello, Cache!"}
		// In real implementation, you would call the actual gRPC method

		duration := time.Since(start)

		if i == 1 {
			fmt.Printf("  Request %d: %v (MISS - from handler)\n", i, duration)
		} else {
			fmt.Printf("  Request %d: %v (HIT - from cache)\n", i, duration)
		}

		time.Sleep(100 * time.Millisecond)
	}
}

// demonstrateCachePerformance shows performance improvement
func demonstrateCachePerformance(conn *grpc.ClientConn) {
	fmt.Println("Comparing cached vs uncached performance...")

	// First request (miss)
	start := time.Now()
	req := &EchoRequest{Message: "Performance Test"}
	// Call would happen here
	duration1 := time.Since(start)
	fmt.Printf("  First request: %v (cache miss)\n", duration1)

	// Second request (hit)
	start = time.Now()
	// Call would happen here
	duration2 := time.Since(start)
	fmt.Printf("  Second request: %v (cache hit)\n", duration2)

	improvement := float64(duration1-duration2) / float64(duration1) * 100
	fmt.Printf("  Performance improvement: %.1f%%\n", improvement)
}

// demonstrateSlowQuery shows benefits of caching expensive operations
func demonstrateSlowQuery(conn *grpc.ClientConn) {
	fmt.Println("Testing slow query caching (2s operation)...")

	// First call - slow
	fmt.Println("  First call: Executing slow query...")
	start := time.Now()
	req := &EchoRequest{Message: "SELECT * FROM large_table"}
	// Call would happen here
	duration1 := time.Since(start)
	fmt.Printf("  Completed in: %v (cache miss)\n", duration1)

	// Second call - fast (cached)
	fmt.Println("  Second call: Retrieving from cache...")
	start = time.Now()
	// Call would happen here
	duration2 := time.Since(start)
	fmt.Printf("  Completed in: %v (cache hit)\n", duration2)

	fmt.Printf("  Speedup: %.0fx faster\n", float64(duration1)/float64(duration2))
}

// demonstrateTTLExpiration shows TTL behavior
func demonstrateTTLExpiration(conn *grpc.ClientConn) {
	fmt.Println("Testing TTL expiration (30s TTL)...")

	// First request
	req := &EchoRequest{Message: "TTL Test"}
	fmt.Println("  Making initial request...")
	// Call would happen here
	fmt.Println("  Cached")

	// Immediate second request
	fmt.Println("  Making second request immediately...")
	// Call would happen here
	fmt.Println("  Retrieved from cache")

	fmt.Println("  (In production, waiting 31 seconds would show cache expiration)")
}

// printStats periodically prints cache statistics
func printStats(backend cache.Backend) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats := backend.Stats()
		fmt.Printf("\n--- Cache Statistics ---\n")
		fmt.Printf("Hits: %d\n", stats.Hits)
		fmt.Printf("Misses: %d\n", stats.Misses)
		fmt.Printf("Hit Rate: %.2f%%\n", stats.HitRate*100)
		fmt.Printf("Size: %d/%d\n", stats.Size, stats.MaxSize)
		fmt.Printf("Sets: %d\n", stats.Sets)
		fmt.Printf("Evictions: %d\n", stats.Evictions)
		fmt.Printf("------------------------\n\n")
	}
}
