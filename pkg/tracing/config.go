package tracing

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// Config represents the tracing configuration
type Config struct {
	Enabled         bool
	ServiceName     string
	ServiceVersion  string
	Environment     string
	Endpoint        string
	SamplingRate    float64
	MaxExportBatch  int
	MaxQueueSize    int
	ExportTimeout   int // in seconds
}

// DefaultConfig returns the default tracing configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:        true,
		ServiceName:    "grpc-guardian-service",
		ServiceVersion: "1.0.0",
		Environment:    getEnvOrDefault("ENVIRONMENT", "development"),
		Endpoint:       getEnvOrDefault("JAEGER_ENDPOINT", "http://localhost:14268/api/traces"),
		SamplingRate:   1.0,
		MaxExportBatch: 512,
		MaxQueueSize:   2048,
		ExportTimeout:  30,
	}
}

// Setup initializes the tracing system based on the configuration
func Setup(config *Config) (*sdktrace.TracerProvider, error) {
	if !config.Enabled {
		return nil, nil
	}

	// Create Jaeger exporter
	exporter, err := jaeger.New(
		jaeger.WithCollectorEndpoint(
			jaeger.WithEndpoint(config.Endpoint),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Jaeger exporter: %w", err)
	}

	// Create resource
	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
			semconv.DeploymentEnvironment(config.Environment),
		),
		resource.WithHost(),
		resource.WithProcess(),
		resource.WithOS(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create sampler
	sampler := sdktrace.ParentBased(
		sdktrace.TraceIDRatioBased(config.SamplingRate),
	)

	// Create tracer provider with batch span processor
	bsp := sdktrace.NewBatchSpanProcessor(
		exporter,
		sdktrace.WithMaxExportBatchSize(config.MaxExportBatch),
		sdktrace.WithMaxQueueSize(config.MaxQueueSize),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(bsp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set global tracer provider
	otel.SetTracerProvider(tp)

	// Set global propagator
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return tp, nil
}

// getEnvOrDefault returns the value of an environment variable or a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// TracingEnabled returns true if tracing is enabled based on environment
func TracingEnabled() bool {
	return getEnvOrDefault("TRACING_ENABLED", "true") == "true"
}

// GetServiceName returns the service name from environment or default
func GetServiceName() string {
	return getEnvOrDefault("SERVICE_NAME", "grpc-guardian-service")
}

// GetJaegerEndpoint returns the Jaeger endpoint from environment or default
func GetJaegerEndpoint() string {
	return getEnvOrDefault("JAEGER_ENDPOINT", "http://localhost:14268/api/traces")
}

// GetSamplingRate returns the sampling rate from environment or default
func GetSamplingRate() float64 {
	// This is simplified - in production you'd want proper parsing
	return 1.0
}

// QuickSetup provides a quick way to setup tracing with sensible defaults
func QuickSetup(serviceName string) (*sdktrace.TracerProvider, error) {
	config := DefaultConfig()
	config.ServiceName = serviceName

	return Setup(config)
}

// MustSetup is like Setup but panics on error
func MustSetup(config *Config) *sdktrace.TracerProvider {
	tp, err := Setup(config)
	if err != nil {
		panic(fmt.Sprintf("failed to setup tracing: %v", err))
	}
	return tp
}
