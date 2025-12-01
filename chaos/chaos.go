// Package chaos provides chaos engineering capabilities for gRPC services
package chaos

import (
	"context"
	"math/rand"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ChaosConfig holds configuration for chaos engineering
type ChaosConfig struct {
	// Latency injection
	LatencyEnabled     bool
	LatencyMin         time.Duration
	LatencyMax         time.Duration
	LatencyProbability float64

	// Error injection
	ErrorEnabled     bool
	ErrorCodes       []codes.Code
	ErrorProbability float64

	// Timeout simulation
	TimeoutEnabled     bool
	TimeoutDuration    time.Duration
	TimeoutProbability float64

	// Circuit breaker simulation
	CircuitBreakerEnabled bool
	CircuitBreakerWindow  time.Duration

	// Conditional enabling
	EnableCondition func() bool
}

// ChaosOption is a functional option for chaos configuration
type ChaosOption func(*ChaosConfig)

// WithLatency enables latency injection
func WithLatency(min, max time.Duration, probability float64) ChaosOption {
	return func(c *ChaosConfig) {
		c.LatencyEnabled = true
		c.LatencyMin = min
		c.LatencyMax = max
		c.LatencyProbability = probability
	}
}

// WithErrors enables error injection
func WithErrors(errorCodes []codes.Code, probability float64) ChaosOption {
	return func(c *ChaosConfig) {
		c.ErrorEnabled = true
		c.ErrorCodes = errorCodes
		c.ErrorProbability = probability
	}
}

// WithTimeout enables timeout simulation
func WithTimeout(duration time.Duration, probability float64) ChaosOption {
	return func(c *ChaosConfig) {
		c.TimeoutEnabled = true
		c.TimeoutDuration = duration
		c.TimeoutProbability = probability
	}
}

// WithCondition sets a condition for enabling chaos
func WithCondition(condition func() bool) ChaosOption {
	return func(c *ChaosConfig) {
		c.EnableCondition = condition
	}
}

// New creates a new chaos engineering middleware
func New(opts ...ChaosOption) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	config := &ChaosConfig{
		EnableCondition: func() bool { return true },
	}

	for _, opt := range opts {
		opt(config)
	}

	// Initialize random seed
	rand.Seed(time.Now().UnixNano())

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Check if chaos is enabled
		if !config.EnableCondition() {
			return handler(ctx, req)
		}

		// Latency injection
		if config.LatencyEnabled && shouldInject(config.LatencyProbability) {
			delay := randomDuration(config.LatencyMin, config.LatencyMax)

			select {
			case <-time.After(delay):
				// Continue after delay
			case <-ctx.Done():
				return nil, status.Errorf(codes.Canceled, "request canceled during chaos latency injection")
			}
		}

		// Error injection
		if config.ErrorEnabled && shouldInject(config.ErrorProbability) {
			code := config.ErrorCodes[rand.Intn(len(config.ErrorCodes))]
			return nil, status.Errorf(code, "chaos engineering: injected error")
		}

		// Timeout simulation
		if config.TimeoutEnabled && shouldInject(config.TimeoutProbability) {
			newCtx, cancel := context.WithTimeout(ctx, config.TimeoutDuration)
			defer cancel()
			return handler(newCtx, req)
		}

		return handler(ctx, req)
	}
}

// LatencyInjector creates latency injection middleware
func LatencyInjector(min, max time.Duration, probability float64) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	rand.Seed(time.Now().UnixNano())

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if shouldInject(probability) {
			delay := randomDuration(min, max)

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, status.Errorf(codes.Canceled, "request canceled during latency injection")
			}
		}

		return handler(ctx, req)
	}
}

// ErrorInjector creates error injection middleware
func ErrorInjector(errorCodes []codes.Code, probability float64) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	rand.Seed(time.Now().UnixNano())

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if shouldInject(probability) {
			code := errorCodes[rand.Intn(len(errorCodes))]
			return nil, status.Errorf(code, "chaos: injected error with code %s", code.String())
		}

		return handler(ctx, req)
	}
}

// RandomErrorInjector injects random errors from all gRPC error codes
func RandomErrorInjector(probability float64) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	errorCodes := []codes.Code{
		codes.Canceled,
		codes.Unknown,
		codes.InvalidArgument,
		codes.DeadlineExceeded,
		codes.NotFound,
		codes.AlreadyExists,
		codes.PermissionDenied,
		codes.ResourceExhausted,
		codes.FailedPrecondition,
		codes.Aborted,
		codes.OutOfRange,
		codes.Unimplemented,
		codes.Internal,
		codes.Unavailable,
		codes.DataLoss,
		codes.Unauthenticated,
	}

	return ErrorInjector(errorCodes, probability)
}

// TimeoutInjector creates timeout injection middleware
func TimeoutInjector(timeout time.Duration, probability float64) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	rand.Seed(time.Now().UnixNano())

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if shouldInject(probability) {
			newCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return handler(newCtx, req)
		}

		return handler(ctx, req)
	}
}

// MethodTargetedChaos applies chaos only to specific methods
type MethodTargetedChaos struct {
	targetMethods map[string]bool
	chaosFunc     func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error)
}

// NewMethodTargetedChaos creates chaos that only affects specific methods
func NewMethodTargetedChaos(methods []string, chaosFunc func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error)) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	targetMethods := make(map[string]bool)
	for _, method := range methods {
		targetMethods[method] = true
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if targetMethods[info.FullMethod] {
			return chaosFunc(ctx, req, info, handler)
		}
		return handler(ctx, req)
	}
}

// PercentageBasedChaos applies chaos to a percentage of requests
type PercentageBasedChaos struct {
	percentage float64
	chaosFunc  func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error)
}

// NewPercentageBasedChaos creates chaos that affects a percentage of requests
func NewPercentageBasedChaos(percentage float64, chaosFunc func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error)) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	rand.Seed(time.Now().UnixNano())

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if rand.Float64() < percentage {
			return chaosFunc(ctx, req, info, handler)
		}
		return handler(ctx, req)
	}
}

// shouldInject determines if chaos should be injected based on probability
func shouldInject(probability float64) bool {
	return rand.Float64() < probability
}

// randomDuration returns a random duration between min and max
func randomDuration(min, max time.Duration) time.Duration {
	if min >= max {
		return min
	}
	return min + time.Duration(rand.Int63n(int64(max-min)))
}

// Presets for common chaos scenarios

// HighLatencyChaos simulates high latency network
func HighLatencyChaos(probability float64) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	return LatencyInjector(500*time.Millisecond, 2*time.Second, probability)
}

// FlakyChaos simulates flaky network with random errors
func FlakyChaos(probability float64) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	return New(
		WithLatency(50*time.Millisecond, 500*time.Millisecond, probability),
		WithErrors([]codes.Code{codes.Unavailable, codes.DeadlineExceeded}, probability/2),
	)
}

// PartitionChaos simulates network partition
func PartitionChaos(probability float64) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	return ErrorInjector([]codes.Code{codes.Unavailable, codes.DeadlineExceeded}, probability)
}

// OverloadedChaos simulates overloaded service
func OverloadedChaos(probability float64) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	return New(
		WithLatency(1*time.Second, 5*time.Second, probability),
		WithErrors([]codes.Code{codes.ResourceExhausted, codes.Unavailable}, probability/2),
	)
}
