package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRetry_Success(t *testing.T) {
	retry := NewRetry(
		WithMaxAttempts(3),
		WithInitialBackoff(10*time.Millisecond),
	)

	attempts := 0
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		return "success", nil
	}

	resp, err := retry.UnaryServerInterceptor()(
		context.Background(),
		"request",
		&grpc.UnaryServerInfo{},
		handler,
	)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if resp != "success" {
		t.Errorf("Expected 'success', got %v", resp)
	}

	if attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", attempts)
	}
}

func TestRetry_RetryableError(t *testing.T) {
	retry := NewRetry(
		WithMaxAttempts(3),
		WithInitialBackoff(10*time.Millisecond),
		WithJitter(false),
	)

	attempts := 0
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		if attempts < 3 {
			return nil, status.Error(codes.Unavailable, "service unavailable")
		}
		return "success", nil
	}

	start := time.Now()
	resp, err := retry.UnaryServerInterceptor()(
		context.Background(),
		"request",
		&grpc.UnaryServerInfo{},
		handler,
	)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if resp != "success" {
		t.Errorf("Expected 'success', got %v", resp)
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}

	// Check that backoff occurred (should be at least initial + second backoff)
	// First retry: 100ms, second retry: 200ms = 300ms total minimum
	expectedMinDuration := 10*time.Millisecond + 20*time.Millisecond
	if duration < expectedMinDuration {
		t.Errorf("Expected duration >= %v, got %v", expectedMinDuration, duration)
	}
}

func TestRetry_NonRetryableError(t *testing.T) {
	retry := NewRetry(
		WithMaxAttempts(3),
		WithInitialBackoff(10*time.Millisecond),
	)

	attempts := 0
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		return nil, status.Error(codes.InvalidArgument, "invalid argument")
	}

	_, err := retry.UnaryServerInterceptor()(
		context.Background(),
		"request",
		&grpc.UnaryServerInfo{},
		handler,
	)

	if err == nil {
		t.Error("Expected error, got nil")
	}

	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("Expected InvalidArgument error, got %v", err)
	}

	if attempts != 1 {
		t.Errorf("Expected 1 attempt (no retries), got %d", attempts)
	}
}

func TestRetry_MaxAttemptsExceeded(t *testing.T) {
	retry := NewRetry(
		WithMaxAttempts(3),
		WithInitialBackoff(10*time.Millisecond),
	)

	attempts := 0
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		return nil, status.Error(codes.Unavailable, "always fails")
	}

	_, err := retry.UnaryServerInterceptor()(
		context.Background(),
		"request",
		&grpc.UnaryServerInfo{},
		handler,
	)

	if err == nil {
		t.Error("Expected error, got nil")
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestRetry_ContextCancellation(t *testing.T) {
	retry := NewRetry(
		WithMaxAttempts(5),
		WithInitialBackoff(100*time.Millisecond),
	)

	attempts := 0
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		return nil, status.Error(codes.Unavailable, "unavailable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	_, err := retry.UnaryServerInterceptor()(
		ctx,
		"request",
		&grpc.UnaryServerInfo{},
		handler,
	)

	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded, got %v", err)
	}

	// Should have attempted at least once, but not all 5 attempts
	if attempts < 1 || attempts >= 5 {
		t.Errorf("Expected 1-4 attempts due to timeout, got %d", attempts)
	}
}

func TestRetry_CustomRetryableCodes(t *testing.T) {
	retry := NewRetry(
		WithMaxAttempts(3),
		WithInitialBackoff(10*time.Millisecond),
		WithRetryableCodes(codes.Internal), // Only Internal is retryable
	)

	// Test with retryable code
	attempts := 0
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		if attempts < 2 {
			return nil, status.Error(codes.Internal, "internal error")
		}
		return "success", nil
	}

	resp, err := retry.UnaryServerInterceptor()(
		context.Background(),
		"request",
		&grpc.UnaryServerInfo{},
		handler,
	)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}

	// Test with non-retryable code (Unavailable)
	attempts = 0
	handler = func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		return nil, status.Error(codes.Unavailable, "unavailable")
	}

	_, err = retry.UnaryServerInterceptor()(
		context.Background(),
		"request",
		&grpc.UnaryServerInfo{},
		handler,
	)

	if err == nil {
		t.Error("Expected error, got nil")
	}

	if attempts != 1 {
		t.Errorf("Expected 1 attempt (no retries), got %d", attempts)
	}
}

func TestRetry_OnRetryCallback(t *testing.T) {
	callbackInvocations := 0
	var capturedAttempts []int
	var capturedErrors []error

	retry := NewRetry(
		WithMaxAttempts(3),
		WithInitialBackoff(10*time.Millisecond),
		WithOnRetry(func(attempt int, err error, nextBackoff time.Duration) {
			callbackInvocations++
			capturedAttempts = append(capturedAttempts, attempt)
			capturedErrors = append(capturedErrors, err)
		}),
	)

	attempts := 0
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		if attempts < 3 {
			return nil, status.Error(codes.Unavailable, "unavailable")
		}
		return "success", nil
	}

	retry.UnaryServerInterceptor()(
		context.Background(),
		"request",
		&grpc.UnaryServerInfo{},
		handler,
	)

	if callbackInvocations != 2 {
		t.Errorf("Expected 2 callback invocations, got %d", callbackInvocations)
	}

	if len(capturedAttempts) != 2 {
		t.Errorf("Expected 2 captured attempts, got %d", len(capturedAttempts))
	}

	if capturedAttempts[0] != 1 || capturedAttempts[1] != 2 {
		t.Errorf("Expected attempts [1, 2], got %v", capturedAttempts)
	}
}

func TestRetry_ExponentialBackoff(t *testing.T) {
	retry := NewRetry(
		WithMaxAttempts(1),
		WithInitialBackoff(100*time.Millisecond),
		WithBackoffMultiplier(2.0),
		WithJitter(false),
	)

	testCases := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 800 * time.Millisecond},
	}

	for _, tc := range testCases {
		backoff := retry.calculateBackoff(tc.attempt)
		if backoff != tc.expected {
			t.Errorf("Attempt %d: expected backoff %v, got %v", tc.attempt, tc.expected, backoff)
		}
	}
}

func TestRetry_MaxBackoffCap(t *testing.T) {
	retry := NewRetry(
		WithMaxAttempts(1),
		WithInitialBackoff(1*time.Second),
		WithMaxBackoff(5*time.Second),
		WithBackoffMultiplier(2.0),
		WithJitter(false),
	)

	// Attempt 4 would be 8 seconds, but should be capped at 5 seconds
	backoff := retry.calculateBackoff(4)
	if backoff != 5*time.Second {
		t.Errorf("Expected backoff to be capped at 5s, got %v", backoff)
	}
}

func TestRetry_Jitter(t *testing.T) {
	retry := NewRetry(
		WithMaxAttempts(1),
		WithInitialBackoff(100*time.Millisecond),
		WithJitter(true),
	)

	// Run multiple times to check jitter variation
	backoffs := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		backoffs[i] = retry.calculateBackoff(1)
	}

	// Check that we got some variation (at least one different value)
	allSame := true
	for i := 1; i < len(backoffs); i++ {
		if backoffs[i] != backoffs[0] {
			allSame = false
			break
		}
	}

	if allSame {
		t.Error("Expected jitter to produce different backoff values")
	}

	// All values should be between 0 and initialBackoff
	for i, backoff := range backoffs {
		if backoff < 0 || backoff > 100*time.Millisecond {
			t.Errorf("Backoff %d out of range: %v", i, backoff)
		}
	}
}

func BenchmarkRetry_NoRetryNeeded(b *testing.B) {
	retry := NewRetry()

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "success", nil
	}

	interceptor := retry.UnaryServerInterceptor()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		interceptor(context.Background(), "request", &grpc.UnaryServerInfo{}, handler)
	}
}

func BenchmarkRetry_WithRetries(b *testing.B) {
	retry := NewRetry(
		WithMaxAttempts(3),
		WithInitialBackoff(1*time.Microsecond), // Very short for benchmark
	)

	attempts := 0
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		if attempts%3 != 0 {
			return nil, status.Error(codes.Unavailable, "unavailable")
		}
		return "success", nil
	}

	interceptor := retry.UnaryServerInterceptor()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		attempts = 0
		interceptor(context.Background(), "request", &grpc.UnaryServerInfo{}, handler)
	}
}
