package middleware

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TracingConfig holds configuration for tracing middleware
type TracingConfig struct {
	Tracer       trace.Tracer
	TracerName   string
	Propagator   propagation.TextMapPropagator
	RecordErrors bool
	RecordEvents bool
	ExtraAttrs   []attribute.KeyValue
}

// TracingOption is a functional option for tracing configuration
type TracingOption func(*TracingConfig)

// WithTracer sets a custom tracer
func WithTracer(tracer trace.Tracer) TracingOption {
	return func(c *TracingConfig) {
		c.Tracer = tracer
	}
}

// WithTracerName sets the tracer name
func WithTracerName(name string) TracingOption {
	return func(c *TracingConfig) {
		c.TracerName = name
	}
}

// WithPropagator sets a custom propagator
func WithPropagator(propagator propagation.TextMapPropagator) TracingOption {
	return func(c *TracingConfig) {
		c.Propagator = propagator
	}
}

// WithRecordErrors enables error recording in spans
func WithRecordErrors() TracingOption {
	return func(c *TracingConfig) {
		c.RecordErrors = true
	}
}

// WithRecordEvents enables event recording in spans
func WithRecordEvents() TracingOption {
	return func(c *TracingConfig) {
		c.RecordEvents = true
	}
}

// WithExtraAttributes adds extra attributes to all spans
func WithExtraAttributes(attrs ...attribute.KeyValue) TracingOption {
	return func(c *TracingConfig) {
		c.ExtraAttrs = append(c.ExtraAttrs, attrs...)
	}
}

// Tracing creates a distributed tracing middleware with OpenTelemetry
func Tracing(opts ...TracingOption) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// Default configuration
	config := &TracingConfig{
		TracerName:   "grpc-guardian",
		Propagator:   otel.GetTextMapPropagator(),
		RecordErrors: true,
		RecordEvents: true,
	}

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	// Get or create tracer
	if config.Tracer == nil {
		config.Tracer = otel.Tracer(config.TracerName)
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Extract trace context from incoming metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			ctx = config.Propagator.Extract(ctx, &metadataCarrier{md: md})
		}

		// Start a new span
		ctx, span := config.Tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(config.ExtraAttrs...),
		)
		defer span.End()

		// Add RPC attributes
		span.SetAttributes(
			attribute.String("rpc.system", "grpc"),
			attribute.String("rpc.service", extractServiceName(info.FullMethod)),
			attribute.String("rpc.method", extractMethodName(info.FullMethod)),
		)

		// Add user context if available
		if userID, ok := GetUserID(ctx); ok {
			span.SetAttributes(attribute.String("user.id", userID))
		}

		// Record request event if enabled
		if config.RecordEvents {
			span.AddEvent("grpc.request.received")
		}

		// Call handler
		resp, err := handler(ctx, req)

		// Record response event if enabled
		if config.RecordEvents {
			span.AddEvent("grpc.response.sent")
		}

		// Handle errors
		if err != nil {
			st := status.Convert(err)

			// Set span status
			span.SetStatus(codes.Error, st.Message())

			// Add error attributes
			span.SetAttributes(
				attribute.String("rpc.grpc.status_code", st.Code().String()),
				attribute.String("error.message", st.Message()),
			)

			// Record error if enabled
			if config.RecordErrors {
				span.RecordError(err)
			}
		} else {
			// Set successful status
			span.SetStatus(codes.Ok, "")
			span.SetAttributes(
				attribute.String("rpc.grpc.status_code", "OK"),
			)
		}

		return resp, err
	}
}

// TracingWithServiceName creates a tracing middleware with a service name
func TracingWithServiceName(serviceName string, opts ...TracingOption) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	opts = append(opts, WithExtraAttributes(attribute.String("service.name", serviceName)))
	return Tracing(opts...)
}

// StreamTracing creates a distributed tracing middleware for streaming RPCs
func StreamTracing(opts ...TracingOption) func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	// Default configuration
	config := &TracingConfig{
		TracerName:   "grpc-guardian",
		Propagator:   otel.GetTextMapPropagator(),
		RecordErrors: true,
		RecordEvents: true,
	}

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	// Get or create tracer
	if config.Tracer == nil {
		config.Tracer = otel.Tracer(config.TracerName)
	}

	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()

		// Extract trace context from incoming metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			ctx = config.Propagator.Extract(ctx, &metadataCarrier{md: md})
		}

		// Start a new span
		ctx, span := config.Tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(config.ExtraAttrs...),
		)
		defer span.End()

		// Add RPC attributes
		span.SetAttributes(
			attribute.String("rpc.system", "grpc"),
			attribute.String("rpc.service", extractServiceName(info.FullMethod)),
			attribute.String("rpc.method", extractMethodName(info.FullMethod)),
			attribute.Bool("rpc.stream.client_streaming", info.IsClientStream),
			attribute.Bool("rpc.stream.server_streaming", info.IsServerStream),
		)

		// Record stream event if enabled
		if config.RecordEvents {
			span.AddEvent("grpc.stream.started")
		}

		// Wrap the server stream with traced context
		wrappedStream := &tracedServerStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		// Call handler
		err := handler(srv, wrappedStream)

		// Record stream completion event if enabled
		if config.RecordEvents {
			span.AddEvent("grpc.stream.completed")
		}

		// Handle errors
		if err != nil {
			st := status.Convert(err)

			// Set span status
			span.SetStatus(codes.Error, st.Message())

			// Add error attributes
			span.SetAttributes(
				attribute.String("rpc.grpc.status_code", st.Code().String()),
				attribute.String("error.message", st.Message()),
			)

			// Record error if enabled
			if config.RecordErrors {
				span.RecordError(err)
			}
		} else {
			// Set successful status
			span.SetStatus(codes.Ok, "")
			span.SetAttributes(
				attribute.String("rpc.grpc.status_code", "OK"),
			)
		}

		return err
	}
}

// metadataCarrier adapts grpc metadata to be a TextMapCarrier
type metadataCarrier struct {
	md metadata.MD
}

// Get returns the value associated with the passed key.
func (mc *metadataCarrier) Get(key string) string {
	values := mc.md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// Set stores the key-value pair.
func (mc *metadataCarrier) Set(key string, value string) {
	mc.md.Set(key, value)
}

// Keys lists the keys stored in this carrier.
func (mc *metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(mc.md))
	for k := range mc.md {
		keys = append(keys, k)
	}
	return keys
}

// tracedServerStream wraps grpc.ServerStream with a traced context
type tracedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the traced context
func (ss *tracedServerStream) Context() context.Context {
	return ss.ctx
}

// Helper functions to extract service and method names from full method path
func extractServiceName(fullMethod string) string {
	// fullMethod format: "/package.Service/Method"
	for i := 1; i < len(fullMethod); i++ {
		if fullMethod[i] == '/' {
			return fullMethod[1:i]
		}
	}
	return fullMethod
}

func extractMethodName(fullMethod string) string {
	// fullMethod format: "/package.Service/Method"
	for i := len(fullMethod) - 1; i >= 0; i-- {
		if fullMethod[i] == '/' {
			return fullMethod[i+1:]
		}
	}
	return fullMethod
}

// InjectTraceContext injects trace context into outgoing gRPC metadata
// This is useful for client-side tracing
func InjectTraceContext(ctx context.Context) context.Context {
	propagator := otel.GetTextMapPropagator()
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}

	carrier := &metadataCarrier{md: md}
	propagator.Inject(ctx, carrier)

	return metadata.NewOutgoingContext(ctx, md)
}

// StartSpan is a helper function to manually start a span in handlers
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tracer := otel.Tracer("grpc-guardian")
	return tracer.Start(ctx, name, opts...)
}

// SpanFromContext returns the current span from the context
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// AddEventToSpan adds an event to the current span
func AddEventToSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// SetSpanAttribute sets an attribute on the current span
func SetSpanAttribute(ctx context.Context, key string, value interface{}) {
	span := trace.SpanFromContext(ctx)

	var attr attribute.KeyValue
	switch v := value.(type) {
	case string:
		attr = attribute.String(key, v)
	case int:
		attr = attribute.Int(key, v)
	case int64:
		attr = attribute.Int64(key, v)
	case float64:
		attr = attribute.Float64(key, v)
	case bool:
		attr = attribute.Bool(key, v)
	default:
		attr = attribute.String(key, fmt.Sprintf("%v", v))
	}

	span.SetAttributes(attr)
}

// RecordError records an error in the current span
func RecordError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
