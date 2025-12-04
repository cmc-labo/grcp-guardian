package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"time"

	"github.com/grpc-guardian/grpc-guardian/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// UnstableService simulates an unreliable service that fails randomly
type UnstableService struct{}

var requestCount = 0

// UnaryMethod simulates an RPC that fails 60% of the time
func (s *UnstableService) UnaryMethod(ctx context.Context, req interface{}) (interface{}, error) {
	requestCount++

	// Simulate random failures (60% failure rate)
	if rand.Float32() < 0.6 {
		log.Printf("[Server] Request #%d: Returning Unavailable error", requestCount)
		return nil, status.Error(codes.Unavailable, "service temporarily unavailable")
	}

	log.Printf("[Server] Request #%d: Success!", requestCount)
	return fmt.Sprintf("Success! Request #%d", requestCount), nil
}

func main() {
	rand.Seed(time.Now().UnixNano())

	// Start server in background
	go startServer()

	// Wait for server to start
	time.Sleep(500 * time.Millisecond)

	// Run client demos
	fmt.Println("=== Retry Middleware Demo ===\n")

	demo1_WithoutRetry()
	time.Sleep(1 * time.Second)

	demo2_WithRetry()
	time.Sleep(1 * time.Second)

	demo3_CustomConfiguration()
	time.Sleep(1 * time.Second)

	demo4_RetryCallback()
}

// startServer starts a gRPC server with the unstable service
func startServer() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	server := grpc.NewServer(
		grpc.UnaryInterceptor(func(
			ctx context.Context,
			req interface{},
			info *grpc.UnaryServerInfo,
			handler grpc.UnaryHandler,
		) (interface{}, error) {
			return (&UnstableService{}).UnaryMethod(ctx, req)
		}),
	)

	log.Printf("Server listening on :50051")
	if err := server.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

// demo1_WithoutRetry shows what happens without retry middleware
func demo1_WithoutRetry() {
	fmt.Println("Demo 1: Without Retry Middleware")
	fmt.Println("==================================")

	conn, err := grpc.Dial(
		"localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Make 5 requests without retry
	successCount := 0
	for i := 1; i <= 5; i++ {
		err := conn.Invoke(context.Background(), "/test.Service/UnaryMethod", nil, nil)
		if err != nil {
			fmt.Printf("[Client] Request %d: FAILED - %v\n", i, err)
		} else {
			fmt.Printf("[Client] Request %d: SUCCESS\n", i)
			successCount++
		}
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("\nResult: %d/5 requests succeeded (%.0f%%)\n\n", successCount, float64(successCount)/5*100)
}

// demo2_WithRetry shows the basic retry middleware in action
func demo2_WithRetry() {
	fmt.Println("Demo 2: With Retry Middleware (Default Config)")
	fmt.Println("===============================================")

	retry := middleware.NewRetry(
		middleware.WithMaxAttempts(3),
		middleware.WithInitialBackoff(100*time.Millisecond),
	)

	conn, err := grpc.Dial(
		"localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(retry.UnaryClientInterceptor()),
	)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Make 5 requests with retry
	successCount := 0
	for i := 1; i <= 5; i++ {
		err := conn.Invoke(context.Background(), "/test.Service/UnaryMethod", nil, nil)
		if err != nil {
			fmt.Printf("[Client] Request %d: FAILED after retries - %v\n", i, err)
		} else {
			fmt.Printf("[Client] Request %d: SUCCESS\n", i)
			successCount++
		}
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("\nResult: %d/5 requests succeeded (%.0f%%)\n", successCount, float64(successCount)/5*100)
	fmt.Println("Note: Success rate should be much higher with retries!\n")
}

// demo3_CustomConfiguration demonstrates custom retry configuration
func demo3_CustomConfiguration() {
	fmt.Println("Demo 3: Custom Retry Configuration")
	fmt.Println("===================================")
	fmt.Println("Config: Max 5 attempts, 50ms initial backoff, 2x multiplier\n")

	retry := middleware.NewRetry(
		middleware.WithMaxAttempts(5),
		middleware.WithInitialBackoff(50*time.Millisecond),
		middleware.WithBackoffMultiplier(2.0),
		middleware.WithJitter(true),
		middleware.WithRetryableCodes(codes.Unavailable, codes.ResourceExhausted),
	)

	conn, err := grpc.Dial(
		"localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(retry.UnaryClientInterceptor()),
	)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Make 3 requests
	for i := 1; i <= 3; i++ {
		start := time.Now()
		err := conn.Invoke(context.Background(), "/test.Service/UnaryMethod", nil, nil)
		duration := time.Since(start)

		if err != nil {
			fmt.Printf("[Client] Request %d: FAILED after %.2fs - %v\n", i, duration.Seconds(), err)
		} else {
			fmt.Printf("[Client] Request %d: SUCCESS after %.2fs\n", i, duration.Seconds())
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Println()
}

// demo4_RetryCallback demonstrates the retry callback feature
func demo4_RetryCallback() {
	fmt.Println("Demo 4: Retry with Callback (Observability)")
	fmt.Println("===========================================")

	retry := middleware.NewRetry(
		middleware.WithMaxAttempts(4),
		middleware.WithInitialBackoff(100*time.Millisecond),
		middleware.WithOnRetry(func(attempt int, err error, nextBackoff time.Duration) {
			fmt.Printf("  [Retry] Attempt %d failed: %v\n", attempt, err)
			fmt.Printf("  [Retry] Waiting %.0fms before next attempt...\n", nextBackoff.Seconds()*1000)
		}),
	)

	conn, err := grpc.Dial(
		"localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(retry.UnaryClientInterceptor()),
	)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Make 2 requests to see retry behavior
	for i := 1; i <= 2; i++ {
		fmt.Printf("\nRequest %d:\n", i)
		err := conn.Invoke(context.Background(), "/test.Service/UnaryMethod", nil, nil)
		if err != nil {
			fmt.Printf("[Client] Final result: FAILED - %v\n", err)
		} else {
			fmt.Printf("[Client] Final result: SUCCESS\n")
		}
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Println("\n=== Demo Complete ===")
	fmt.Println("\nKey Takeaways:")
	fmt.Println("1. Retry middleware dramatically improves success rates for transient failures")
	fmt.Println("2. Exponential backoff prevents overwhelming failing services")
	fmt.Println("3. Jitter helps avoid thundering herd problems")
	fmt.Println("4. Callbacks enable observability and monitoring")
	fmt.Println("5. Configurable retry policies adapt to different scenarios")
}

/*
Expected Output:

=== Retry Middleware Demo ===

Demo 1: Without Retry Middleware
==================================
[Server] Request #1: Returning Unavailable error
[Client] Request 1: FAILED - rpc error: code = Unavailable desc = service temporarily unavailable
[Server] Request #2: Returning Unavailable error
[Client] Request 2: FAILED - rpc error: code = Unavailable desc = service temporarily unavailable
[Server] Request #3: Success!
[Client] Request 3: SUCCESS
[Server] Request #4: Returning Unavailable error
[Client] Request 4: FAILED - rpc error: code = Unavailable desc = service temporarily unavailable
[Server] Request #5: Returning Unavailable error
[Client] Request 5: FAILED - rpc error: code = Unavailable desc = service temporarily unavailable

Result: 1/5 requests succeeded (20%)

Demo 2: With Retry Middleware (Default Config)
===============================================
[Server] Request #6: Returning Unavailable error
[Server] Request #7: Success!
[Client] Request 1: SUCCESS
[Server] Request #8: Success!
[Client] Request 2: SUCCESS
[Server] Request #9: Returning Unavailable error
[Server] Request #10: Returning Unavailable error
[Server] Request #11: Success!
[Client] Request 3: SUCCESS
[Server] Request #12: Success!
[Client] Request 4: SUCCESS
[Server] Request #13: Returning Unavailable error
[Server] Request #14: Success!
[Client] Request 5: SUCCESS

Result: 5/5 requests succeeded (100%)
Note: Success rate should be much higher with retries!

...
*/
