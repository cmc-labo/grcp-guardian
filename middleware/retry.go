package middleware

import (
	"context"
	"math"
	"math/rand"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Retry implements retry logic with exponential backoff for gRPC requests
type Retry struct {
	maxAttempts      int
	initialBackoff   time.Duration
	maxBackoff       time.Duration
	backoffMultiplier float64
	jitter           bool
	retryableErrors  map[codes.Code]bool
	onRetry          func(attempt int, err error, nextBackoff time.Duration)
}

// RetryOption configures a Retry middleware
type RetryOption func(*Retry)

// WithMaxAttempts sets the maximum number of retry attempts
// Default: 3
func WithMaxAttempts(n int) RetryOption {
	return func(r *Retry) {
		if n > 0 {
			r.maxAttempts = n
		}
	}
}

// WithInitialBackoff sets the initial backoff duration
// Default: 100ms
func WithInitialBackoff(d time.Duration) RetryOption {
	return func(r *Retry) {
		if d > 0 {
			r.initialBackoff = d
		}
	}
}

// WithMaxBackoff sets the maximum backoff duration
// Default: 10s
func WithMaxBackoff(d time.Duration) RetryOption {
	return func(r *Retry) {
		if d > 0 {
			r.maxBackoff = d
		}
	}
}

// WithBackoffMultiplier sets the exponential backoff multiplier
// Default: 2.0 (doubles each retry)
func WithBackoffMultiplier(m float64) RetryOption {
	return func(r *Retry) {
		if m > 1.0 {
			r.backoffMultiplier = m
		}
	}
}

// WithJitter enables jitter to prevent thundering herd
// Default: true
func WithJitter(enabled bool) RetryOption {
	return func(r *Retry) {
		r.jitter = enabled
	}
}

// WithRetryableCodes sets which gRPC status codes should trigger a retry
// Default: Unavailable, ResourceExhausted, Aborted, DeadlineExceeded
func WithRetryableCodes(codes ...codes.Code) RetryOption {
	return func(r *Retry) {
		r.retryableErrors = make(map[codes.Code]bool)
		for _, code := range codes {
			r.retryableErrors[code] = true
		}
	}
}

// WithOnRetry sets a callback function called before each retry attempt
func WithOnRetry(callback func(attempt int, err error, nextBackoff time.Duration)) RetryOption {
	return func(r *Retry) {
		r.onRetry = callback
	}
}

// NewRetry creates a new Retry middleware with default configuration
func NewRetry(opts ...RetryOption) *Retry {
	r := &Retry{
		maxAttempts:       3,
		initialBackoff:    100 * time.Millisecond,
		maxBackoff:        10 * time.Second,
		backoffMultiplier: 2.0,
		jitter:            true,
		retryableErrors: map[codes.Code]bool{
			codes.Unavailable:       true,
			codes.ResourceExhausted: true,
			codes.Aborted:           true,
			codes.DeadlineExceeded:  true,
		},
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// UnaryClientInterceptor returns a unary client interceptor with retry logic
func (r *Retry) UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		var lastErr error

		for attempt := 1; attempt <= r.maxAttempts; attempt++ {
			// Check if context is already cancelled
			if ctx.Err() != nil {
				return ctx.Err()
			}

			// Make the call
			err := invoker(ctx, method, req, reply, cc, opts...)

			// Success - no retry needed
			if err == nil {
				return nil
			}

			lastErr = err

			// Check if error is retryable
			if !r.isRetryable(err) {
				return err
			}

			// Don't retry if this was the last attempt
			if attempt >= r.maxAttempts {
				break
			}

			// Calculate backoff duration
			backoff := r.calculateBackoff(attempt)

			// Call retry callback if provided
			if r.onRetry != nil {
				r.onRetry(attempt, err, backoff)
			}

			// Wait for backoff duration or context cancellation
			select {
			case <-time.After(backoff):
				// Continue to next attempt
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		return lastErr
	}
}

// UnaryServerInterceptor returns a unary server interceptor with retry logic
// Note: Server-side retry is less common but can be useful for retrying downstream calls
func (r *Retry) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		var lastErr error
		var resp interface{}

		for attempt := 1; attempt <= r.maxAttempts; attempt++ {
			// Check if context is already cancelled
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			// Call the handler
			resp, lastErr = handler(ctx, req)

			// Success - no retry needed
			if lastErr == nil {
				return resp, nil
			}

			// Check if error is retryable
			if !r.isRetryable(lastErr) {
				return resp, lastErr
			}

			// Don't retry if this was the last attempt
			if attempt >= r.maxAttempts {
				break
			}

			// Calculate backoff duration
			backoff := r.calculateBackoff(attempt)

			// Call retry callback if provided
			if r.onRetry != nil {
				r.onRetry(attempt, lastErr, backoff)
			}

			// Wait for backoff duration or context cancellation
			select {
			case <-time.After(backoff):
				// Continue to next attempt
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		return resp, lastErr
	}
}

// StreamClientInterceptor returns a stream client interceptor with retry logic
func (r *Retry) StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		var lastErr error
		var stream grpc.ClientStream

		for attempt := 1; attempt <= r.maxAttempts; attempt++ {
			// Check if context is already cancelled
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			// Establish stream
			stream, lastErr = streamer(ctx, desc, cc, method, opts...)

			// Success
			if lastErr == nil {
				return stream, nil
			}

			// Check if error is retryable
			if !r.isRetryable(lastErr) {
				return nil, lastErr
			}

			// Don't retry if this was the last attempt
			if attempt >= r.maxAttempts {
				break
			}

			// Calculate backoff duration
			backoff := r.calculateBackoff(attempt)

			// Call retry callback if provided
			if r.onRetry != nil {
				r.onRetry(attempt, lastErr, backoff)
			}

			// Wait for backoff duration or context cancellation
			select {
			case <-time.After(backoff):
				// Continue to next attempt
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		return nil, lastErr
	}
}

// isRetryable checks if an error should trigger a retry
func (r *Retry) isRetryable(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		// Not a gRPC status error - don't retry
		return false
	}

	return r.retryableErrors[st.Code()]
}

// calculateBackoff calculates the backoff duration for the given attempt
// Uses exponential backoff with optional jitter
func (r *Retry) calculateBackoff(attempt int) time.Duration {
	// Calculate exponential backoff: initialBackoff * (multiplier ^ (attempt - 1))
	backoff := float64(r.initialBackoff) * math.Pow(r.backoffMultiplier, float64(attempt-1))

	// Cap at max backoff
	if backoff > float64(r.maxBackoff) {
		backoff = float64(r.maxBackoff)
	}

	// Add jitter if enabled (randomize between 0 and calculated backoff)
	if r.jitter {
		backoff = rand.Float64() * backoff
	}

	return time.Duration(backoff)
}

// RetryStats holds statistics about retry operations
type RetryStats struct {
	TotalRequests    uint64
	TotalRetries     uint64
	SuccessfulRetries uint64
	FailedRetries    uint64
	AverageAttempts  float64
}

// RetryWithStats wraps Retry with statistics tracking
type RetryWithStats struct {
	*Retry
	stats RetryStats
}

// NewRetryWithStats creates a new Retry middleware with statistics tracking
func NewRetryWithStats(opts ...RetryOption) *RetryWithStats {
	return &RetryWithStats{
		Retry: NewRetry(opts...),
	}
}

// GetStats returns the current retry statistics
func (r *RetryWithStats) GetStats() RetryStats {
	return r.stats
}

// ResetStats resets the retry statistics
func (r *RetryWithStats) ResetStats() {
	r.stats = RetryStats{}
}
