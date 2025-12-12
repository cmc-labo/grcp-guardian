package middleware

import (
	"context"
	"time"

	guardian "github.com/grpc-guardian/grpc-guardian"
	"github.com/grpc-guardian/grpc-guardian/pkg/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MetricsMiddleware creates a middleware that collects metrics
func MetricsMiddleware(collector metrics.MetricsCollector) guardian.Middleware {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		method := info.FullMethod
		start := time.Now()

		// Increment active requests
		collector.RecordActiveRequests(method, 1)
		defer collector.RecordActiveRequests(method, -1)

		// Call the handler
		resp, err := handler(ctx, req)

		// Record duration and status
		duration := time.Since(start)
		code := codes.OK
		if err != nil {
			st, ok := status.FromError(err)
			if ok {
				code = st.Code()
			} else {
				code = codes.Unknown
			}
			collector.RecordError(method, code.String())
		}

		collector.RecordRequest(method, code.String(), duration)

		return resp, err
	}
}

// Metrics creates a metrics middleware with a new Prometheus collector
func Metrics(opts ...metrics.ConfigOption) guardian.Middleware {
	collector, err := metrics.NewPrometheusCollector(opts...)
	if err != nil {
		panic(err) // Should not happen with valid options
	}

	return MetricsMiddleware(collector)
}

// StreamMetricsMiddleware creates a streaming middleware that collects metrics
func StreamMetricsMiddleware(collector metrics.MetricsCollector) guardian.StreamMiddleware {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		method := info.FullMethod
		start := time.Now()

		// Increment active requests
		collector.RecordActiveRequests(method, 1)
		defer collector.RecordActiveRequests(method, -1)

		// Call the handler
		err := handler(srv, ss)

		// Record duration and status
		duration := time.Since(start)
		code := codes.OK
		if err != nil {
			st, ok := status.FromError(err)
			if ok {
				code = st.Code()
			} else {
				code = codes.Unknown
			}
			collector.RecordError(method, code.String())
		}

		collector.RecordRequest(method, code.String(), duration)

		return err
	}
}

// StreamMetrics creates a streaming metrics middleware with a new Prometheus collector
func StreamMetrics(opts ...metrics.ConfigOption) guardian.StreamMiddleware {
	collector, err := metrics.NewPrometheusCollector(opts...)
	if err != nil {
		panic(err)
	}

	return StreamMetricsMiddleware(collector)
}
