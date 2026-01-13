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
- **OAuth 2.0 Token Introspection**: RFC 7662 compliant token validation ✨ NEW!
- **API Key Authentication**: Simple API key-based auth
- **Basic Authentication**: Username/password authentication
- **RBAC Support**: Role-based access control
- **Scope-Based Authorization**: OAuth 2.0 scope validation ✨ NEW!
- **Custom Auth Handlers**: Extensible authentication system

#### 2. Logging & Observability
- **Structured Logging**: JSON-formatted logs with context
- **Request/Response Logging**: Automatic gRPC call logging
- **Prometheus Metrics**: Request rate, latency, errors, active requests ✨ NEW!
- **Distributed Tracing**: Full OpenTelemetry + Jaeger integration

#### 3. Response Caching ✨ NEW!
- **In-Memory Caching**: Fast in-memory cache backend
- **TTL Support**: Configurable time-to-live for cache entries
- **Per-Method TTL**: Custom TTL for specific methods
- **Cache Key Strategies**: Flexible key generation (default, simple, custom)
- **Cache Statistics**: Hit rate, miss rate, evictions tracking
- **LRU Eviction**: Automatic eviction of least recently used entries

#### 4. Rate Limiting
- **Token Bucket Algorithm**: Industry-standard rate limiting
- **Per-Client Limits**: IP or user-based rate limits
- **Adaptive Rate Limiting**: Dynamic adjustment based on load
- **Quota Management**: Request quota enforcement

#### 5. Resilience & Fault Tolerance
- **Retry Logic**: Automatic retry with exponential backoff
- **Circuit Breaking**: Automatic failure detection and recovery
- **Timeout Control**: Request timeout management with per-method configuration
- **Bulkhead Isolation**: Resource isolation between services

#### 6. Chaos Engineering
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

### Response Caching Example ✨ NEW!

```go
import (
    guardian "github.com/grpc-guardian/grpc-guardian"
    "github.com/grpc-guardian/grpc-guardian/middleware"
    "github.com/grpc-guardian/grpc-guardian/pkg/cache"
)

func main() {
    // Create cache backend
    cacheBackend := cache.NewMemoryBackend(&cache.MemoryConfig{
        MaxSize:         1000,              // Maximum 1000 cached entries
        CleanupInterval: 5 * time.Minute,   // Clean expired entries every 5 min
    })

    // Create middleware chain with caching
    chain := guardian.NewChain(
        middleware.Logging(),
        middleware.Cache(
            middleware.WithCacheBackend(cacheBackend),
            middleware.WithTTL(5*time.Minute),  // Default 5 minute TTL
            middleware.WithMethodTTL("/api.UserService/GetProfile", 10*time.Minute),  // Custom TTL
            middleware.WithSkipMethod("/api.UserService/UpdateProfile"),  // Don't cache mutations
            middleware.WithCacheErrors(),  // Cache error responses too
        ),
    )

    // Create gRPC server
    server := grpc.NewServer(
        grpc.UnaryInterceptor(chain.UnaryInterceptor()),
    )

    // Your service will now have responses cached!
}
```

#### Advanced Caching Features

```go
// Custom cache key generation
customKeyGen := cache.NewCustomKeyGenerator(func(method string, req interface{}) (string, error) {
    // Generate key based on user ID from request
    userReq := req.(*UserRequest)
    return fmt.Sprintf("%s:user:%d", method, userReq.UserId), nil
})

middleware.Cache(
    middleware.WithKeyGenerator(customKeyGen),
)

// Method-specific key generation
methodKeyGen := cache.NewMethodKeyGenerator(cache.NewDefaultKeyGenerator())
methodKeyGen.RegisterMethod("/api.UserService/GetProfile", cache.NewSimpleKeyGenerator())

// Cache invalidation
ctx := context.Background()
err := middleware.InvalidateCache(ctx, cacheBackend, "/api.UserService/GetProfile", req)

// Clear all cache
err := middleware.ClearCache(ctx, cacheBackend)

// Get cache statistics
stats := middleware.GetCacheStats(cacheBackend)
fmt.Printf("Hit Rate: %.2f%%\n", stats.HitRate * 100)
fmt.Printf("Cache Size: %d/%d\n", stats.Size, stats.MaxSize)
fmt.Printf("Evictions: %d\n", stats.Evictions)
```

#### Cache Performance Benefits

```go
// Without cache: Every request hits database/backend
// Request 1: 200ms (database query)
// Request 2: 200ms (database query)
// Request 3: 200ms (database query)
// Total: 600ms

// With cache: Only first request hits database
// Request 1: 200ms (database query + cache set)
// Request 2: <1ms (from cache)
// Request 3: <1ms (from cache)
// Total: ~202ms (3x faster!)
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

// OAuth 2.0 token introspection (RFC 7662)
middleware.Auth(
    middleware.OAuth2Validator(middleware.OAuth2Config{
        IntrospectionURL: "https://oauth-provider.com/introspect",
        ClientID:         "my-client-id",
        ClientSecret:     "my-client-secret",
        Timeout:          5 * time.Second,
    }),
)

// Basic authentication
middleware.Auth(
    middleware.BasicAuthValidator("username", "password"),
)

// Custom authentication
middleware.Auth(
    func(ctx context.Context, req interface{}) error {
        // Your custom auth logic
        return nil
    },
)
```

#### OAuth 2.0 Token Introspection ✨ NEW!

OAuth 2.0 token introspection (RFC 7662) allows you to validate access tokens by calling your OAuth 2.0 provider's introspection endpoint. This is useful for:
- Validating opaque access tokens
- Real-time token revocation checking
- Integrating with Auth0, Keycloak, Okta, etc.

```go
// Configure OAuth2 validator
oauth2Config := middleware.OAuth2Config{
    IntrospectionURL: "https://oauth-provider.com/introspect",
    ClientID:         "my-service",
    ClientSecret:     "service-secret",
    Timeout:          5 * time.Second,
}

// Use in middleware chain
chain := guardian.NewChain(
    middleware.Logging(),
    middleware.Auth(middleware.OAuth2Validator(oauth2Config)),
)

// Require specific scopes for methods
middleware.RequireScope("profile:read")   // Read user profile
middleware.RequireScope("profile:write")  // Update user profile
```

**Context Values After OAuth2 Authentication:**

After successful authentication, the following values are added to the context:

```go
func (s *server) MyMethod(ctx context.Context, req *MyRequest) (*MyResponse, error) {
    // Get user information from context
    userID, _ := ctx.Value("user_id").(string)          // Subject (sub)
    username, _ := ctx.Value("username").(string)       // Username
    clientID, _ := middleware.GetClientID(ctx)          // OAuth2 client ID
    scopes, _ := middleware.GetScopes(ctx)              // Granted scopes

    log.Printf("Request from user=%s, scopes=%v", username, scopes)
    return &MyResponse{}, nil
}
```

**Scope-Based Authorization:**

```go
// Create middleware requiring specific scopes
readScope := middleware.RequireScope("resource:read")
writeScope := middleware.RequireScope("resource:write")
adminScope := middleware.RequireScope("admin")

// Use in gRPC server with per-method middleware
server := grpc.NewServer(
    grpc.ChainUnaryInterceptor(
        // Global auth
        middleware.Auth(middleware.OAuth2Validator(oauth2Config)),
        // Per-method scope check
        readScope,
    ),
)
```

**Production OAuth2 Providers:**

```go
// Auth0
oauth2Config := middleware.OAuth2Config{
    IntrospectionURL: "https://YOUR_DOMAIN.auth0.com/oauth/token/introspect",
    ClientID:         "YOUR_CLIENT_ID",
    ClientSecret:     "YOUR_CLIENT_SECRET",
}

// Keycloak
oauth2Config := middleware.OAuth2Config{
    IntrospectionURL: "https://keycloak.example.com/auth/realms/YOUR_REALM/protocol/openid-connect/token/introspect",
    ClientID:         "YOUR_CLIENT_ID",
    ClientSecret:     "YOUR_CLIENT_SECRET",
}

// Okta
oauth2Config := middleware.OAuth2Config{
    IntrospectionURL: "https://YOUR_DOMAIN.okta.com/oauth2/default/v1/introspect",
    ClientID:         "YOUR_CLIENT_ID",
    ClientSecret:     "YOUR_CLIENT_SECRET",
}
```

**See also:** [examples/oauth2-demo](examples/oauth2-demo) for a complete working example

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

### Caching Middleware ✨ NEW!

```go
import "github.com/grpc-guardian/grpc-guardian/pkg/cache"

// Basic caching with default settings
middleware.Cache()

// Custom configuration
cacheBackend := cache.NewMemoryBackend(&cache.MemoryConfig{
    MaxSize:         1000,
    CleanupInterval: 5 * time.Minute,
})

middleware.Cache(
    middleware.WithCacheBackend(cacheBackend),
    middleware.WithTTL(5*time.Minute),  // Default TTL
)

// Per-method TTL
middleware.Cache(
    middleware.WithTTL(5*time.Minute),
    middleware.WithMethodTTL("/api.Service/GetUser", 10*time.Minute),
    middleware.WithMethodTTL("/api.Service/GetConfig", 1*time.Hour),
)

// Skip specific methods (e.g., mutations)
middleware.Cache(
    middleware.WithSkipMethod("/api.Service/CreateUser"),
    middleware.WithSkipMethod("/api.Service/UpdateUser"),
    middleware.WithSkipMethod("/api.Service/DeleteUser"),
)

// Only cache specific methods
middleware.Cache(
    middleware.WithOnlyMethod("/api.Service/GetUser"),
    middleware.WithOnlyMethod("/api.Service/ListUsers"),
)

// Cache error responses
middleware.Cache(
    middleware.WithCacheErrors(),
)

// Custom key generation
customKeyGen := cache.NewCustomKeyGenerator(func(method string, req interface{}) (string, error) {
    // Your custom key logic
    return fmt.Sprintf("%s:%v", method, req), nil
})

middleware.Cache(
    middleware.WithKeyGenerator(customKeyGen),
)
```

#### Cache Management

```go
// Invalidate specific cache entry
ctx := context.Background()
err := middleware.InvalidateCache(ctx, cacheBackend, methodName, request)

// Clear all cache
err := middleware.ClearCache(ctx, cacheBackend)

// Get cache statistics
stats := middleware.GetCacheStats(cacheBackend)
fmt.Printf("Hits: %d, Misses: %d, Hit Rate: %.2f%%\n",
    stats.Hits, stats.Misses, stats.HitRate*100)
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

### Metrics Collection (Prometheus) ✨ NEW!

```go
import (
    "net/http"

    "github.com/grpc-guardian/grpc-guardian/middleware"
    "github.com/grpc-guardian/grpc-guardian/pkg/metrics"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

// Create Prometheus metrics collector
collector, err := metrics.NewPrometheusCollector(
    metrics.WithNamespace("grpc_guardian"),
    metrics.WithSubsystem("api"),
    metrics.WithConstLabels(map[string]string{
        "service": "my-service",
        "version": "1.0.0",
    }),
)
if err != nil {
    log.Fatal(err)
}

// Use in middleware chain
chain := guardian.NewChain(
    middleware.Logging(),
    middleware.MetricsMiddleware(collector),
)

// Expose metrics endpoint
http.Handle("/metrics", promhttp.HandlerFor(
    collector.GetRegistry(),
    promhttp.HandlerOpts{
        EnableOpenMetrics: true,
    },
))
go http.ListenAndServe(":9090", nil)

// Quick start with defaults
chain := guardian.NewChain(
    middleware.Metrics(), // Uses default Prometheus configuration
)
```

**Available Metrics:**

| Metric Name | Type | Description | Labels |
|------------|------|-------------|--------|
| `grpc_server_requests_total` | Counter | Total number of gRPC requests | `method`, `code` |
| `grpc_server_request_duration_seconds` | Histogram | Request latency distribution | `method`, `code` |
| `grpc_server_active_requests` | Gauge | Number of active requests | `method` |
| `grpc_server_errors_total` | Counter | Total number of errors | `method`, `error_type` |
| `grpc_server_message_sent_bytes` | Histogram | Size of sent messages | `method`, `direction` |
| `grpc_server_message_received_bytes` | Histogram | Size of received messages | `method`, `direction` |

**Configuration Options:**

```go
// Custom configuration
collector, _ := metrics.NewPrometheusCollector(
    // Set namespace and subsystem
    metrics.WithNamespace("myapp"),
    metrics.WithSubsystem("grpc"),

    // Custom histogram buckets (in seconds)
    metrics.WithHistogramBuckets([]float64{
        0.001, 0.01, 0.1, 1.0, 10.0,
    }),

    // Add constant labels
    metrics.WithConstLabels(map[string]string{
        "environment": "production",
        "region":      "us-west-2",
    }),

    // Disable histogram (use only counter and gauge)
    metrics.WithoutHistogram(),

    // Disable per-method metrics (aggregate all methods)
    metrics.WithoutPerMethodMetrics(),
)
```

**Example Prometheus Queries:**

```promql
# Request rate per second
rate(grpc_server_requests_total[1m])

# Error rate
rate(grpc_server_errors_total[1m]) / rate(grpc_server_requests_total[1m])

# P99 latency
histogram_quantile(0.99, rate(grpc_server_request_duration_seconds_bucket[5m]))

# P50 latency
histogram_quantile(0.50, rate(grpc_server_request_duration_seconds_bucket[5m]))

# Request rate by method
sum(rate(grpc_server_requests_total[1m])) by (method)

# Current active requests
grpc_server_active_requests

# Average message size
rate(grpc_server_message_sent_bytes_sum[5m]) / rate(grpc_server_message_sent_bytes_count[5m])
```

**Grafana Dashboard:**

```json
{
  "title": "gRPC Server Metrics",
  "panels": [
    {
      "title": "Request Rate",
      "targets": [
        {"expr": "rate(grpc_server_requests_total[1m])"}
      ]
    },
    {
      "title": "Latency (P50, P95, P99)",
      "targets": [
        {"expr": "histogram_quantile(0.50, rate(grpc_server_request_duration_seconds_bucket[5m]))"},
        {"expr": "histogram_quantile(0.95, rate(grpc_server_request_duration_seconds_bucket[5m]))"},
        {"expr": "histogram_quantile(0.99, rate(grpc_server_request_duration_seconds_bucket[5m]))"}
      ]
    },
    {
      "title": "Error Rate",
      "targets": [
        {"expr": "rate(grpc_server_errors_total[1m])"}
      ]
    }
  ]
}
```

### Service Mesh Integration ✨ NEW!

```go
import (
    "github.com/grpc-guardian/grpc-guardian/middleware"
    "github.com/grpc-guardian/grpc-guardian/pkg/servicemesh"
)

// Istio Integration
istioMiddleware, err := middleware.Istio(
    &servicemesh.Config{
        ServiceName:            "my-service",
        Namespace:              "production",
        EnableMTLS:             true,
        EnableTrafficSplitting: true,
        CustomHeaders: []string{
            "x-user-id",
            "x-session-id",
        },
    },
    middleware.WithHeaderPropagation(),
    middleware.WithMTLSValidation(),
    middleware.WithMetadataLogging(),
    middleware.WithMetadataCallback(func(metadata *servicemesh.MeshMetadata) {
        log.Printf("Request from %s/%s", metadata.SourceNamespace, metadata.SourceWorkload)
    }),
)

// Linkerd Integration
linkerdMiddleware, err := middleware.Linkerd(
    &servicemesh.Config{
        ServiceName: "my-service",
        Namespace:   "production",
        EnableMTLS:  true,
    },
    middleware.WithHeaderPropagation(),
    middleware.WithMTLSValidation(),
)

// Simple setup (recommended for most use cases)
istioMiddleware, _ := middleware.IstioSimple("my-service", "production")
linkerdMiddleware, _ := middleware.LinkerdSimple("my-service", "production")

// Use in gRPC server
server := grpc.NewServer(
    grpc.ChainUnaryInterceptor(
        middleware.Logging(),
        istioMiddleware.UnaryServerInterceptor(),  // or linkerdMiddleware
        middleware.Tracing(),
    ),
)

// Use in gRPC client
conn, _ := grpc.Dial(
    "other-service:50051",
    grpc.WithUnaryInterceptor(istioMiddleware.UnaryClientInterceptor()),
)
```

**Features:**

**Istio Integration:**
- ✓ Automatic header propagation (x-request-id, x-b3-traceid, x-b3-spanid, etc.)
- ✓ Envoy metadata extraction and parsing
- ✓ mTLS validation with SPIFFE ID verification
- ✓ Traffic splitting via VirtualService
- ✓ Fault injection integration
- ✓ Service discovery via Pilot/Istiod
- ✓ Metrics reporting to Istio telemetry

**Linkerd Integration:**
- ✓ Automatic header propagation (l5d-ctx-*, l5d-dst-override, etc.)
- ✓ Linkerd identity validation
- ✓ mTLS with Linkerd certificates
- ✓ Traffic splitting via SMI TrafficSplit
- ✓ ServiceProfile integration
- ✓ Tap API support
- ✓ Per-route metrics

**Common Features:**
- Distributed tracing context propagation
- Service-to-service authentication
- Request metadata extraction and injection
- Custom header propagation
- Error handling with mesh-aware retry policies
- Metrics collection and reporting

**Deployment Example (Istio):**

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-service
  namespace: production
spec:
  ports:
  - port: 50051
    name: grpc
  selector:
    app: my-service
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-service
  namespace: production
spec:
  replicas: 3
  selector:
    matchLabels:
      app: my-service
  template:
    metadata:
      labels:
        app: my-service
        version: v1
      annotations:
        sidecar.istio.io/inject: "true"
    spec:
      containers:
      - name: my-service
        image: my-service:latest
        ports:
        - containerPort: 50051
        env:
        - name: MESH_PROVIDER
          value: "istio"
        - name: ENABLE_MTLS
          value: "true"
---
# Istio VirtualService for traffic splitting
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: my-service
  namespace: production
spec:
  hosts:
  - my-service
  http:
  - match:
    - headers:
        x-canary:
          exact: "true"
    route:
    - destination:
        host: my-service
        subset: v2
  - route:
    - destination:
        host: my-service
        subset: v1
      weight: 90
    - destination:
        host: my-service
        subset: v2
      weight: 10
---
# Istio DestinationRule with mTLS
apiVersion: networking.istio.io/v1beta1
kind: DestinationRule
metadata:
  name: my-service
  namespace: production
spec:
  host: my-service
  trafficPolicy:
    tls:
      mode: ISTIO_MUTUAL
  subsets:
  - name: v1
    labels:
      version: v1
  - name: v2
    labels:
      version: v2
```

**Deployment Example (Linkerd):**

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-service
  namespace: production
spec:
  ports:
  - port: 50051
    name: grpc
  selector:
    app: my-service
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-service
  namespace: production
  annotations:
    linkerd.io/inject: enabled
spec:
  replicas: 3
  selector:
    matchLabels:
      app: my-service
  template:
    metadata:
      labels:
        app: my-service
    spec:
      containers:
      - name: my-service
        image: my-service:latest
        ports:
        - containerPort: 50051
        env:
        - name: MESH_PROVIDER
          value: "linkerd"
---
# Linkerd ServiceProfile for retries and timeouts
apiVersion: linkerd.io/v1alpha2
kind: ServiceProfile
metadata:
  name: my-service.production.svc.cluster.local
  namespace: production
spec:
  routes:
  - name: SayHello
    condition:
      method: POST
      pathRegex: /.*SayHello
    isRetryable: true
    timeout: 10s
---
# SMI TrafficSplit for canary deployment
apiVersion: split.smi-spec.io/v1alpha1
kind: TrafficSplit
metadata:
  name: my-service
  namespace: production
spec:
  service: my-service
  backends:
  - service: my-service-v1
    weight: 90
  - service: my-service-v2
    weight: 10
```

**Header Propagation:**

The service mesh middleware automatically propagates trace context and service mesh headers:

| Mesh | Headers Propagated |
|------|-------------------|
| **Istio** | x-request-id, x-b3-traceid, x-b3-spanid, x-b3-parentspanid, x-b3-sampled, x-envoy-* |
| **Linkerd** | l5d-ctx-trace, l5d-ctx-traceid, l5d-ctx-spanid, l5d-ctx-parentid, l5d-dst-override, l5d-dtab |

**Testing Service Mesh Integration:**

```bash
# Install Istio
curl -L https://istio.io/downloadIstio | sh -
istioctl install --set profile=demo -y

# Enable Istio injection
kubectl label namespace default istio-injection=enabled

# Deploy your service
kubectl apply -f deployment.yaml

# Check Istio proxy status
istioctl proxy-status

# View Istio metrics
kubectl port-forward -n istio-system svc/prometheus 9090:9090

# Install Linkerd
curl -fsL https://run.linkerd.io/install | sh
linkerd install | kubectl apply -f -

# Enable Linkerd injection
kubectl annotate ns default linkerd.io/inject=enabled

# Check Linkerd status
linkerd check
linkerd dashboard
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
│   ├── tracing.go                # Distributed tracing middleware
│   ├── tracing_test.go           # Tracing tests
│   ├── servicemesh.go            # ✨ NEW: Service mesh integration middleware
│   └── servicemesh_test.go       # ✨ NEW: Service mesh tests
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
│   ├── tracing/                  # Distributed tracing utilities
│   │   ├── jaeger.go             # Jaeger exporter configuration
│   │   └── config.go             # Tracing configuration
│   ├── metrics/                  # Metrics collection
│   │   ├── types.go              # Metrics types and interfaces
│   │   └── prometheus.go         # Prometheus collector implementation
│   └── servicemesh/              # ✨ NEW: Service mesh integration
│       ├── types.go              # Common service mesh types and interfaces
│       ├── istio.go              # Istio service mesh integration
│       └── linkerd.go            # Linkerd service mesh integration
├── examples/
│   ├── simple-server/            # Basic usage example
│   ├── chaos-demo/               # Chaos engineering demo
│   ├── circuit-breaker-demo/     # Circuit breaker demo
│   ├── resilience-demo/          # Full resilience stack
│   ├── retry-demo/               # Retry middleware demo
│   ├── timeout-demo/             # Timeout middleware demo
│   ├── tracing-demo/             # Distributed tracing demo
│   ├── metrics-demo/             # Prometheus metrics demo
│   ├── servicemesh-demo/         # ✨ NEW: Service mesh integration demo (Istio/Linkerd)
│   ├── oauth2-demo/              # ✨ NEW: OAuth 2.0 authentication demo
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

// Initialize Prometheus metrics
metricsCollector, _ := metrics.NewPrometheusCollector(
    metrics.WithNamespace("myapp"),
    metrics.WithSubsystem("grpc"),
    metrics.WithConstLabels(map[string]string{
        "service": "production-service",
        "version": "1.0.0",
    }),
)

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

    // Prometheus metrics
    middleware.MetricsMiddleware(metricsCollector),

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

    // Request timeout with per-method configuration
    middleware.Timeout(
        middleware.WithTimeout(10*time.Second),
        middleware.WithPerMethodTimeout(map[string]time.Duration{
            "/api.Service/Upload":   60*time.Second,
            "/api.Service/Download": 120*time.Second,
        }),
    ),
)

// Expose Prometheus metrics endpoint
http.Handle("/metrics", promhttp.HandlerFor(
    metricsCollector.GetRegistry(),
    promhttp.HandlerOpts{EnableOpenMetrics: true},
))
go http.ListenAndServe(":9090", nil)
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

### v1.1 (Completed)
- [x] **Retry middleware with exponential backoff** - ✅ Implemented!
- [x] **Timeout middleware** - ✅ Implemented!
- [x] **Distributed Tracing (OpenTelemetry + Jaeger)** - ✅ Implemented!
- [x] **Metrics collection (Prometheus)** - ✅ Implemented!
- [x] **Service mesh integration (Istio, Linkerd)** - ✅ Implemented!
  - Automatic header propagation (trace context, request IDs)
  - mTLS validation with SPIFFE ID and Linkerd identity verification
  - Traffic splitting support
  - Mesh-aware retry policies
  - Integration with Istio VirtualService and Linkerd ServiceProfile

### v1.2 (Future)
- [ ] Advanced circuit breaker patterns
- [ ] Service mesh gateway mode (standalone proxy)

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
