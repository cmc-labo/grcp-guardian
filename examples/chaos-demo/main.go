package main

import (
	"context"
	"log"
	"net"
	"os"
	"time"

	guardian "github.com/grpc-guardian/grpc-guardian"
	"github.com/grpc-guardian/grpc-guardian/chaos"
	"github.com/grpc-guardian/grpc-guardian/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// ReliableService is a service we want to test with chaos
type ReliableService struct{}

type Request struct {
	Data string
}

type Response struct {
	Result string
}

func (s *ReliableService) Process(ctx context.Context, req *Request) (*Response, error) {
	return &Response{
		Result: "Processed: " + req.Data,
	}, nil
}

func main() {
	// Check if chaos mode is enabled
	enableChaos := os.Getenv("ENABLE_CHAOS") == "true"

	log.Println("gRPC Guardian - Chaos Engineering Demo")
	log.Println("======================================")
	log.Printf("Chaos mode: %v\n", enableChaos)
	log.Println()

	// Create middleware chain with chaos engineering
	var chaosMiddlewares []func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error)

	if enableChaos {
		log.Println("Chaos experiments enabled:")
		log.Println()

		// Scenario 1: Latency injection
		log.Println("1. Latency Injection")
		log.Println("   - 30% of requests will have 100-500ms delay")
		chaosMiddlewares = append(chaosMiddlewares,
			chaos.LatencyInjector(100*time.Millisecond, 500*time.Millisecond, 0.3),
		)

		// Scenario 2: Error injection
		log.Println("2. Error Injection")
		log.Println("   - 10% of requests will return Unavailable errors")
		log.Println("   - 5% of requests will return Internal errors")
		chaosMiddlewares = append(chaosMiddlewares,
			chaos.ErrorInjector(
				[]codes.Code{codes.Unavailable, codes.Internal},
				0.15,
			),
		)

		// Scenario 3: Timeout simulation
		log.Println("3. Timeout Simulation")
		log.Println("   - 5% of requests will timeout after 1 second")
		chaosMiddlewares = append(chaosMiddlewares,
			chaos.TimeoutInjector(1*time.Second, 0.05),
		)

		// Scenario 4: Combined chaos (flaky network)
		log.Println("4. Flaky Network Simulation")
		log.Println("   - Random combination of latency and errors")
		chaosMiddlewares = append(chaosMiddlewares,
			chaos.FlakyChaos(0.2),
		)

		log.Println()
		log.Println("Circuit breaker enabled to handle failures")
	} else {
		log.Println("Running in normal mode (set ENABLE_CHAOS=true to enable chaos)")
	}

	// Build middleware chain
	middlewareChain := []func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error){
		// Logging
		middleware.Logging(),

		// Add chaos middlewares if enabled
	}
	middlewareChain = append(middlewareChain, chaosMiddlewares...)

	// Add circuit breaker to handle chaos-induced failures
	// (In a real implementation, you would add circuit breaker middleware here)

	chain := guardian.NewChain(middlewareChain...)

	// Create gRPC server
	server := grpc.NewServer(
		grpc.UnaryInterceptor(chain.UnaryInterceptor()),
	)

	// Register service
	// In a real application:
	// pb.RegisterReliableServiceServer(server, &ReliableService{})

	// Start server
	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	log.Println()
	log.Println("Server starting on :50052")
	log.Println("Press Ctrl+C to stop")
	log.Println()
	log.Println("Test the chaos:")
	log.Println("  1. Normal mode: ENABLE_CHAOS=false go run main.go")
	log.Println("  2. Chaos mode:  ENABLE_CHAOS=true go run main.go")
	log.Println()
	log.Println("Send requests and observe:")
	log.Println("  - Random latencies")
	log.Println("  - Random errors")
	log.Println("  - Timeouts")
	log.Println("  - Circuit breaker behavior")
	log.Println()

	if err := server.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
