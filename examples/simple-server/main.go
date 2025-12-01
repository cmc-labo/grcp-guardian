package main

import (
	"context"
	"log"
	"net"
	"time"

	guardian "github.com/grpc-guardian/grpc-guardian"
	"github.com/grpc-guardian/grpc-guardian/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Example service implementation
type ExampleService struct{}

// SayHello is a simple RPC method
func (s *ExampleService) SayHello(ctx context.Context, req *HelloRequest) (*HelloResponse, error) {
	// Simulate some processing
	time.Sleep(50 * time.Millisecond)

	return &HelloResponse{
		Message: "Hello, " + req.Name + "!",
	}, nil
}

// SlowMethod simulates a slow operation
func (s *ExampleService) SlowMethod(ctx context.Context, req *SlowRequest) (*SlowResponse, error) {
	// Simulate expensive operation
	time.Sleep(2 * time.Second)

	return &SlowResponse{
		Result: "Completed after 2 seconds",
	}, nil
}

// ProtectedMethod requires authentication
func (s *ExampleService) ProtectedMethod(ctx context.Context, req *ProtectedRequest) (*ProtectedResponse, error) {
	// Get user ID from context (set by auth middleware)
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	return &ProtectedResponse{
		Message: "Hello authenticated user: " + userID,
		Data:    req.Data,
	}, nil
}

// Simple proto message types (normally generated from .proto files)
type HelloRequest struct {
	Name string
}

type HelloResponse struct {
	Message string
}

type SlowRequest struct {
	Data string
}

type SlowResponse struct {
	Result string
}

type ProtectedRequest struct {
	Data string
}

type ProtectedResponse struct {
	Message string
	Data    string
}

func main() {
	// Create middleware chain with all features
	chain := guardian.NewChain(
		// 1. Logging - log all requests
		middleware.Logging(
			middleware.WithLevel(0), // Info level
		),

		// 2. Performance monitoring - log slow requests
		middleware.PerformanceLog(1 * time.Second),

		// 3. Authentication - JWT validation
		// (commented out for this example - requires valid JWT tokens)
		// middleware.Auth(
		// 	middleware.JWTValidator("your-secret-key"),
		// ),

		// 4. Rate limiting - 100 requests per second, burst of 10
		middleware.RateLimit(100, 10),

		// 5. Per-method rate limiting
		middleware.RateLimitPerMethod(
			1000, // default: 1000 req/sec
			50,   // default burst: 50
			map[string]struct{ Rate, Burst int }{
				"/example.ExampleService/SlowMethod": {Rate: 10, Burst: 2},
			},
		),
	)

	// Create gRPC server with middleware
	server := grpc.NewServer(
		grpc.UnaryInterceptor(chain.UnaryInterceptor()),
	)

	// Register service
	// In a real application, you would use:
	// pb.RegisterExampleServiceServer(server, &ExampleService{})

	// Start server
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	log.Println("gRPC Guardian Example Server")
	log.Println("=============================")
	log.Println("Server starting on :50051")
	log.Println()
	log.Println("Middleware enabled:")
	log.Println("  ✓ Logging")
	log.Println("  ✓ Performance monitoring")
	log.Println("  ✓ Global rate limiting (100 req/s, burst 10)")
	log.Println("  ✓ Per-method rate limiting")
	log.Println()
	log.Println("Available methods:")
	log.Println("  - SayHello: Simple greeting method")
	log.Println("  - SlowMethod: Simulates slow operation (rate limited to 10 req/s)")
	log.Println("  - ProtectedMethod: Requires authentication")
	log.Println()

	if err := server.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
