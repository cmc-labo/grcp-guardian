package middleware

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TimeoutConfig holds configuration for timeout middleware
type TimeoutConfig struct {
	Timeout       time.Duration
	OnTimeout     func(method string, duration time.Duration)
	PerMethod     map[string]time.Duration
	DefaultMethod time.Duration
}

// TimeoutOption is a functional option for timeout configuration
type TimeoutOption func(*TimeoutConfig)

// WithTimeout sets the default timeout duration
func WithTimeout(timeout time.Duration) TimeoutOption {
	return func(c *TimeoutConfig) {
		c.Timeout = timeout
	}
}

// WithTimeoutCallback sets a callback function when timeout occurs
func WithTimeoutCallback(callback func(method string, duration time.Duration)) TimeoutOption {
	return func(c *TimeoutConfig) {
		c.OnTimeout = callback
	}
}

// WithPerMethodTimeout sets method-specific timeout durations
func WithPerMethodTimeout(methodTimeouts map[string]time.Duration) TimeoutOption {
	return func(c *TimeoutConfig) {
		c.PerMethod = methodTimeouts
	}
}

// Timeout creates a timeout middleware that enforces request deadlines
// Default timeout is 10 seconds if not specified
func Timeout(opts ...TimeoutOption) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// Default configuration
	config := &TimeoutConfig{
		Timeout:   10 * time.Second,
		PerMethod: make(map[string]time.Duration),
	}

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Determine timeout for this method
		timeout := config.Timeout
		if methodTimeout, ok := config.PerMethod[info.FullMethod]; ok {
			timeout = methodTimeout
		}

		// Create context with timeout
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		// Channel to receive handler result
		type result struct {
			resp interface{}
			err  error
		}
		resultChan := make(chan result, 1)

		// Execute handler in goroutine
		go func() {
			resp, err := handler(ctx, req)
			resultChan <- result{resp: resp, err: err}
		}()

		// Wait for either completion or timeout
		select {
		case res := <-resultChan:
			return res.resp, res.err
		case <-ctx.Done():
			// Timeout occurred
			if config.OnTimeout != nil {
				config.OnTimeout(info.FullMethod, timeout)
			}
			return nil, status.Errorf(codes.DeadlineExceeded, "request timeout after %v", timeout)
		}
	}
}

// TimeoutSimple creates a simple timeout middleware with a fixed duration
func TimeoutSimple(timeout time.Duration) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	return Timeout(WithTimeout(timeout))
}

// TimeoutPerMethod creates a timeout middleware with method-specific timeouts
func TimeoutPerMethod(defaultTimeout time.Duration, methodTimeouts map[string]time.Duration) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	return Timeout(
		WithTimeout(defaultTimeout),
		WithPerMethodTimeout(methodTimeouts),
	)
}

// StreamTimeout creates a timeout middleware for streaming RPCs
func StreamTimeout(timeout time.Duration) func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		// Wrap the server stream with the new context
		wrappedStream := &timeoutStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		// Channel to receive handler result
		errChan := make(chan error, 1)

		// Execute handler in goroutine
		go func() {
			errChan <- handler(srv, wrappedStream)
		}()

		// Wait for either completion or timeout
		select {
		case err := <-errChan:
			return err
		case <-ctx.Done():
			return status.Errorf(codes.DeadlineExceeded, "stream timeout after %v", timeout)
		}
	}
}

// timeoutStream wraps grpc.ServerStream with a custom context
type timeoutStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the custom context with timeout
func (s *timeoutStream) Context() context.Context {
	return s.ctx
}
