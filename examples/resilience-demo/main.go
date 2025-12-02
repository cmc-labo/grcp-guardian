package main

import (
	"context"
	"fmt"
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

// ResilientService demonstrates a production-ready service with full resilience features
type ResilientService struct{}

// ProcessRequest handles a request with full resilience features
func (s *ResilientService) ProcessRequest(ctx context.Context, req *Request) (*Response, error) {
	// Business logic here
	return &Response{
		Status:  "success",
		Message: fmt.Sprintf("Processed: %s", req.Data),
	}, nil
}

// Request is the request message
type Request struct {
	Data string
}

// Response is the response message
type Response struct {
	Status  string
	Message string
}

func main() {
	log.Println("🚀 Starting Resilience Demo - Full Production Stack")
	log.Println("")

	// Check if chaos is enabled via environment variable
	enableChaos := os.Getenv("ENABLE_CHAOS") == "true"

	// Create resilience middleware stack
	var middlewares []func(context.Context, interface{}, *grpc.UnaryServerInfo, grpc.UnaryHandler) (interface{}, error)

	// 1. Logging - Always first for complete visibility
	middlewares = append(middlewares, middleware.Logging())

	// 2. Circuit Breaker - Protect against cascading failures
	middlewares = append(middlewares, middleware.CircuitBreakerMiddleware(
		middleware.WithFailureThreshold(0.5),   // Open after 50% failures
		middleware.WithTimeout(30*time.Second),  // Stay open for 30 seconds
		middleware.WithInterval(60*time.Second), // 60 second failure window
		middleware.WithMaxRequests(5),           // Allow 5 test requests in half-open
		middleware.WithSuccessThreshold(3),      // Need 3 successes to close
		middleware.WithOnStateChange(func(from, to middleware.State) {
			log.Printf("🔄 Circuit Breaker: %s -> %s", from, to)

			// In production, emit metrics here
			// metrics.CircuitBreakerStateChange(from.String(), to.String())
		}),
	))

	// 3. Rate Limiting - Protect against overload
	middlewares = append(middlewares, middleware.RateLimit(100, 10)) // 100 req/s, burst 10

	// 4. Chaos Engineering - Only in test/staging environments
	if enableChaos {
		log.Println("⚠️  CHAOS MODE ENABLED - Injecting failures!")
		log.Println("")

		middlewares = append(middlewares, chaos.New(
			// Inject latency into 20% of requests
			chaos.WithLatency(
				100*time.Millisecond,
				500*time.Millisecond,
				0.2,
			),
			// Inject errors into 10% of requests
			chaos.WithErrors(
				[]codes.Code{codes.Unavailable, codes.Internal},
				0.1,
			),
			// Inject timeouts into 5% of requests
			chaos.WithTimeout(
				2*time.Second,
				0.05,
			),
		))
	}

	// Create middleware chain
	chain := guardian.NewChain(middlewares...)

	// Create gRPC server with resilience features
	server := grpc.NewServer(
		grpc.UnaryInterceptor(chain.UnaryInterceptor()),
		grpc.MaxConcurrentStreams(1000),
		grpc.ConnectionTimeout(10*time.Second),
	)

	// Register service
	// In real application: pb.RegisterServiceServer(server, &ResilientService{})

	// Start server
	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("❌ Failed to listen: %v", err)
	}

	// Print configuration
	log.Println("📋 Server Configuration:")
	log.Println("   Port: 50052")
	log.Println("   Circuit Breaker: ✅ Enabled")
	log.Println("     - Failure Threshold: 50%")
	log.Println("     - Open Timeout: 30s")
	log.Println("     - Half-Open Max Requests: 5")
	log.Println("   Rate Limiting: ✅ Enabled (100 req/s)")
	log.Println("   Logging: ✅ Enabled")
	if enableChaos {
		log.Println("   Chaos Engineering: ⚠️  ENABLED")
		log.Println("     - Latency Injection: 20%")
		log.Println("     - Error Injection: 10%")
		log.Println("     - Timeout Injection: 5%")
	} else {
		log.Println("   Chaos Engineering: ❌ Disabled")
		log.Println("     (Set ENABLE_CHAOS=true to enable)")
	}
	log.Println("")
	log.Println("✅ Server ready to accept connections")
	log.Println("")

	if err := server.Serve(lis); err != nil {
		log.Fatalf("❌ Failed to serve: %v", err)
	}
}

/*
Usage Examples:

1. Normal production mode:
   $ go run main.go

2. With chaos engineering enabled:
   $ ENABLE_CHAOS=true go run main.go

3. Load testing with circuit breaker:
   $ go run main.go &
   $ # In another terminal:
   $ ghz --insecure --proto api.proto \
         --call api.Service/ProcessRequest \
         -d '{"data":"test"}' \
         -n 1000 -c 50 \
         localhost:50052

Expected Behavior:

Phase 1 - Closed State (Normal Operation):
- Requests processed normally
- Circuit breaker monitors failure rate
- If failure rate < 50%, stays closed

Phase 2 - Opening (High Failure Rate):
- When failure rate exceeds 50%
- Circuit breaker opens immediately
- New requests fail fast with "circuit breaker open" error
- Reduces load on struggling service

Phase 3 - Half-Open (Testing Recovery):
- After 30 seconds in open state
- Circuit transitions to half-open
- Allows 5 test requests through
- If 3 consecutive successes -> Close
- If any failure -> Back to Open

Phase 4 - Closed Again (Recovered):
- Service recovered and working normally
- Circuit breaker back to monitoring mode
- Normal request processing resumes

Benefits Demonstrated:

1. Fail Fast:
   - Open circuit prevents wasting resources on failing calls
   - Immediate feedback to clients

2. Automatic Recovery:
   - System automatically tests if service recovered
   - No manual intervention needed

3. Cascading Failure Prevention:
   - Stops bad requests from overwhelming downstream services
   - Preserves system stability

4. Observable:
   - State change logging
   - Detailed statistics
   - Easy to monitor and alert on

5. Configurable:
   - Adjust thresholds per service needs
   - Fine-tune recovery behavior
   - Environment-specific settings
*/
