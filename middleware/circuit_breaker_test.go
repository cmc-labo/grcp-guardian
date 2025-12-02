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

func TestCircuitBreakerStateMachine(t *testing.T) {
	cb := NewCircuitBreaker(
		WithFailureThreshold(0.5),
		WithTimeout(100*time.Millisecond),
		WithMaxRequests(2),
		WithSuccessThreshold(2),
	)

	// Initially closed
	if cb.State() != StateClosed {
		t.Errorf("Expected initial state to be Closed, got %v", cb.State())
	}

	// Simulate failures to open circuit
	for i := 0; i < 20; i++ {
		gen, err := cb.beforeRequest()
		if err != nil {
			break
		}
		// 80% failure rate
		if i%5 != 0 {
			cb.afterRequest(gen, errors.New("simulated failure"))
		} else {
			cb.afterRequest(gen, nil)
		}
	}

	// Should be open now
	if cb.State() != StateOpen {
		t.Errorf("Expected state to be Open after failures, got %v", cb.State())
	}

	// Requests should be rejected while open
	_, err := cb.beforeRequest()
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("Expected ErrCircuitOpen, got %v", err)
	}

	// Wait for timeout to transition to half-open
	time.Sleep(150 * time.Millisecond)

	// Should now be half-open
	gen, err := cb.beforeRequest()
	if err != nil {
		t.Fatalf("Expected request to be allowed in half-open state, got %v", err)
	}

	if cb.State() != StateHalfOpen {
		t.Errorf("Expected state to be HalfOpen, got %v", cb.State())
	}

	// Send successful requests to close circuit
	cb.afterRequest(gen, nil)
	gen, _ = cb.beforeRequest()
	cb.afterRequest(gen, nil)

	// Should be closed now
	if cb.State() != StateClosed {
		t.Errorf("Expected state to be Closed after successes, got %v", cb.State())
	}
}

func TestCircuitBreakerMiddleware(t *testing.T) {
	cbMiddleware := CircuitBreakerMiddleware(
		WithFailureThreshold(0.5),
		WithTimeout(100*time.Millisecond),
	)

	// Successful handler
	successHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "success", nil
	}

	// Failing handler
	failureHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, status.Error(codes.Unavailable, "service unavailable")
	}

	ctx := context.Background()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	// Execute successful requests
	for i := 0; i < 5; i++ {
		_, err := cbMiddleware(ctx, nil, info, successHandler)
		if err != nil {
			t.Fatalf("Unexpected error on success: %v", err)
		}
	}

	// Execute failing requests to trip circuit
	for i := 0; i < 20; i++ {
		_, err := cbMiddleware(ctx, nil, info, failureHandler)
		// After circuit opens, we expect circuit breaker error
		if err != nil {
			st, ok := status.FromError(err)
			if ok && st.Code() == codes.Unavailable {
				// Check if it's circuit breaker error
				if st.Message() != "circuit breaker: circuit breaker is open" {
					continue // Still in closed state, continue failing
				}
			}
		}
	}
}

func TestCircuitBreakerCounts(t *testing.T) {
	cb := NewCircuitBreaker(
		WithFailureThreshold(0.6),
	)

	// Send mix of successful and failed requests
	for i := 0; i < 10; i++ {
		gen, err := cb.beforeRequest()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if i%3 == 0 {
			cb.afterRequest(gen, errors.New("failure"))
		} else {
			cb.afterRequest(gen, nil)
		}
	}

	counts := cb.GetCounts()

	if counts.Requests != 10 {
		t.Errorf("Expected 10 requests, got %d", counts.Requests)
	}

	expectedFailures := uint32(4) // 0, 3, 6, 9
	if counts.TotalFailures != expectedFailures {
		t.Errorf("Expected %d failures, got %d", expectedFailures, counts.TotalFailures)
	}

	expectedSuccesses := uint32(6)
	if counts.TotalSuccesses != expectedSuccesses {
		t.Errorf("Expected %d successes, got %d", expectedSuccesses, counts.TotalSuccesses)
	}
}

func TestCircuitBreakerStateCallback(t *testing.T) {
	stateChanges := []struct {
		from State
		to   State
	}{}

	cb := NewCircuitBreaker(
		WithFailureThreshold(0.5),
		WithTimeout(50*time.Millisecond),
		WithOnStateChange(func(from, to State) {
			stateChanges = append(stateChanges, struct {
				from State
				to   State
			}{from, to})
		}),
	)

	// Force state transitions
	cb.mu.Lock()
	cb.setState(StateOpen, time.Now())
	cb.mu.Unlock()

	time.Sleep(60 * time.Millisecond)

	cb.mu.Lock()
	cb.currentState(time.Now())
	cb.mu.Unlock()

	// Should have at least one state change (Closed -> Open)
	if len(stateChanges) < 1 {
		t.Errorf("Expected at least 1 state change, got %d", len(stateChanges))
	}

	if stateChanges[0].from != StateClosed || stateChanges[0].to != StateOpen {
		t.Errorf("Expected Closed -> Open, got %v -> %v", stateChanges[0].from, stateChanges[0].to)
	}
}

func TestCircuitBreakerIsFailure(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		isFailure bool
	}{
		{"nil error", nil, false},
		{"regular error", errors.New("regular"), true},
		{"unavailable", status.Error(codes.Unavailable, "unavailable"), true},
		{"internal", status.Error(codes.Internal, "internal"), true},
		{"deadline exceeded", status.Error(codes.DeadlineExceeded, "timeout"), true},
		{"invalid argument", status.Error(codes.InvalidArgument, "invalid"), false},
		{"not found", status.Error(codes.NotFound, "not found"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := defaultIsFailure(tt.err)
			if result != tt.isFailure {
				t.Errorf("Expected isFailure(%v) = %v, got %v", tt.err, tt.isFailure, result)
			}
		})
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	cb := NewCircuitBreaker(
		WithFailureThreshold(0.5),
	)

	// Force open state
	cb.mu.Lock()
	cb.setState(StateOpen, time.Now())
	cb.mu.Unlock()

	if cb.State() != StateOpen {
		t.Fatalf("Expected Open state, got %v", cb.State())
	}

	// Reset should bring it back to closed
	cb.Reset()

	if cb.State() != StateClosed {
		t.Errorf("Expected Closed state after reset, got %v", cb.State())
	}

	// Counts should be reset
	counts := cb.GetCounts()
	if counts.Requests != 0 || counts.TotalFailures != 0 || counts.TotalSuccesses != 0 {
		t.Errorf("Expected counts to be reset, got %+v", counts)
	}
}

func BenchmarkCircuitBreakerClosed(b *testing.B) {
	cb := NewCircuitBreaker()
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}

	middleware := CircuitBreakerMiddleware()
	ctx := context.Background()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		middleware(ctx, nil, info, handler)
	}
}

func BenchmarkCircuitBreakerOpen(b *testing.B) {
	cb := NewCircuitBreaker(
		WithFailureThreshold(0.1),
	)

	// Trip the circuit
	for i := 0; i < 20; i++ {
		gen, _ := cb.beforeRequest()
		cb.afterRequest(gen, errors.New("failure"))
	}

	middleware := CircuitBreakerMiddleware(
		WithFailureThreshold(0.1),
	)
	ctx := context.Background()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		middleware(ctx, nil, info, func(ctx context.Context, req interface{}) (interface{}, error) {
			return nil, errors.New("should not be called")
		})
	}
}
