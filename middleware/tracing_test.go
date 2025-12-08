package middleware

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTracing(t *testing.T) {
	// Create in-memory span recorder for testing
	sr := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(sr))
	otel.SetTracerProvider(tp)

	// Create tracing middleware
	tracingMW := Tracing(
		WithRecordErrors(),
		WithRecordEvents(),
	)

	tests := []struct {
		name          string
		method        string
		handler       grpc.UnaryHandler
		wantErr       bool
		wantSpanCount int
	}{
		{
			name:   "successful request",
			method: "/test.Service/Method",
			handler: func(ctx context.Context, req interface{}) (interface{}, error) {
				return "response", nil
			},
			wantErr:       false,
			wantSpanCount: 1,
		},
		{
			name:   "failed request",
			method: "/test.Service/Method",
			handler: func(ctx context.Context, req interface{}) (interface{}, error) {
				return nil, status.Error(codes.Internal, "test error")
			},
			wantErr:       true,
			wantSpanCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset span recorder
			sr.Reset()

			// Create mock server info
			info := &grpc.UnaryServerInfo{
				FullMethod: tt.method,
			}

			// Execute middleware
			ctx := context.Background()
			_, err := tracingMW(ctx, nil, info, tt.handler)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("Tracing() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Force flush spans
			_ = tp.ForceFlush(context.Background())

			// Check span count
			spans := sr.Ended()
			if len(spans) != tt.wantSpanCount {
				t.Errorf("Expected %d spans, got %d", tt.wantSpanCount, len(spans))
			}

			if len(spans) > 0 {
				span := spans[0]

				// Check span name
				if span.Name() != tt.method {
					t.Errorf("Expected span name %s, got %s", tt.method, span.Name())
				}

				// Check span status
				if tt.wantErr {
					if span.Status().Code != 2 { // Error code
						t.Errorf("Expected error status, got %v", span.Status())
					}
				}
			}
		})
	}
}

func TestExtractServiceName(t *testing.T) {
	tests := []struct {
		fullMethod  string
		wantService string
	}{
		{"/grpc.health.v1.Health/Check", "grpc.health.v1.Health"},
		{"/test.Service/Method", "test.Service"},
		{"/Service/Method", "Service"},
	}

	for _, tt := range tests {
		t.Run(tt.fullMethod, func(t *testing.T) {
			got := extractServiceName(tt.fullMethod)
			if got != tt.wantService {
				t.Errorf("extractServiceName(%s) = %s, want %s", tt.fullMethod, got, tt.wantService)
			}
		})
	}
}

func TestExtractMethodName(t *testing.T) {
	tests := []struct {
		fullMethod string
		wantMethod string
	}{
		{"/grpc.health.v1.Health/Check", "Check"},
		{"/test.Service/Method", "Method"},
		{"/Service/Method", "Method"},
	}

	for _, tt := range tests {
		t.Run(tt.fullMethod, func(t *testing.T) {
			got := extractMethodName(tt.fullMethod)
			if got != tt.wantMethod {
				t.Errorf("extractMethodName(%s) = %s, want %s", tt.fullMethod, got, tt.wantMethod)
			}
		})
	}
}

func TestSpanHelpers(t *testing.T) {
	// Create in-memory span recorder
	sr := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(sr))
	otel.SetTracerProvider(tp)

	ctx, span := StartSpan(context.Background(), "test-span")
	defer span.End()

	// Test SetSpanAttribute
	SetSpanAttribute(ctx, "string.attr", "value")
	SetSpanAttribute(ctx, "int.attr", 42)
	SetSpanAttribute(ctx, "bool.attr", true)

	// Test AddEventToSpan
	AddEventToSpan(ctx, "test-event")

	// Test RecordError
	testErr := errors.New("test error")
	RecordError(ctx, testErr)

	// Force flush
	_ = tp.ForceFlush(context.Background())

	// Verify span was created
	spans := sr.Ended()
	if len(spans) != 1 {
		t.Errorf("Expected 1 span, got %d", len(spans))
	}

	if len(spans) > 0 {
		s := spans[0]
		if s.Name() != "test-span" {
			t.Errorf("Expected span name 'test-span', got '%s'", s.Name())
		}

		// Check error was recorded
		if s.Status().Code != 2 { // Error
			t.Errorf("Expected error status, got %v", s.Status())
		}
	}
}
