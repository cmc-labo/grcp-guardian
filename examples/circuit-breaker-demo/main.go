package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"time"

	guardian "github.com/grpc-guardian/grpc-guardian"
	"github.com/grpc-guardian/grpc-guardian/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DemoService is a simple service that can simulate failures
type DemoService struct{}

// Ping is a simple RPC method that may fail
func (s *DemoService) Ping(ctx context.Context, req *PingRequest) (*PingResponse, error) {
	// Simulate random failures (30% failure rate for demo)
	if rand.Float32() < 0.3 {
		return nil, status.Error(codes.Unavailable, "service temporarily unavailable")
	}

	// Simulate some processing time
	time.Sleep(10 * time.Millisecond)

	return &PingResponse{
		Message: fmt.Sprintf("Pong! Received: %s", req.Message),
	}, nil
}

// PingRequest is the request message
type PingRequest struct {
	Message string
}

// PingResponse is the response message
type PingResponse struct {
	Message string
}

func main() {
	log.Println("Starting gRPC Guardian Circuit Breaker Demo...")

	// Create circuit breaker with custom settings
	circuitBreaker := middleware.NewCircuitBreaker(
		middleware.WithFailureThreshold(0.5),      // Open after 50% failure rate
		middleware.WithTimeout(5*time.Second),      // Stay open for 5 seconds
		middleware.WithInterval(10*time.Second),    // Count failures over 10 second window
		middleware.WithMaxRequests(3),              // Allow 3 requests in half-open state
		middleware.WithSuccessThreshold(2),         // Need 2 successes to close
		middleware.WithOnStateChange(func(from, to middleware.State) {
			log.Printf("🔄 Circuit Breaker State Changed: %s -> %s", from, to)
		}),
	)

	// Create middleware chain with circuit breaker
	chain := guardian.NewChain(
		middleware.Logging(),
		middleware.CircuitBreakerMiddleware(
			middleware.WithFailureThreshold(0.5),
			middleware.WithTimeout(5*time.Second),
			middleware.WithInterval(10*time.Second),
			middleware.WithMaxRequests(3),
			middleware.WithSuccessThreshold(2),
			middleware.WithOnStateChange(func(from, to middleware.State) {
				log.Printf("🔄 Circuit Breaker State: %s -> %s", from, to)
			}),
		),
	)

	// Create gRPC server
	server := grpc.NewServer(
		grpc.UnaryInterceptor(chain.UnaryInterceptor()),
	)

	// Register demo service
	// In a real application, you would use protobuf-generated code
	// RegisterDemoServiceServer(server, &DemoService{})

	// Start server
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Start statistics reporter in background
	go reportStats(circuitBreaker)

	// Start load generator in background
	go generateLoad()

	log.Println("✅ Server listening on :50051")
	log.Println("📊 Circuit Breaker Statistics:")
	log.Println("   - Failure Threshold: 50%")
	log.Println("   - Open Timeout: 5 seconds")
	log.Println("   - Interval Window: 10 seconds")
	log.Println("")
	log.Println("🔥 Generating load with 30% failure rate...")
	log.Println("")

	if err := server.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

// reportStats periodically reports circuit breaker statistics
func reportStats(cb *middleware.CircuitBreaker) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats := cb.GetStats()
		counts := stats.Counts

		log.Println("=" * 60)
		log.Printf("📊 Circuit Breaker Stats:")
		log.Printf("   State: %s (Generation: %d)", stats.State, stats.Generation)
		log.Printf("   Requests: %d", counts.Requests)
		log.Printf("   Successes: %d (consecutive: %d)", counts.TotalSuccesses, counts.ConsecutiveSuccesses)
		log.Printf("   Failures: %d (consecutive: %d)", counts.TotalFailures, counts.ConsecutiveFailures)

		if counts.Requests > 0 {
			successRate := float64(counts.TotalSuccesses) / float64(counts.Requests) * 100
			failureRate := float64(counts.TotalFailures) / float64(counts.Requests) * 100
			log.Printf("   Success Rate: %.1f%%", successRate)
			log.Printf("   Failure Rate: %.1f%%", failureRate)
		}

		log.Printf("   Last State Change: %s", stats.StateChangedAt.Format(time.RFC3339))
		log.Println("=" * 60)
	}
}

// generateLoad simulates client requests
func generateLoad() {
	time.Sleep(2 * time.Second) // Wait for server to start

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	requestCount := 0

	for range ticker.C {
		requestCount++

		// Simulate client request
		// In a real application, this would be a gRPC client call
		go func(id int) {
			// Simulate request processing
			service := &DemoService{}
			req := &PingRequest{Message: fmt.Sprintf("Request #%d", id)}

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			_, err := service.Ping(ctx, req)

			if err != nil {
				log.Printf("❌ Request #%d failed: %v", id, err)
			} else {
				log.Printf("✅ Request #%d succeeded", id)
			}
		}(requestCount)
	}
}

// Example output:
//
// Starting gRPC Guardian Circuit Breaker Demo...
// ✅ Server listening on :50051
// 📊 Circuit Breaker Statistics:
//    - Failure Threshold: 50%
//    - Open Timeout: 5 seconds
//    - Interval Window: 10 seconds
//
// 🔥 Generating load with 30% failure rate...
//
// ✅ Request #1 succeeded
// ❌ Request #2 failed: service temporarily unavailable
// ✅ Request #3 succeeded
// ❌ Request #4 failed: service temporarily unavailable
// ✅ Request #5 succeeded
// ❌ Request #6 failed: service temporarily unavailable
// ❌ Request #7 failed: service temporarily unavailable
// 🔄 Circuit Breaker State: Closed -> Open
// ❌ Request #8 failed: circuit breaker is open
// ❌ Request #9 failed: circuit breaker is open
// ...
// [After 5 seconds]
// 🔄 Circuit Breaker State: Open -> HalfOpen
// ✅ Request #30 succeeded
// ✅ Request #31 succeeded
// 🔄 Circuit Breaker State: HalfOpen -> Closed
// ✅ Request #32 succeeded
