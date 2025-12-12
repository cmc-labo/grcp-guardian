package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"time"

	guardian "github.com/grpc-guardian/grpc-guardian"
	"github.com/grpc-guardian/grpc-guardian/middleware"
	"github.com/grpc-guardian/grpc-guardian/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	pb "google.golang.org/grpc/examples/helloworld/helloworld"
)

// server is used to implement helloworld.GreeterServer
type server struct {
	pb.UnimplementedGreeterServer
}

// SayHello implements helloworld.GreeterServer
func (s *server) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	// Simulate some processing time
	delay := time.Duration(rand.Intn(100)) * time.Millisecond
	time.Sleep(delay)

	// Randomly return errors to demonstrate error metrics
	if rand.Float32() < 0.1 { // 10% error rate
		return nil, status.Error(codes.Internal, "random error for testing")
	}

	log.Printf("Received: %v (processed in %v)", in.GetName(), delay)
	return &pb.HelloReply{Message: "Hello " + in.GetName()}, nil
}

func main() {
	// Create Prometheus metrics collector
	collector, err := metrics.NewPrometheusCollector(
		metrics.WithNamespace("grpc_guardian"),
		metrics.WithSubsystem("demo"),
		metrics.WithConstLabels(map[string]string{
			"service": "greeter",
			"version": "1.0.0",
		}),
	)
	if err != nil {
		log.Fatalf("Failed to create metrics collector: %v", err)
	}

	// Create middleware chain with metrics
	chain := guardian.NewChain(
		middleware.Logging(),
		middleware.MetricsMiddleware(collector),
	)

	// Create gRPC server
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(chain.UnaryInterceptor()),
	)

	// Register service
	pb.RegisterGreeterServer(grpcServer, &server{})
	reflection.Register(grpcServer)

	// Start metrics HTTP server
	go func() {
		http.Handle("/metrics", promhttp.HandlerFor(
			collector.GetRegistry(),
			promhttp.HandlerOpts{
				EnableOpenMetrics: true,
			},
		))

		// Add a simple status page
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>gRPC Guardian Metrics Demo</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        h1 { color: #333; }
        .container { max-width: 800px; margin: 0 auto; }
        .section { background: #f5f5f5; padding: 20px; margin: 20px 0; border-radius: 5px; }
        code { background: #eee; padding: 2px 6px; border-radius: 3px; }
        a { color: #0066cc; text-decoration: none; }
        a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🛡️ gRPC Guardian - Metrics Demo</h1>

        <div class="section">
            <h2>📊 Prometheus Metrics</h2>
            <p>Access Prometheus metrics at: <a href="/metrics">/metrics</a></p>
            <p>The server exposes the following metrics:</p>
            <ul>
                <li><code>grpc_guardian_demo_requests_total</code> - Total number of requests</li>
                <li><code>grpc_guardian_demo_request_duration_seconds</code> - Request latency histogram</li>
                <li><code>grpc_guardian_demo_active_requests</code> - Number of active requests</li>
                <li><code>grpc_guardian_demo_errors_total</code> - Total number of errors</li>
                <li><code>grpc_guardian_demo_message_sent_bytes</code> - Message size histogram (sent)</li>
                <li><code>grpc_guardian_demo_message_received_bytes</code> - Message size histogram (received)</li>
            </ul>
        </div>

        <div class="section">
            <h2>🧪 Testing</h2>
            <p>Test the gRPC service using grpcurl:</p>
            <pre><code>grpcurl -plaintext -d '{"name":"World"}' localhost:50051 helloworld.Greeter/SayHello</code></pre>

            <p>Generate load for testing:</p>
            <pre><code>for i in {1..100}; do
  grpcurl -plaintext -d '{"name":"User'$i'"}' localhost:50051 helloworld.Greeter/SayHello
  sleep 0.1
done</code></pre>
        </div>

        <div class="section">
            <h2>📈 Prometheus Configuration</h2>
            <p>Add this job to your <code>prometheus.yml</code>:</p>
            <pre><code>scrape_configs:
  - job_name: 'grpc-guardian-demo'
    static_configs:
      - targets: ['localhost:9090']</code></pre>
        </div>

        <div class="section">
            <h2>📝 Example Prometheus Queries</h2>
            <ul>
                <li><strong>Request rate:</strong> <code>rate(grpc_guardian_demo_requests_total[1m])</code></li>
                <li><strong>Error rate:</strong> <code>rate(grpc_guardian_demo_errors_total[1m])</code></li>
                <li><strong>P99 latency:</strong> <code>histogram_quantile(0.99, rate(grpc_guardian_demo_request_duration_seconds_bucket[1m]))</code></li>
                <li><strong>Active requests:</strong> <code>grpc_guardian_demo_active_requests</code></li>
            </ul>
        </div>
    </div>
</body>
</html>
			`)
		})

		log.Println("📊 Metrics server listening on :9090")
		log.Println("   - Metrics endpoint: http://localhost:9090/metrics")
		log.Println("   - Status page: http://localhost:9090/")
		if err := http.ListenAndServe(":9090", nil); err != nil {
			log.Fatalf("Failed to start metrics server: %v", err)
		}
	}()

	// Start gRPC server
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	log.Println("🚀 gRPC server listening on :50051")
	log.Println("")
	log.Println("Try these commands:")
	log.Println("  grpcurl -plaintext -d '{\"name\":\"World\"}' localhost:50051 helloworld.Greeter/SayHello")
	log.Println("  curl http://localhost:9090/metrics")
	log.Println("")

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
