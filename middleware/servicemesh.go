package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/grpc-guardian/grpc-guardian/pkg/servicemesh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ServiceMeshMiddleware provides integration with service mesh platforms
type ServiceMeshMiddleware struct {
	mesh                servicemesh.ServiceMesh
	config              *ServiceMeshConfig
	enableMetadataLog   bool
	enableMTLSValidation bool
}

// ServiceMeshConfig holds configuration for service mesh middleware
type ServiceMeshConfig struct {
	// ValidateMTLS enables mutual TLS validation
	ValidateMTLS bool

	// PropagateHeaders enables automatic header propagation
	PropagateHeaders bool

	// ReportMetrics enables metric reporting to the mesh
	ReportMetrics bool

	// LogMetadata enables logging of mesh metadata
	LogMetadata bool

	// EnableTrafficSplit enables traffic splitting/routing
	EnableTrafficSplit bool

	// OnMetadataExtracted is called after metadata is extracted
	OnMetadataExtracted func(*servicemesh.MeshMetadata)

	// OnError is called when an error occurs
	OnError func(error)
}

// ServiceMeshOption is a functional option for configuring ServiceMeshMiddleware
type ServiceMeshOption func(*ServiceMeshConfig)

// WithMTLSValidation enables mutual TLS validation
func WithMTLSValidation() ServiceMeshOption {
	return func(c *ServiceMeshConfig) {
		c.ValidateMTLS = true
	}
}

// WithHeaderPropagation enables automatic header propagation
func WithHeaderPropagation() ServiceMeshOption {
	return func(c *ServiceMeshConfig) {
		c.PropagateHeaders = true
	}
}

// WithMeshMetrics enables metric reporting to the mesh
func WithMeshMetrics() ServiceMeshOption {
	return func(c *ServiceMeshConfig) {
		c.ReportMetrics = true
	}
}

// WithMetadataLogging enables logging of mesh metadata
func WithMetadataLogging() ServiceMeshOption {
	return func(c *ServiceMeshConfig) {
		c.LogMetadata = true
	}
}

// WithTrafficSplit enables traffic splitting
func WithTrafficSplit() ServiceMeshOption {
	return func(c *ServiceMeshConfig) {
		c.EnableTrafficSplit = true
	}
}

// WithMetadataCallback sets callback for extracted metadata
func WithMetadataCallback(callback func(*servicemesh.MeshMetadata)) ServiceMeshOption {
	return func(c *ServiceMeshConfig) {
		c.OnMetadataExtracted = callback
	}
}

// WithErrorCallback sets callback for errors
func WithErrorCallback(callback func(error)) ServiceMeshOption {
	return func(c *ServiceMeshConfig) {
		c.OnError = callback
	}
}

// NewServiceMeshMiddleware creates a new service mesh middleware
func NewServiceMeshMiddleware(mesh servicemesh.ServiceMesh, opts ...ServiceMeshOption) *ServiceMeshMiddleware {
	config := &ServiceMeshConfig{
		PropagateHeaders: true, // Enable by default
		ReportMetrics:    false,
		ValidateMTLS:     false,
		LogMetadata:      false,
	}

	for _, opt := range opts {
		opt(config)
	}

	return &ServiceMeshMiddleware{
		mesh:                 mesh,
		config:               config,
		enableMetadataLog:    config.LogMetadata,
		enableMTLSValidation: config.ValidateMTLS,
	}
}

// UnaryServerInterceptor returns a gRPC unary server interceptor for service mesh
func (m *ServiceMeshMiddleware) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()

		// Extract mesh metadata
		metadata, err := m.mesh.ExtractMetadata(ctx)
		if err != nil {
			if m.config.OnError != nil {
				m.config.OnError(fmt.Errorf("failed to extract mesh metadata: %w", err))
			}
			// Continue anyway - metadata extraction is not critical
		}

		// Log metadata if enabled
		if m.enableMetadataLog && metadata != nil {
			m.logMetadata(info.FullMethod, metadata)
		}

		// Callback for metadata
		if m.config.OnMetadataExtracted != nil && metadata != nil {
			m.config.OnMetadataExtracted(metadata)
		}

		// Validate mTLS if enabled
		if m.enableMTLSValidation {
			if err := m.mesh.ValidateMTLS(ctx); err != nil {
				if m.config.OnError != nil {
					m.config.OnError(fmt.Errorf("mTLS validation failed: %w", err))
				}
				return nil, status.Errorf(codes.Unauthenticated, "mTLS validation failed: %v", err)
			}
		}

		// Inject metadata for outgoing requests if configured
		if m.config.PropagateHeaders && metadata != nil {
			ctx = m.mesh.InjectMetadata(ctx, metadata)
		}

		// Call the handler
		resp, err := handler(ctx, req)

		// Report metrics if enabled
		if m.config.ReportMetrics {
			duration := time.Since(start)
			metrics := &servicemesh.Metrics{
				RequestDuration: duration,
				Method:          info.FullMethod,
				Success:         err == nil,
			}

			if err != nil {
				if st, ok := status.FromError(err); ok {
					metrics.StatusCode = int(st.Code())
				}
			} else {
				metrics.StatusCode = int(codes.OK)
			}

			if reportErr := m.mesh.ReportMetrics(ctx, metrics); reportErr != nil {
				if m.config.OnError != nil {
					m.config.OnError(fmt.Errorf("failed to report metrics: %w", reportErr))
				}
			}
		}

		return resp, err
	}
}

// UnaryClientInterceptor returns a gRPC unary client interceptor for service mesh
func (m *ServiceMeshMiddleware) UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// Extract metadata from current context
		metadata, err := m.mesh.ExtractMetadata(ctx)
		if err != nil {
			// Create new metadata if extraction fails
			metadata = &servicemesh.MeshMetadata{
				CustomLabels: make(map[string]string),
			}
		}

		// Inject metadata into outgoing context
		if m.config.PropagateHeaders {
			ctx = m.mesh.InjectMetadata(ctx, metadata)
		}

		// Make the call
		start := time.Now()
		err = invoker(ctx, method, req, reply, cc, opts...)

		// Report metrics if enabled
		if m.config.ReportMetrics {
			duration := time.Since(start)
			metrics := &servicemesh.Metrics{
				RequestDuration: duration,
				Method:          method,
				Success:         err == nil,
			}

			if err != nil {
				if st, ok := status.FromError(err); ok {
					metrics.StatusCode = int(st.Code())
				}
			} else {
				metrics.StatusCode = int(codes.OK)
			}

			if reportErr := m.mesh.ReportMetrics(ctx, metrics); reportErr != nil && m.config.OnError != nil {
				m.config.OnError(fmt.Errorf("failed to report metrics: %w", reportErr))
			}
		}

		return err
	}
}

// StreamServerInterceptor returns a gRPC stream server interceptor for service mesh
func (m *ServiceMeshMiddleware) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()

		// Extract mesh metadata
		metadata, err := m.mesh.ExtractMetadata(ctx)
		if err != nil && m.config.OnError != nil {
			m.config.OnError(fmt.Errorf("failed to extract mesh metadata: %w", err))
		}

		// Log metadata if enabled
		if m.enableMetadataLog && metadata != nil {
			m.logMetadata(info.FullMethod, metadata)
		}

		// Validate mTLS if enabled
		if m.enableMTLSValidation {
			if err := m.mesh.ValidateMTLS(ctx); err != nil {
				if m.config.OnError != nil {
					m.config.OnError(fmt.Errorf("mTLS validation failed: %w", err))
				}
				return status.Errorf(codes.Unauthenticated, "mTLS validation failed: %v", err)
			}
		}

		// Call the handler
		return handler(srv, ss)
	}
}

// logMetadata logs mesh metadata (if logging is available)
func (m *ServiceMeshMiddleware) logMetadata(method string, metadata *servicemesh.MeshMetadata) {
	// This is a simple logger - in production, integrate with your logging framework
	fmt.Printf("[ServiceMesh] Method: %s, RequestID: %s, TraceID: %s, Source: %s/%s\n",
		method,
		metadata.RequestID,
		metadata.TraceID,
		metadata.SourceNamespace,
		metadata.SourceWorkload,
	)
}

// Istio creates a new service mesh middleware for Istio
func Istio(meshConfig *servicemesh.Config, opts ...ServiceMeshOption) (*ServiceMeshMiddleware, error) {
	mesh, err := servicemesh.NewIstioMesh(meshConfig)
	if err != nil {
		return nil, err
	}

	return NewServiceMeshMiddleware(mesh, opts...), nil
}

// Linkerd creates a new service mesh middleware for Linkerd
func Linkerd(meshConfig *servicemesh.Config, opts ...ServiceMeshOption) (*ServiceMeshMiddleware, error) {
	mesh, err := servicemesh.NewLinkerdMesh(meshConfig)
	if err != nil {
		return nil, err
	}

	return NewServiceMeshMiddleware(mesh, opts...), nil
}

// IstioSimple creates a simple Istio middleware with default settings
func IstioSimple(serviceName, namespace string) (*ServiceMeshMiddleware, error) {
	config := &servicemesh.Config{
		ServiceName:            serviceName,
		Namespace:              namespace,
		EnableMTLS:             false,
		EnableTrafficSplitting: false,
	}

	return Istio(config,
		WithHeaderPropagation(),
		WithMetadataLogging(),
	)
}

// LinkerdSimple creates a simple Linkerd middleware with default settings
func LinkerdSimple(serviceName, namespace string) (*ServiceMeshMiddleware, error) {
	config := &servicemesh.Config{
		ServiceName:            serviceName,
		Namespace:              namespace,
		EnableMTLS:             false,
		EnableTrafficSplitting: false,
	}

	return Linkerd(config,
		WithHeaderPropagation(),
		WithMetadataLogging(),
	)
}
