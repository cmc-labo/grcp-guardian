package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// JaegerConfig holds configuration for Jaeger exporter
type JaegerConfig struct {
	ServiceName     string
	ServiceVersion  string
	Environment     string
	AgentEndpoint   string
	CollectorEndpoint string
	SamplingRate    float64
	ExtraAttributes map[string]string
}

// JaegerOption is a functional option for Jaeger configuration
type JaegerOption func(*JaegerConfig)

// WithServiceName sets the service name
func WithServiceName(name string) JaegerOption {
	return func(c *JaegerConfig) {
		c.ServiceName = name
	}
}

// WithServiceVersion sets the service version
func WithServiceVersion(version string) JaegerOption {
	return func(c *JaegerConfig) {
		c.ServiceVersion = version
	}
}

// WithEnvironment sets the deployment environment
func WithEnvironment(env string) JaegerOption {
	return func(c *JaegerConfig) {
		c.Environment = env
	}
}

// WithAgentEndpoint sets the Jaeger agent endpoint (UDP)
func WithAgentEndpoint(endpoint string) JaegerOption {
	return func(c *JaegerConfig) {
		c.AgentEndpoint = endpoint
	}
}

// WithCollectorEndpoint sets the Jaeger collector endpoint (HTTP)
func WithCollectorEndpoint(endpoint string) JaegerOption {
	return func(c *JaegerConfig) {
		c.CollectorEndpoint = endpoint
	}
}

// WithSamplingRate sets the trace sampling rate (0.0 to 1.0)
func WithSamplingRate(rate float64) JaegerOption {
	return func(c *JaegerConfig) {
		c.SamplingRate = rate
	}
}

// WithAttribute adds an extra resource attribute
func WithAttribute(key, value string) JaegerOption {
	return func(c *JaegerConfig) {
		if c.ExtraAttributes == nil {
			c.ExtraAttributes = make(map[string]string)
		}
		c.ExtraAttributes[key] = value
	}
}

// InitJaeger initializes Jaeger tracing with OpenTelemetry
func InitJaeger(opts ...JaegerOption) (*sdktrace.TracerProvider, error) {
	// Default configuration
	config := &JaegerConfig{
		ServiceName:    "grpc-guardian",
		ServiceVersion: "1.0.0",
		Environment:    "development",
		AgentEndpoint:  "localhost:6831",
		SamplingRate:   1.0, // Sample all traces by default
	}

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	// Create Jaeger exporter
	var exporter *jaeger.Exporter
	var err error

	if config.CollectorEndpoint != "" {
		// Use HTTP collector endpoint
		exporter, err = jaeger.New(
			jaeger.WithCollectorEndpoint(
				jaeger.WithEndpoint(config.CollectorEndpoint),
			),
		)
	} else {
		// Use UDP agent endpoint
		exporter, err = jaeger.New(
			jaeger.WithAgentEndpoint(
				jaeger.WithAgentHost(config.AgentEndpoint),
			),
		)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create Jaeger exporter: %w", err)
	}

	// Build resource attributes
	attrs := []attribute.KeyValue{
		semconv.ServiceName(config.ServiceName),
		semconv.ServiceVersion(config.ServiceVersion),
		semconv.DeploymentEnvironment(config.Environment),
	}

	// Add extra attributes
	for key, value := range config.ExtraAttributes {
		attrs = append(attrs, attribute.String(key, value))
	}

	// Create resource
	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(attrs...),
		resource.WithHost(),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create sampler based on sampling rate
	var sampler sdktrace.Sampler
	if config.SamplingRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if config.SamplingRate <= 0.0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(config.SamplingRate)
	}

	// Create tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set global tracer provider
	otel.SetTracerProvider(tp)

	// Set global propagator to W3C Trace Context and Baggage
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return tp, nil
}

// Shutdown gracefully shuts down the tracer provider
func Shutdown(ctx context.Context, tp *sdktrace.TracerProvider) error {
	if tp == nil {
		return nil
	}
	return tp.Shutdown(ctx)
}

// ForceFlush forces the tracer provider to flush all pending spans
func ForceFlush(ctx context.Context, tp *sdktrace.TracerProvider) error {
	if tp == nil {
		return nil
	}
	return tp.ForceFlush(ctx)
}
