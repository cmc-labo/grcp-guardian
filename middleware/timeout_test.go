package middleware

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTimeout_Success(t *testing.T) {
	timeout := TimeoutSimple(1 * time.Second)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		// Fast response
		time.Sleep(100 * time.Millisecond)
		return "success", nil
	}

	resp, err := timeout(
		context.Background(),
		"request",
		&grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"},
		handler,
	)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if resp != "success" {
		t.Errorf("Expected 'success', got %v", resp)
	}
}

func TestTimeout_Exceeded(t *testing.T) {
	timeout := TimeoutSimple(100 * time.Millisecond)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		// Slow response
		time.Sleep(500 * time.Millisecond)
		return "success", nil
	}

	start := time.Now()
	resp, err := timeout(
		context.Background(),
		"request",
		&grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"},
		handler,
	)
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	if resp != nil {
		t.Errorf("Expected nil response, got %v", resp)
	}

	// Check error code
	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("Expected gRPC status error")
	}

	if st.Code() != codes.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded code, got %v", st.Code())
	}

	// Verify timeout occurred around the expected time
	expectedTimeout := 100 * time.Millisecond
	tolerance := 50 * time.Millisecond
	if duration < expectedTimeout || duration > expectedTimeout+tolerance {
		t.Errorf("Expected timeout around %v, got %v", expectedTimeout, duration)
	}
}

func TestTimeout_WithCallback(t *testing.T) {
	callbackCalled := false
	var callbackMethod string
	var callbackDuration time.Duration

	timeout := Timeout(
		WithTimeout(100*time.Millisecond),
		WithTimeoutCallback(func(method string, duration time.Duration) {
			callbackCalled = true
			callbackMethod = method
			callbackDuration = duration
		}),
	)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		time.Sleep(200 * time.Millisecond)
		return "success", nil
	}

	_, err := timeout(
		context.Background(),
		"request",
		&grpc.UnaryServerInfo{FullMethod: "/test.Service/SlowMethod"},
		handler,
	)

	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	if !callbackCalled {
		t.Error("Expected callback to be called")
	}

	if callbackMethod != "/test.Service/SlowMethod" {
		t.Errorf("Expected method '/test.Service/SlowMethod', got %s", callbackMethod)
	}

	if callbackDuration != 100*time.Millisecond {
		t.Errorf("Expected duration 100ms, got %v", callbackDuration)
	}
}

func TestTimeout_PerMethod(t *testing.T) {
	methodTimeouts := map[string]time.Duration{
		"/test.Service/FastMethod": 50 * time.Millisecond,
		"/test.Service/SlowMethod": 500 * time.Millisecond,
	}

	timeout := TimeoutPerMethod(100*time.Millisecond, methodTimeouts)

	// Test fast method with short timeout
	t.Run("FastMethod", func(t *testing.T) {
		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			time.Sleep(30 * time.Millisecond)
			return "success", nil
		}

		resp, err := timeout(
			context.Background(),
			"request",
			&grpc.UnaryServerInfo{FullMethod: "/test.Service/FastMethod"},
			handler,
		)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if resp != "success" {
			t.Errorf("Expected 'success', got %v", resp)
		}
	})

	// Test slow method with longer timeout
	t.Run("SlowMethod", func(t *testing.T) {
		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			time.Sleep(200 * time.Millisecond)
			return "success", nil
		}

		resp, err := timeout(
			context.Background(),
			"request",
			&grpc.UnaryServerInfo{FullMethod: "/test.Service/SlowMethod"},
			handler,
		)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if resp != "success" {
			t.Errorf("Expected 'success', got %v", resp)
		}
	})

	// Test default timeout for unknown method
	t.Run("DefaultMethod", func(t *testing.T) {
		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			time.Sleep(80 * time.Millisecond)
			return "success", nil
		}

		resp, err := timeout(
			context.Background(),
			"request",
			&grpc.UnaryServerInfo{FullMethod: "/test.Service/UnknownMethod"},
			handler,
		)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if resp != "success" {
			t.Errorf("Expected 'success', got %v", resp)
		}
	})
}

func TestTimeout_ContextCancellation(t *testing.T) {
	timeout := TimeoutSimple(1 * time.Second)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
			return "success", nil
		}
	}

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := timeout(
		ctx,
		"request",
		&grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"},
		handler,
	)

	if err == nil {
		t.Error("Expected error due to cancelled context, got nil")
	}
}

func TestTimeout_ZeroDuration(t *testing.T) {
	// Should use default timeout (10 seconds)
	timeout := Timeout()

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		time.Sleep(50 * time.Millisecond)
		return "success", nil
	}

	resp, err := timeout(
		context.Background(),
		"request",
		&grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"},
		handler,
	)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if resp != "success" {
		t.Errorf("Expected 'success', got %v", resp)
	}
}

// Benchmark tests
func BenchmarkTimeout_NoTimeout(b *testing.B) {
	timeout := TimeoutSimple(1 * time.Second)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "success", nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = timeout(
			context.Background(),
			"request",
			&grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"},
			handler,
		)
	}
}

func BenchmarkTimeout_WithSmallDelay(b *testing.B) {
	timeout := TimeoutSimple(100 * time.Millisecond)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		time.Sleep(10 * time.Millisecond)
		return "success", nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = timeout(
			context.Background(),
			"request",
			&grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"},
			handler,
		)
	}
}
