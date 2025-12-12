// Package metrics provides monitoring and metrics collection capabilities
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsCollector defines the interface for metrics collection
type MetricsCollector interface {
	// RecordRequest records a completed request with duration and status
	RecordRequest(method string, code string, duration time.Duration)

	// RecordError records an error occurrence
	RecordError(method string, errorType string)

	// RecordActiveRequests updates the active requests gauge
	RecordActiveRequests(method string, delta int)

	// RecordMessageSize records request/response message sizes
	RecordMessageSize(method string, direction string, size int)

	// GetRegistry returns the prometheus registry
	GetRegistry() *prometheus.Registry
}

// Config holds configuration for metrics collection
type Config struct {
	// Namespace for metrics (e.g., "grpc_guardian")
	Namespace string

	// Subsystem for metrics (e.g., "server")
	Subsystem string

	// Enable histogram buckets for latency distribution
	EnableHistogram bool

	// Custom histogram buckets (in seconds)
	HistogramBuckets []float64

	// Enable per-method metrics
	EnablePerMethodMetrics bool

	// Constant labels to add to all metrics
	ConstLabels map[string]string
}

// DefaultConfig returns the default metrics configuration
func DefaultConfig() *Config {
	return &Config{
		Namespace:              "grpc",
		Subsystem:              "server",
		EnableHistogram:        true,
		EnablePerMethodMetrics: true,
		HistogramBuckets: []float64{
			0.001, // 1ms
			0.005, // 5ms
			0.01,  // 10ms
			0.025, // 25ms
			0.05,  // 50ms
			0.1,   // 100ms
			0.25,  // 250ms
			0.5,   // 500ms
			1.0,   // 1s
			2.5,   // 2.5s
			5.0,   // 5s
			10.0,  // 10s
		},
		ConstLabels: make(map[string]string),
	}
}

// ConfigOption is a function that configures a Config
type ConfigOption func(*Config)

// WithNamespace sets the namespace for metrics
func WithNamespace(namespace string) ConfigOption {
	return func(c *Config) {
		c.Namespace = namespace
	}
}

// WithSubsystem sets the subsystem for metrics
func WithSubsystem(subsystem string) ConfigOption {
	return func(c *Config) {
		c.Subsystem = subsystem
	}
}

// WithHistogramBuckets sets custom histogram buckets
func WithHistogramBuckets(buckets []float64) ConfigOption {
	return func(c *Config) {
		c.HistogramBuckets = buckets
	}
}

// WithConstLabels sets constant labels for all metrics
func WithConstLabels(labels map[string]string) ConfigOption {
	return func(c *Config) {
		c.ConstLabels = labels
	}
}

// WithoutHistogram disables histogram metrics
func WithoutHistogram() ConfigOption {
	return func(c *Config) {
		c.EnableHistogram = false
	}
}

// WithoutPerMethodMetrics disables per-method metrics
func WithoutPerMethodMetrics() ConfigOption {
	return func(c *Config) {
		c.EnablePerMethodMetrics = false
	}
}
