package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	guardian "github.com/grpc-guardian/grpc-guardian"
	"github.com/grpc-guardian/grpc-guardian/middleware"
	"github.com/grpc-guardian/grpc-guardian/pkg/tracing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Simple gRPC service implementation
type echoServer struct{}

// Echo RPC implementation
func (s *echoServer) Echo(ctx context.Context, req *EchoRequest) (*EchoResponse, error) {
	// Add custom span attributes
	middleware.SetSpanAttribute(ctx, "request.message", req.Message)

	// Add span events
	middleware.AddEventToSpan(ctx, "processing_message")

	// Simulate some work
	time.Sleep(50 * time.Millisecond)

	// Create a child span for business logic
	ctx, span := middleware.StartSpan(ctx, "business-logic")
	defer span.End()

	// More business logic
	result := fmt.Sprintf("Echo: %s", req.Message)

	middleware.AddEventToSpan(ctx, "message_processed")

	return &EchoResponse{Message: result}, nil
}

// SlowEcho demonstrates slow request tracing
func (s *echoServer) SlowEcho(ctx context.Context, req *EchoRequest) (*EchoResponse, error) {
	middleware.AddEventToSpan(ctx, "slow_operation_started")

	// Simulate slow operation
	time.Sleep(2 * time.Second)

	middleware.AddEventToSpan(ctx, "slow_operation_completed")

	return &EchoResponse{Message: "Slow: " + req.Message}, nil
}

// FailingEcho demonstrates error tracing
func (s *echoServer) FailingEcho(ctx context.Context, req *EchoRequest) (*EchoResponse, error) {
	middleware.AddEventToSpan(ctx, "operation_failed")

	err := status.Error(codes.Internal, "simulated internal error")

	// Record the error in the span
	middleware.RecordError(ctx, err)

	return nil, err
}

// Simple protobuf message definitions (normally generated)
type EchoRequest struct {
	Message string
}

type EchoResponse struct {
	Message string
}

func main() {
	fmt.Println("=== gRPC Guardian - Distributed Tracing Demo ===\n")

	// Initialize Jaeger tracing
	fmt.Println("Initializing Jaeger tracing...")
	tp, err := tracing.InitJaeger(
		tracing.WithServiceName("echo-service"),
		tracing.WithServiceVersion("1.0.0"),
		tracing.WithEnvironment("demo"),
		tracing.WithCollectorEndpoint("http://localhost:14268/api/traces"),
		tracing.WithSamplingRate(1.0), // Sample all traces for demo
		tracing.WithAttribute("team", "platform"),
		tracing.WithAttribute("component", "grpc-server"),
	)
	if err != nil {
		log.Fatalf("Failed to initialize tracing: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tracing.Shutdown(ctx, tp); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}()

	fmt.Println("✓ Tracing initialized with Jaeger")
	fmt.Println("  Jaeger UI: http://localhost:16686")
	fmt.Println()

	// Create middleware chain with tracing
	chain := guardian.NewChain(
		// Logging middleware (logs will include trace IDs)
		middleware.Logging(),

		// Tracing middleware - this is the key component
		middleware.TracingWithServiceName(
			"echo-service",
			middleware.WithRecordErrors(),
			middleware.WithRecordEvents(),
		),

		// Other middleware can be added here
		middleware.Timeout(5 * time.Second),
	)

	// Create gRPC server with middleware
	server := grpc.NewServer(
		grpc.UnaryInterceptor(chain.UnaryInterceptor()),
	)

	// Register service
	// In real implementation, you would use:
	// pb.RegisterEchoServiceServer(server, &echoServer{})

	// Start server
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	fmt.Println("Server Configuration:")
	fmt.Println("  Address: localhost:50051")
	fmt.Println("  Middleware: Logging → Tracing → Timeout")
	fmt.Println()

	fmt.Println("Starting gRPC server with distributed tracing...")
	fmt.Println()

	fmt.Println("Example Client Code:")
	fmt.Println("---------------------")
	fmt.Println(`
	// Create client connection with tracing
	conn, err := grpc.Dial("localhost:50051",
		grpc.WithInsecure(),
		grpc.WithUnaryInterceptor(tracingClientInterceptor()),
	)

	// Make RPC call
	ctx := context.Background()
	resp, err := client.Echo(ctx, &EchoRequest{Message: "Hello"})

	// Traces will be automatically sent to Jaeger
	`)
	fmt.Println()

	fmt.Println("Tracing Features Demonstrated:")
	fmt.Println("  ✓ Automatic span creation for each RPC")
	fmt.Println("  ✓ Context propagation across service boundaries")
	fmt.Println("  ✓ Custom span attributes and events")
	fmt.Println("  ✓ Error recording in spans")
	fmt.Println("  ✓ Performance tracking (slow requests)")
	fmt.Println("  ✓ Service topology visualization in Jaeger UI")
	fmt.Println()

	fmt.Println("Test Scenarios:")
	fmt.Println("  1. Normal Echo - Standard request with tracing")
	fmt.Println("  2. Slow Echo - Request that takes >1s (visible in Jaeger)")
	fmt.Println("  3. Failing Echo - Error request (error span in Jaeger)")
	fmt.Println()

	fmt.Println("View traces in Jaeger UI at: http://localhost:16686")
	fmt.Println("  Service: echo-service")
	fmt.Println("  Search for recent traces")
	fmt.Println()

	fmt.Println("Press Ctrl+C to stop the server")
	fmt.Println("==========================================")

	if err := server.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

// Example of client-side tracing interceptor
func tracingClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// Inject trace context into outgoing request
		ctx = middleware.InjectTraceContext(ctx)

		// Start client span
		ctx, span := middleware.StartSpan(ctx, method)
		defer span.End()

		// Call the actual RPC
		err := invoker(ctx, method, req, reply, cc, opts...)

		// Record error if any
		if err != nil {
			middleware.RecordError(ctx, err)
		}

		return err
	}
}
