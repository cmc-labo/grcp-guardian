package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/grpc-guardian/grpc-guardian/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMetricsMiddleware(t *testing.T) {
	collector, err := metrics.NewPrometheusCollector()
	if err != nil {
		t.Fatalf("Failed to create metrics collector: %v", err)
	}

	middleware := MetricsMiddleware(collector)

	// Test successful request
	t.Run("successful request", func(t *testing.T) {
		ctx := context.Background()
		req := "test request"
		info := &grpc.UnaryServerInfo{
			FullMethod: "/test.Service/Method",
		}

		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			time.Sleep(10 * time.Millisecond)
			return "response", nil
		}

		resp, err := middleware(ctx, req, info, handler)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if resp != "response" {
			t.Errorf("Expected response 'response', got: %v", resp)
		}

		// Verify metrics
		registry := collector.GetRegistry()
		metricFamilies, _ := registry.Gather()

		foundRequestsTotal := false
		foundDuration := false

		for _, mf := range metricFamilies {
			switch *mf.Name {
			case "grpc_server_requests_total":
				foundRequestsTotal = true
				if len(mf.Metric) > 0 {
					if *mf.Metric[0].Counter.Value != 1 {
						t.Errorf("Expected requests_total to be 1, got: %f", *mf.Metric[0].Counter.Value)
					}
				}
			case "grpc_server_request_duration_seconds":
				foundDuration = true
			}
		}

		if !foundRequestsTotal {
			t.Error("requests_total metric not found")
		}
		if !foundDuration {
			t.Error("request_duration_seconds metric not found")
		}
	})

	// Test error request
	t.Run("error request", func(t *testing.T) {
		collector, _ := metrics.NewPrometheusCollector()
		middleware := MetricsMiddleware(collector)

		ctx := context.Background()
		req := "test request"
		info := &grpc.UnaryServerInfo{
			FullMethod: "/test.Service/ErrorMethod",
		}

		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			return nil, status.Error(codes.Internal, "internal error")
		}

		_, err := middleware(ctx, req, info, handler)
		if err == nil {
			t.Error("Expected error, got nil")
		}

		// Verify error metrics
		registry := collector.GetRegistry()
		metricFamilies, _ := registry.Gather()

		for _, mf := range metricFamilies {
			if *mf.Name == "grpc_server_errors_total" {
				if len(mf.Metric) > 0 {
					if *mf.Metric[0].Counter.Value != 1 {
						t.Errorf("Expected errors_total to be 1, got: %f", *mf.Metric[0].Counter.Value)
					}
				}
				return
			}
		}

		t.Error("errors_total metric not found")
	})
}

func TestPrometheusCollector(t *testing.T) {
	t.Run("record request", func(t *testing.T) {
		collector, err := metrics.NewPrometheusCollector()
		if err != nil {
			t.Fatalf("Failed to create collector: %v", err)
		}

		collector.RecordRequest("/test.Service/Method", "OK", 100*time.Millisecond)

		registry := collector.GetRegistry()
		metricFamilies, _ := registry.Gather()

		for _, mf := range metricFamilies {
			if *mf.Name == "grpc_server_requests_total" {
				if len(mf.Metric) > 0 {
					if *mf.Metric[0].Counter.Value != 1 {
						t.Errorf("Expected 1 request, got: %f", *mf.Metric[0].Counter.Value)
					}
				}
				return
			}
		}

		t.Error("requests_total metric not found")
	})

	t.Run("record active requests", func(t *testing.T) {
		collector, _ := metrics.NewPrometheusCollector()

		collector.RecordActiveRequests("/test.Service/Method", 1)
		collector.RecordActiveRequests("/test.Service/Method", 1)
		collector.RecordActiveRequests("/test.Service/Method", -1)

		registry := collector.GetRegistry()
		metricFamilies, _ := registry.Gather()

		for _, mf := range metricFamilies {
			if *mf.Name == "grpc_server_active_requests" {
				if len(mf.Metric) > 0 {
					if *mf.Metric[0].Gauge.Value != 1 {
						t.Errorf("Expected 1 active request, got: %f", *mf.Metric[0].Gauge.Value)
					}
				}
				return
			}
		}

		t.Error("active_requests metric not found")
	})

	t.Run("record error", func(t *testing.T) {
		collector, _ := metrics.NewPrometheusCollector()

		collector.RecordError("/test.Service/Method", "Internal")
		collector.RecordError("/test.Service/Method", "Internal")

		registry := collector.GetRegistry()
		metricFamilies, _ := registry.Gather()

		for _, mf := range metricFamilies {
			if *mf.Name == "grpc_server_errors_total" {
				if len(mf.Metric) > 0 {
					if *mf.Metric[0].Counter.Value != 2 {
						t.Errorf("Expected 2 errors, got: %f", *mf.Metric[0].Counter.Value)
					}
				}
				return
			}
		}

		t.Error("errors_total metric not found")
	})
}

func TestCustomConfiguration(t *testing.T) {
	t.Run("custom namespace and subsystem", func(t *testing.T) {
		collector, err := metrics.NewPrometheusCollector(
			metrics.WithNamespace("custom"),
			metrics.WithSubsystem("api"),
		)
		if err != nil {
			t.Fatalf("Failed to create collector: %v", err)
		}

		collector.RecordRequest("/test.Service/Method", "OK", 10*time.Millisecond)

		registry := collector.GetRegistry()
		metricFamilies, _ := registry.Gather()

		found := false
		for _, mf := range metricFamilies {
			if *mf.Name == "custom_api_requests_total" {
				found = true
				break
			}
		}

		if !found {
			t.Error("Custom metric name not found")
		}
	})

	t.Run("custom histogram buckets", func(t *testing.T) {
		customBuckets := []float64{0.01, 0.1, 1.0, 10.0}
		collector, err := metrics.NewPrometheusCollector(
			metrics.WithHistogramBuckets(customBuckets),
		)
		if err != nil {
			t.Fatalf("Failed to create collector: %v", err)
		}

		collector.RecordRequest("/test.Service/Method", "OK", 50*time.Millisecond)

		registry := collector.GetRegistry()
		metricFamilies, _ := registry.Gather()

		for _, mf := range metricFamilies {
			if *mf.Name == "grpc_server_request_duration_seconds" {
				if len(mf.Metric) > 0 {
					histogram := mf.Metric[0].Histogram
					if len(histogram.Bucket) != len(customBuckets)+1 { // +1 for +Inf
						t.Errorf("Expected %d buckets, got: %d", len(customBuckets)+1, len(histogram.Bucket))
					}
				}
				return
			}
		}

		t.Error("request_duration_seconds metric not found")
	})

	t.Run("without histogram", func(t *testing.T) {
		collector, err := metrics.NewPrometheusCollector(
			metrics.WithoutHistogram(),
		)
		if err != nil {
			t.Fatalf("Failed to create collector: %v", err)
		}

		collector.RecordRequest("/test.Service/Method", "OK", 10*time.Millisecond)

		registry := collector.GetRegistry()
		metricFamilies, _ := registry.Gather()

		for _, mf := range metricFamilies {
			if *mf.Name == "grpc_server_request_duration_seconds" {
				t.Error("Histogram should be disabled but was found")
			}
		}
	})
}

func getMetricValue(registry *prometheus.Registry, name string) (float64, error) {
	metricFamilies, err := registry.Gather()
	if err != nil {
		return 0, err
	}

	for _, mf := range metricFamilies {
		if *mf.Name == name {
			if len(mf.Metric) > 0 {
				metric := mf.Metric[0]
				switch *mf.Type {
				case dto.MetricType_COUNTER:
					return *metric.Counter.Value, nil
				case dto.MetricType_GAUGE:
					return *metric.Gauge.Value, nil
				}
			}
		}
	}

	return 0, errors.New("metric not found")
}
