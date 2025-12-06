package main

import (
	"context"
	"log"
	"net"
	"time"

	guardian "github.com/grpc-guardian/grpc-guardian"
	"github.com/grpc-guardian/grpc-guardian/middleware"
	"google.golang.org/grpc"
)

// Example service implementation demonstrating timeout behavior
type TimeoutDemoService struct{}

// FastMethod completes quickly (within timeout)
func (s *TimeoutDemoService) FastMethod(ctx context.Context, req *Request) (*Response, error) {
	log.Printf("FastMethod called with: %s", req.Data)

	// Simulate fast processing (100ms)
	time.Sleep(100 * time.Millisecond)

	return &Response{
		Message: "FastMethod completed successfully",
		Data:    req.Data,
	}, nil
}

// SlowMethod takes longer but within its specific timeout
func (s *TimeoutDemoService) SlowMethod(ctx context.Context, req *Request) (*Response, error) {
	log.Printf("SlowMethod called with: %s", req.Data)

	// Simulate slower processing (800ms)
	time.Sleep(800 * time.Millisecond)

	return &Response{
		Message: "SlowMethod completed successfully",
		Data:    req.Data,
	}, nil
}

// VerySlowMethod exceeds timeout and will fail
func (s *TimeoutDemoService) VerySlowMethod(ctx context.Context, req *Request) (*Response, error) {
	log.Printf("VerySlowMethod called with: %s", req.Data)

	// Simulate very slow processing (3 seconds)
	// This will exceed the default timeout
	select {
	case <-time.After(3 * time.Second):
		return &Response{
			Message: "VerySlowMethod completed (should not reach here)",
			Data:    req.Data,
		}, nil
	case <-ctx.Done():
		log.Println("VerySlowMethod: context cancelled due to timeout")
		return nil, ctx.Err()
	}
}

// ContextAwareMethod demonstrates proper context handling
func (s *TimeoutDemoService) ContextAwareMethod(ctx context.Context, req *Request) (*Response, error) {
	log.Printf("ContextAwareMethod called with: %s", req.Data)

	// Perform work while checking context
	for i := 0; i < 10; i++ {
		select {
		case <-ctx.Done():
			log.Printf("ContextAwareMethod: cancelled after %d iterations", i)
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
			log.Printf("ContextAwareMethod: iteration %d/10", i+1)
		}
	}

	return &Response{
		Message: "ContextAwareMethod completed all iterations",
		Data:    req.Data,
	}, nil
}

// Simple proto message types
type Request struct {
	Data string
}

type Response struct {
	Message string
	Data    string
}

func main() {
	log.Println("==============================================")
	log.Println("gRPC Guardian - Timeout Middleware Demo")
	log.Println("==============================================")
	log.Println()

	// Callback for timeout events
	timeoutCallback := func(method string, duration time.Duration) {
		log.Printf("⏱️  TIMEOUT: %s exceeded timeout of %v", method, duration)
	}

	// Create middleware chain with timeout configurations
	chain := guardian.NewChain(
		// 1. Logging - log all requests
		middleware.Logging(),

		// 2. Timeout middleware with different settings per method
		middleware.Timeout(
			// Default timeout for all methods
			middleware.WithTimeout(1*time.Second),

			// Callback when timeout occurs
			middleware.WithTimeoutCallback(timeoutCallback),

			// Method-specific timeouts
			middleware.WithPerMethodTimeout(map[string]time.Duration{
				"/demo.TimeoutDemoService/FastMethod":         500 * time.Millisecond,
				"/demo.TimeoutDemoService/SlowMethod":         2 * time.Second,
				"/demo.TimeoutDemoService/VerySlowMethod":     1 * time.Second,
				"/demo.TimeoutDemoService/ContextAwareMethod": 1500 * time.Millisecond,
			}),
		),

		// 3. Performance monitoring
		middleware.PerformanceLog(500 * time.Millisecond),
	)

	// Create gRPC server with middleware
	server := grpc.NewServer(
		grpc.UnaryInterceptor(chain.UnaryInterceptor()),
	)

	// Register service
	// In a real application:
	// pb.RegisterTimeoutDemoServiceServer(server, &TimeoutDemoService{})

	// Start server
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	log.Println("Server configuration:")
	log.Println("  • Port: 50051")
	log.Println()
	log.Println("Timeout configuration:")
	log.Println("  • Default timeout: 1 second")
	log.Println("  • FastMethod: 500ms")
	log.Println("  • SlowMethod: 2 seconds")
	log.Println("  • VerySlowMethod: 1 second (will timeout!)")
	log.Println("  • ContextAwareMethod: 1.5 seconds")
	log.Println()
	log.Println("Available methods:")
	log.Println("  ✓ FastMethod (100ms processing) - Will succeed")
	log.Println("  ✓ SlowMethod (800ms processing) - Will succeed")
	log.Println("  ✗ VerySlowMethod (3s processing) - Will timeout")
	log.Println("  ✓ ContextAwareMethod (checks context) - Depends on timing")
	log.Println()
	log.Println("Middleware stack:")
	log.Println("  1. Logging")
	log.Println("  2. Timeout enforcement")
	log.Println("  3. Performance monitoring (>500ms)")
	log.Println()
	log.Println("Server is ready to accept connections...")
	log.Println("==============================================")
	log.Println()

	if err := server.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
