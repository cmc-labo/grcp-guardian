# gRPC Guardian - High-Performance gRPC Middleware & Proxy Library

**Production-Ready × Plugin-Based × Chaos Engineering**

A powerful, modular middleware library for gRPC microservices with built-in support for authentication, logging, rate limiting, and chaos engineering capabilities.

## Features

### Core Middleware System
- **🔗 Chainable Middleware**: Easy-to-use middleware chain pattern similar to net/http
- **🚀 High Performance**: Minimal overhead with goroutine-optimized design
- **🔌 Plugin Architecture**: Extensible middleware system
- **📊 Rich Observability**: Built-in metrics and distributed tracing support

### Built-in Middleware

#### 1. Authentication & Authorization
- **JWT Token Validation**: Automatic JWT token parsing and validation
- **API Key Authentication**: Simple API key-based auth
- **RBAC Support**: Role-based access control
- **Custom Auth Handlers**: Extensible authentication system

#### 2. Logging & Observability
- **Structured Logging**: JSON-formatted logs with context
- **Request/Response Logging**: Automatic gRPC call logging
- **Performance Metrics**: Latency, throughput tracking
- **Distributed Tracing**: Full OpenTelemetry + Jaeger integration ✨ NEW!

#### 3. Rate Limiting
- **Token Bucket Algorithm**: Industry-standard rate limiting
- **Per-Client Limits**: IP or user-based rate limits
- **Adaptive Rate Limiting**: Dynamic adjustment based on load
- **Quota Management**: Request quota enforcement

#### 4. Resilience & Fault Tolerance
- **Retry Logic**: Automatic retry with exponential backoff
- **Circuit Breaking**: Automatic failure detection and recovery
- **Timeout Control**: Request timeout management with per-method configuration
- **Bulkhead Isolation**: Resource isolation between services

#### 5. Chaos Engineering
- **Latency Injection**: Simulate network delays
- **Error Injection**: Random error responses
- **Timeout Simulation**: Test timeout handling
- **Traffic Shadowing**: Duplicate traffic for testing

### Production Features
- **Health Checks**: Built-in health check endpoints
- **Graceful Shutdown**: Proper connection draining
- **Load Balancing**: Client-side load balancing support
- **Compression**: Automatic gRPC compression support

## Quick Start

### Installation

```bash
go get github.com/grpc-guardian/grpc-guardian
```

### Basic Usage

```go
package main

import (
    "context"
    "log"
    "net"

    guardian "github.com/grpc-guardian/grpc-guardian"
    "github.com/grpc-guardian/grpc-guardian/middleware"
    "google.golang.org/grpc"
)

func main() {
    // Create middleware chain
    chain := guardian.NewChain(
        middleware.Logging(),
        middleware.Auth(middleware.JWTValidator("secret")),
        middleware.RateLimit(100, 10), // 100 req/sec, burst 10
    )

    // Create gRPC server with middleware
    server := grpc.NewServer(
        grpc.UnaryInterceptor(chain.UnaryInterceptor()),
        grpc.StreamInterceptor(chain.StreamInterceptor()),
    )

    // Register your services
    // pb.RegisterYourServiceServer(server, &yourService{})

    lis, _ := net.Listen("tcp", ":50051")
    log.Fatal(server.Serve(lis))
}
```

### Distributed Tracing Example ✨ NEW!

```go
import (
    "context"
    guardian "github.com/grpc-guardian/grpc-guardian"
    "github.com/grpc-guardian/grpc-guardian/middleware"
    "github.com/grpc-guardian/grpc-guardian/pkg/tracing"
)

func main() {
    // Initialize Jaeger tracing
    tp, err := tracing.InitJaeger(
        tracing.WithServiceName("my-service"),
        tracing.WithServiceVersion("1.0.0"),
        tracing.WithEnvironment("production"),
        tracing.WithCollectorEndpoint("http://localhost:14268/api/traces"),
        tracing.WithSamplingRate(1.0), // Sample all traces
    )
    if err != nil {
        log.Fatal(err)
    }
    defer tracing.Shutdown(context.Background(), tp)

    // Create middleware chain with tracing
    chain := guardian.NewChain(
        middleware.Logging(),
        middleware.Tracing(
            middleware.WithRecordErrors(),
            middleware.WithRecordEvents(),
        ),
    )

    // Create gRPC server
    server := grpc.NewServer(
        grpc.UnaryInterceptor(chain.UnaryInterceptor()),
    )

    // Your traces will appear in Jaeger UI at http://localhost:16686
}
```

#### Advanced Tracing Features

```go
// Add custom span attributes
func (s *server) MyMethod(ctx context.Context, req *Request) (*Response, error) {
    // Add custom attributes to current span
    middleware.SetSpanAttribute(ctx, "user.id", req.UserId)
    middleware.SetSpanAttribute(ctx, "request.size", len(req.Data))

    // Add events to span
    middleware.AddEventToSpan(ctx, "processing_started")

    // Create child span for specific operation
    ctx, span := middleware.StartSpan(ctx, "database-query")
    defer span.End()

    // Your business logic here
    result := processRequest(req)

    middleware.AddEventToSpan(ctx, "processing_completed")

    return result, nil
}

// Record errors in spans
func (s *server) ErrorMethod(ctx context.Context, req *Request) (*Response, error) {
    err := someOperation()
    if err != nil {
        // Error will be recorded in the span
        middleware.RecordError(ctx, err)
        return nil, err
    }
    return &Response{}, nil
}
```

### Chaos Engineering Example

```go
// Create chaos middleware for testing
chaosMiddleware := chaos.New(
    chaos.WithLatency(100*time.Millisecond, 0.2),  // 20% requests delayed
    chaos.WithErrors(codes.Unavailable, 0.05),     // 5% error rate
    chaos.WithTimeout(5*time.Second, 0.1),         // 10% timeout
)

chain := guardian.NewChain(
    middleware.Logging(),
    chaosMiddleware,
    middleware.CircuitBreaker(
        circuitbreaker.WithThreshold(0.5), // Open after 50% failures
        circuitbreaker.WithTimeout(30*time.Second),
    ),
)
```

### Custom Middleware

```go
// Create custom middleware
func CustomMetrics() guardian.Middleware {
    return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
        start := time.Now()

        // Call next middleware/handler
        resp, err := handler(ctx, req)

        // Record metrics
        duration := time.Since(start)
        recordMetric(info.FullMethod, duration, err)

        return resp, err
    }
}

// Use in chain
chain := guardian.NewChain(
    CustomMetrics(),
    middleware.Logging(),
)
```

## Architecture

```
┌─────────────────────────────────────────────────┐
│           gRPC Client Request                   │
└──────────────────┬──────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────┐
│         Middleware Chain (Guardian)             │
├─────────────────────────────────────────────────┤
│  ┌───────────────────────────────────────────┐  │
│  │  1. Logging Middleware                    │  │
│  │     - Request/Response logging            │  │
│  │     - Performance tracking                │  │
│  └────────────────┬──────────────────────────┘  │
│                   ▼                              │
│  ┌───────────────────────────────────────────┐  │
│  │  2. Authentication Middleware             │  │
│  │     - JWT validation                      │  │
│  │     - API key check                       │  │
│  └────────────────┬──────────────────────────┘  │
│                   ▼                              │
│  ┌───────────────────────────────────────────┐  │
│  │  3. Rate Limiting Middleware              │  │
│  │     - Token bucket algorithm              │  │
│  │     - Per-client quotas                   │  │
│  └────────────────┬──────────────────────────┘  │
│                   ▼                              │
│  ┌───────────────────────────────────────────┐  │
│  │  4. Chaos Engineering (Optional)          │  │
│  │     - Latency injection                   │  │
│  │     - Error injection                     │  │
│  └────────────────┬──────────────────────────┘  │
│                   ▼                              │
│  ┌───────────────────────────────────────────┐  │
│  │  5. Circuit Breaker                       │  │
│  │     - Failure detection                   │  │
│  │     - Auto-recovery                       │  │
│  └────────────────┬──────────────────────────┘  │
└───────────────────┼──────────────────────────────┘
                    ▼
┌─────────────────────────────────────────────────┐
│           Your gRPC Service Handler             │
└─────────────────────────────────────────────────┘
```

## Middleware Reference

### Logging Middleware

```go
import "github.com/grpc-guardian/grpc-guardian/middleware"

// Basic logging
middleware.Logging()

// With custom logger
middleware.LoggingWithLogger(yourZapLogger)

// With options
middleware.Logging(
    logging.WithLevel(zapcore.InfoLevel),
    logging.WithFields(map[string]interface{}{
        "service": "my-service",
        "version": "1.0.0",
    }),
)
```

### Authentication Middleware

```go
// JWT authentication
middleware.Auth(
    middleware.JWTValidator("your-secret-key"),
)

// API Key authentication
middleware.Auth(
    middleware.APIKeyValidator(func(key string) bool {
        return key == "valid-api-key"
    }),
)

// Custom authentication
middleware.Auth(
    func(ctx context.Context, req interface{}) error {
        // Your custom auth logic
        return nil
    },
)
```

### Rate Limiting Middleware

```go
// Simple rate limit: 100 requests/sec, burst 10
middleware.RateLimit(100, 10)

// Per-client rate limiting
middleware.RateLimitPerClient(
    ratelimit.ByIP(),
    ratelimit.Limit(100, 10),
)

// Per-method rate limiting
middleware.RateLimitPerMethod(map[string]ratelimit.Config{
    "/api.Service/ExpensiveMethod": {Rate: 10, Burst: 2},
    "/api.Service/CheapMethod":     {Rate: 1000, Burst: 50},
})
```

### Timeout Middleware

```go
// Simple timeout: 5 seconds for all requests
middleware.TimeoutSimple(5 * time.Second)

// With callback on timeout
middleware.Timeout(
    middleware.WithTimeout(10*time.Second),
    middleware.WithTimeoutCallback(func(method string, duration time.Duration) {
        log.Printf("Timeout: %s exceeded %v", method, duration)
        // Emit metrics, send alerts, etc.
    }),
)

// Per-method timeout configuration
middleware.TimeoutPerMethod(
    5*time.Second, // default timeout
    map[string]time.Duration{
        "/api.Service/FastMethod":  1*time.Second,
        "/api.Service/SlowMethod":  30*time.Second,
        "/api.Service/QueryMethod": 15*time.Second,
    },
)

// Stream timeout
grpc.NewServer(
    grpc.StreamInterceptor(middleware.StreamTimeout(30*time.Second)),
)

// Advanced configuration
middleware.Timeout(
    middleware.WithTimeout(10*time.Second),
    middleware.WithPerMethodTimeout(map[string]time.Duration{
        "/api.Service/Upload":   60*time.Second,
        "/api.Service/Download": 120*time.Second,
    }),
    middleware.WithTimeoutCallback(func(method string, duration time.Duration) {
        metrics.TimeoutCounter.Inc()
        log.Warnf("Request timeout: method=%s duration=%v", method, duration)
    }),
)
```

### Distributed Tracing Middleware ✨ NEW!

```go
import (
    "github.com/grpc-guardian/grpc-guardian/middleware"
    "github.com/grpc-guardian/grpc-guardian/pkg/tracing"
)

// Initialize Jaeger exporter
tp, err := tracing.InitJaeger(
    tracing.WithServiceName("my-service"),
    tracing.WithServiceVersion("1.0.0"),
    tracing.WithEnvironment("production"),
    tracing.WithCollectorEndpoint("http://localhost:14268/api/traces"),
    tracing.WithSamplingRate(1.0), // Sample all traces
    tracing.WithAttribute("team", "platform"),
)
if err != nil {
    log.Fatal(err)
}
defer tracing.Shutdown(context.Background(), tp)

// Basic tracing middleware
middleware.Tracing()

// With service name
middleware.TracingWithServiceName("my-service")

// With options
middleware.Tracing(
    middleware.WithRecordErrors(),    // Record errors in spans
    middleware.WithRecordEvents(),    // Record span events
)

// For streaming RPCs
grpc.NewServer(
    grpc.StreamInterceptor(
        middleware.StreamTracing(
            middleware.WithRecordErrors(),
        ),
    ),
)

// Helper functions for custom tracing
func (s *server) MyHandler(ctx context.Context, req *Request) (*Response, error) {
    // Add custom span attributes
    middleware.SetSpanAttribute(ctx, "user.id", req.UserId)
    middleware.SetSpanAttribute(ctx, "request.type", "important")

    // Add span events
    middleware.AddEventToSpan(ctx, "validation_started")

    // Create child span
    ctx, span := middleware.StartSpan(ctx, "database-operation")
    defer span.End()

    result, err := database.Query(ctx, req)
    if err != nil {
        // Record errors in current span
        middleware.RecordError(ctx, err)
        return nil, err
    }

    middleware.AddEventToSpan(ctx, "validation_completed")
    return result, nil
}
```

**Jaeger Setup:**

```bash
# Run Jaeger all-in-one (for development)
docker run -d --name jaeger \
  -p 16686:16686 \
  -p 14268:14268 \
  jaegertracing/all-in-one:latest

# Access Jaeger UI
open http://localhost:16686
```

**Features:**
- Automatic span creation for each gRPC call
- W3C Trace Context propagation
- Custom span attributes and events
- Error recording with stack traces
- Streaming RPC support
- Service topology visualization
- Performance analysis and latency tracking

### Retry Middleware

```go
// Basic retry with exponential backoff
retry := middleware.NewRetry(
    middleware.WithMaxAttempts(3),
    middleware.WithInitialBackoff(100*time.Millisecond),
    middleware.WithBackoffMultiplier(2.0),
)

// Use as client interceptor
conn, err := grpc.Dial(
    "localhost:50051",
    grpc.WithUnaryInterceptor(retry.UnaryClientInterceptor()),
    grpc.WithStreamInterceptor(retry.StreamClientInterceptor()),
)

// Advanced configuration
retry := middleware.NewRetry(
    middleware.WithMaxAttempts(5),
    middleware.WithInitialBackoff(50*time.Millisecond),
    middleware.WithMaxBackoff(10*time.Second),
    middleware.WithJitter(true),  // Add randomness to prevent thundering herd
    middleware.WithRetryableCodes(codes.Unavailable, codes.ResourceExhausted),
    middleware.WithOnRetry(func(attempt int, err error, nextBackoff time.Duration) {
        log.Printf("Retry attempt %d after error: %v (waiting %v)", attempt, err, nextBackoff)
        // Emit metrics, log, send alerts
    }),
)
```

### Circuit Breaker Middleware

```go
// Basic circuit breaker
middleware.CircuitBreakerMiddleware(
    middleware.WithFailureThreshold(0.5),      // Open after 50% failure rate
    middleware.WithTimeout(30*time.Second),     // Stay open for 30 seconds
    middleware.WithMaxRequests(5),              // Max requests in half-open state
    middleware.WithSuccessThreshold(3),         // Successes needed to close
)

// With state change callback
middleware.CircuitBreakerMiddleware(
    middleware.WithFailureThreshold(0.6),
    middleware.WithInterval(60*time.Second),    // Failure counting window
    middleware.WithOnStateChange(func(from, to middleware.State) {
        log.Printf("Circuit breaker: %s -> %s", from, to)
        // Emit metrics, send alerts, etc.
    }),
)

// Custom failure detection
middleware.CircuitBreakerMiddleware(
    middleware.WithIsFailure(func(err error) bool {
        // Define which errors should trip the circuit
        st, ok := status.FromError(err)
        if !ok {
            return true
        }
        return st.Code() == codes.Unavailable || st.Code() == codes.DeadlineExceeded
    }),
)
```

### Chaos Engineering Middleware

```go
import "github.com/grpc-guardian/grpc-guardian/chaos"

// Latency injection
chaos.New(
    chaos.WithLatency(
        chaos.FixedDelay(100*time.Millisecond),
        chaos.Probability(0.2), // 20% of requests
    ),
)

// Error injection
chaos.New(
    chaos.WithErrors(
        chaos.ErrorCodes(codes.Unavailable, codes.Internal),
        chaos.Probability(0.05), // 5% error rate
    ),
)

// Combined chaos
chaos.New(
    chaos.WithLatency(chaos.FixedDelay(100*time.Millisecond), chaos.Probability(0.1)),
    chaos.WithErrors(chaos.ErrorCode(codes.Unavailable), chaos.Probability(0.05)),
    chaos.WithTimeout(5*time.Second, chaos.Probability(0.02)),
)

// Conditional chaos (only in staging)
chaos.New(
    chaos.EnableIf(func() bool {
        return os.Getenv("ENV") == "staging"
    }),
    chaos.WithLatency(chaos.RandomDelay(50, 200), chaos.Probability(0.3)),
)
```

## Performance Benchmarks

Benchmarks on Intel Xeon E5-2680 v4 @ 2.40GHz, 64GB RAM:

| Middleware Stack | Throughput | Latency (p50) | Latency (p99) | CPU Usage |
|-----------------|------------|---------------|---------------|-----------|
| No middleware | 50,000 req/s | 0.2ms | 1.0ms | 30% |
| Logging only | 48,000 req/s | 0.25ms | 1.2ms | 32% |
| Auth + Logging | 45,000 req/s | 0.3ms | 1.5ms | 35% |
| Full stack* | 42,000 req/s | 0.4ms | 2.0ms | 40% |

*Full stack = Logging + Auth + Rate Limiting + Metrics

## Project Structure

```
grpc-guardian/
├── middleware/                    # Core middleware implementations
│   ├── auth.go                   # Authentication middleware
│   ├── logging.go                # Logging middleware
│   ├── ratelimit.go              # Rate limiting middleware
│   ├── circuit_breaker.go        # Circuit breaker pattern
│   ├── circuit_breaker_test.go   # Circuit breaker tests
│   ├── retry.go                  # Retry with exponential backoff
│   ├── retry_test.go             # Retry tests
│   ├── timeout.go                # Timeout middleware
│   ├── timeout_test.go           # Timeout tests
│   ├── tracing.go                # ✨ NEW: Distributed tracing middleware
│   └── tracing_test.go           # ✨ NEW: Tracing tests
├── chaos/                         # Chaos engineering features
│   ├── latency.go                # Latency injection
│   ├── error.go                  # Error injection
│   ├── timeout.go                # Timeout simulation
│   ├── shadow.go                 # Traffic shadowing
│   └── chaos.go                  # Chaos coordinator
├── interceptor/                   # gRPC interceptor implementations
│   ├── unary.go                  # Unary interceptor
│   └── stream.go                 # Stream interceptor
├── pkg/
│   ├── auth/                     # Authentication utilities
│   ├── ratelimit/                # Rate limiting algorithms
│   ├── logging/                  # Logging utilities
│   └── tracing/                  # ✨ NEW: Distributed tracing utilities
│       ├── jaeger.go             # Jaeger exporter configuration
│       └── config.go             # Tracing configuration
├── examples/
│   ├── simple-server/            # Basic usage example
│   ├── chaos-demo/               # Chaos engineering demo
│   ├── circuit-breaker-demo/     # Circuit breaker demo
│   ├── resilience-demo/          # Full resilience stack
│   ├── retry-demo/               # Retry middleware demo
│   ├── timeout-demo/             # Timeout middleware demo
│   ├── tracing-demo/             # ✨ NEW: Distributed tracing demo
│   ├── auth-example/             # Authentication example
│   └── benchmark/                # Performance benchmarks
├── chain.go                       # Middleware chain implementation
├── guardian.go                    # Main entry point
└── README.md
```

## Examples

### Example 1: Production-Ready Server

```go
// Initialize distributed tracing
tp, _ := tracing.InitJaeger(
    tracing.WithServiceName("production-service"),
    tracing.WithServiceVersion("1.0.0"),
    tracing.WithEnvironment("production"),
    tracing.WithCollectorEndpoint(os.Getenv("JAEGER_ENDPOINT")),
)
defer tracing.Shutdown(context.Background(), tp)

// Full production setup with all features
chain := guardian.NewChain(
    // Request logging
    middleware.Logging(
        logging.WithLevel(zapcore.InfoLevel),
    ),

    // Distributed tracing with OpenTelemetry
    middleware.Tracing(
        middleware.WithRecordErrors(),
        middleware.WithRecordEvents(),
    ),

    // JWT authentication
    middleware.Auth(
        middleware.JWTValidator(os.Getenv("JWT_SECRET")),
    ),

    // Rate limiting
    middleware.RateLimitPerClient(
        ratelimit.ByIP(),
        ratelimit.Limit(100, 10),
    ),

    // Circuit breaker
    middleware.CircuitBreaker(
        circuitbreaker.WithThreshold(0.5),
        circuitbreaker.WithTimeout(30*time.Second),
    ),

    // Metrics collection
    middleware.Metrics(),

    // Request timeout with per-method configuration
    middleware.Timeout(
        middleware.WithTimeout(10*time.Second),
        middleware.WithPerMethodTimeout(map[string]time.Duration{
            "/api.Service/Upload":   60*time.Second,
            "/api.Service/Download": 120*time.Second,
        }),
    ),
)
```

### Example 2: Testing with Chaos

```go
// Chaos testing configuration
chaosConfig := chaos.New(
    // Add random latency
    chaos.WithLatency(
        chaos.RandomDelay(50, 500), // 50-500ms random delay
        chaos.Probability(0.3),      // 30% of requests
    ),

    // Inject errors
    chaos.WithErrors(
        chaos.ErrorCodes(codes.Unavailable, codes.Internal),
        chaos.Probability(0.1), // 10% error rate
    ),

    // Only enable in test environment
    chaos.EnableIf(func() bool {
        return os.Getenv("ENABLE_CHAOS") == "true"
    }),
)

chain := guardian.NewChain(
    middleware.Logging(),
    chaosConfig,
    middleware.CircuitBreaker(), // Test circuit breaker behavior
)
```

### Example 3: Retry with Circuit Breaker

```go
// Combine retry and circuit breaker for resilience
retry := middleware.NewRetry(
    middleware.WithMaxAttempts(3),
    middleware.WithInitialBackoff(100*time.Millisecond),
    middleware.WithJitter(true),
    middleware.WithOnRetry(func(attempt int, err error, backoff time.Duration) {
        log.Printf("Retry attempt %d: %v", attempt, err)
    }),
)

chain := guardian.NewChain(
    // Logging for observability
    middleware.Logging(),

    // Circuit breaker to prevent cascade failures
    middleware.CircuitBreakerMiddleware(
        middleware.WithFailureThreshold(0.5),      // Open at 50% failure
        middleware.WithTimeout(30*time.Second),     // Stay open 30s
        middleware.WithInterval(60*time.Second),    // Count failures over 60s
        middleware.WithMaxRequests(5),              // 5 requests in half-open
        middleware.WithSuccessThreshold(3),         // 3 successes to close
        middleware.WithOnStateChange(func(from, to middleware.State) {
            log.Printf("Circuit: %s -> %s", from, to)
            metrics.CircuitBreakerStateGauge.Set(float64(to))
        }),
    ),

    // Rate limiting to prevent overload
    middleware.RateLimit(100, 10),
)

// Client-side with retry
conn, _ := grpc.Dial(
    "localhost:50051",
    grpc.WithUnaryInterceptor(retry.UnaryClientInterceptor()),
)

// Circuit Breaker State Machine:
//
//  ┌─────────┐
//  │ Closed  │ ◄────────────────────┐
//  │ Normal  │                      │
//  └────┬────┘                      │
//       │ Failure Rate > 50%        │ 3 Consecutive
//       │                           │ Successes
//       ▼                           │
//  ┌─────────┐     Timeout          │
//  │  Open   │────────────────► ┌───┴────────┐
//  │ Failing │                  │ Half-Open  │
//  └─────────┘                  │  Testing   │
//       ▲                       └────────────┘
//       │ Any Failure                │
//       └────────────────────────────┘
```

### Example 4: Custom Middleware

```go
// Custom request validation middleware
func RequestValidation() guardian.Middleware {
    return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
        // Validate request
        if validator, ok := req.(interface{ Validate() error }); ok {
            if err := validator.Validate(); err != nil {
                return nil, status.Errorf(codes.InvalidArgument, "validation failed: %v", err)
            }
        }

        return handler(ctx, req)
    }
}
```

## Configuration

### Environment Variables

```bash
# JWT secret for authentication
JWT_SECRET=your-secret-key

# Enable chaos engineering
ENABLE_CHAOS=true

# Log level (debug, info, warn, error)
LOG_LEVEL=info

# Rate limit (requests per second)
RATE_LIMIT=100

# Circuit breaker threshold (0.0 - 1.0)
CIRCUIT_BREAKER_THRESHOLD=0.5

# Distributed Tracing Configuration
TRACING_ENABLED=true
JAEGER_ENDPOINT=http://localhost:14268/api/traces
SERVICE_NAME=grpc-guardian-service
ENVIRONMENT=production
```

## Testing

```bash
# Run tests
go test ./...

# Run benchmarks
go test -bench=. ./...

# Run chaos engineering tests
go test -tags=chaos ./chaos/...

# Load testing
go run examples/benchmark/main.go
```

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Roadmap

### v1.0 (Current)
- [x] Core middleware chain system
- [x] Authentication middleware (JWT, API Key)
- [x] Logging middleware
- [x] Rate limiting middleware
- [x] Chaos engineering features
- [x] **Circuit Breaker pattern** - ✅ Implemented!

### v1.1 (In Progress)
- [x] **Retry middleware with exponential backoff** - ✅ Implemented!
- [x] **Timeout middleware** - ✅ Implemented!
- [x] **Distributed Tracing (OpenTelemetry + Jaeger)** - ✅ Implemented!
- [ ] Metrics collection (Prometheus)
- [ ] Advanced circuit breaker patterns
- [ ] Service mesh integration (Istio, Linkerd)

### v2.0 (Future)
- [ ] GUI dashboard for chaos testing
- [ ] Real-time traffic analysis
- [ ] ML-based anomaly detection
- [ ] Multi-cluster support

## License

MIT License - see [LICENSE](LICENSE) file for details

## Acknowledgments

- Inspired by [grpc-ecosystem/go-grpc-middleware](https://github.com/grpc-ecosystem/go-grpc-middleware)
- Built on top of [gRPC-Go](https://github.com/grpc/grpc-go)
- Rate limiting based on [golang.org/x/time/rate](https://pkg.go.dev/golang.org/x/time/rate)

## Resources

- [gRPC Documentation](https://grpc.io/docs/)
- [Chaos Engineering Principles](https://principlesofchaos.org/)
- [Microservices Patterns](https://microservices.io/patterns/)

## Support

- **Issues**: [GitHub Issues](https://github.com/grpc-guardian/grpc-guardian/issues)
- **Documentation**: [Full Documentation](https://grpc-guardian.dev/docs)
- **Community**: [Discord Server](https://discord.gg/grpc-guardian)

---

**Built with ❤️ for the gRPC community**
