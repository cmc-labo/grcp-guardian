# Quick Start Guide - gRPC Guardian

Get started with gRPC Guardian in 5 minutes!

## Installation

```bash
go get github.com/grpc-guardian/grpc-guardian
```

## Basic Server Setup

### Step 1: Import Guardian

```go
import (
    guardian "github.com/grpc-guardian/grpc-guardian"
    "github.com/grpc-guardian/grpc-guardian/middleware"
)
```

### Step 2: Create Middleware Chain

```go
chain := guardian.NewChain(
    middleware.Logging(),                    // Log all requests
    middleware.RateLimit(100, 10),          // 100 req/sec, burst 10
)
```

### Step 3: Create gRPC Server

```go
server := grpc.NewServer(
    grpc.UnaryInterceptor(chain.UnaryInterceptor()),
)
```

### Step 4: Register and Serve

```go
pb.RegisterYourServiceServer(server, &yourService{})

lis, _ := net.Listen("tcp", ":50051")
server.Serve(lis)
```

## Complete Example

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
        middleware.RateLimit(100, 10),
    )

    // Create server
    server := grpc.NewServer(
        grpc.UnaryInterceptor(chain.UnaryInterceptor()),
    )

    // Start server
    lis, _ := net.Listen("tcp", ":50051")
    log.Println("Server running on :50051")
    server.Serve(lis)
}
```

## Common Patterns

### Production Server

```go
chain := guardian.NewChain(
    middleware.Logging(),
    middleware.Auth(middleware.JWTValidator(secret)),
    middleware.RateLimitPerClient(100, 10, middleware.ExtractClientIP),
    middleware.PerformanceLog(1 * time.Second),
)
```

### Testing with Chaos

```go
import "github.com/grpc-guardian/grpc-guardian/chaos"

chain := guardian.NewChain(
    middleware.Logging(),
    chaos.New(
        chaos.WithLatency(100*time.Millisecond, 500*time.Millisecond, 0.2),
        chaos.WithErrors([]codes.Code{codes.Unavailable}, 0.05),
    ),
)
```

## Next Steps

- [Full Documentation](https://grpc-guardian.dev/docs)
- [Examples](./examples/)
- [API Reference](https://pkg.go.dev/github.com/grpc-guardian/grpc-guardian)
