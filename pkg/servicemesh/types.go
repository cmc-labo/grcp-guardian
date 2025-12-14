// Package servicemesh provides integration with service mesh platforms like Istio and Linkerd
package servicemesh

import (
	"context"
	"crypto/tls"
	"time"

	"google.golang.org/grpc/metadata"
)

// MeshProvider represents different service mesh implementations
type MeshProvider string

const (
	// ProviderIstio represents Istio service mesh
	ProviderIstio MeshProvider = "istio"
	// ProviderLinkerd represents Linkerd service mesh
	ProviderLinkerd MeshProvider = "linkerd"
	// ProviderConsul represents Consul Connect
	ProviderConsul MeshProvider = "consul"
)

// Config holds service mesh configuration
type Config struct {
	// Provider specifies which service mesh to integrate with
	Provider MeshProvider

	// ServiceName is the name of this service in the mesh
	ServiceName string

	// Namespace is the service mesh namespace
	Namespace string

	// EnableMTLS enables mutual TLS authentication
	EnableMTLS bool

	// TLSConfig for mTLS connections
	TLSConfig *tls.Config

	// EnableTrafficSplitting enables traffic splitting/routing
	EnableTrafficSplitting bool

	// EnableFaultInjection enables fault injection integration
	EnableFaultInjection bool

	// CustomHeaders are additional headers to propagate
	CustomHeaders []string

	// Timeout for mesh operations
	Timeout time.Duration
}

// MeshMetadata contains service mesh related metadata
type MeshMetadata struct {
	// RequestID is a unique request identifier
	RequestID string

	// TraceID for distributed tracing
	TraceID string

	// SpanID for distributed tracing
	SpanID string

	// ParentSpanID for distributed tracing
	ParentSpanID string

	// ServiceVersion is the version of the calling service
	ServiceVersion string

	// SourceWorkload is the workload that initiated the request
	SourceWorkload string

	// SourceNamespace is the namespace of the source workload
	SourceNamespace string

	// DestinationWorkload is the target workload
	DestinationWorkload string

	// DestinationNamespace is the namespace of the destination
	DestinationNamespace string

	// CanaryWeight for traffic splitting (0.0-1.0)
	CanaryWeight float64

	// CustomLabels are additional mesh-specific labels
	CustomLabels map[string]string
}

// ServiceMesh defines the interface for service mesh integrations
type ServiceMesh interface {
	// ExtractMetadata extracts mesh metadata from gRPC context
	ExtractMetadata(ctx context.Context) (*MeshMetadata, error)

	// InjectMetadata injects mesh metadata into gRPC context
	InjectMetadata(ctx context.Context, metadata *MeshMetadata) context.Context

	// ValidateMTLS validates mutual TLS certificates
	ValidateMTLS(ctx context.Context) error

	// GetServiceEndpoints returns available service endpoints
	GetServiceEndpoints(serviceName string) ([]string, error)

	// ReportMetrics reports metrics to the mesh
	ReportMetrics(ctx context.Context, metrics *Metrics) error

	// ShouldRetry determines if a request should be retried based on mesh policy
	ShouldRetry(err error) bool

	// GetTrafficSplit returns traffic split configuration
	GetTrafficSplit(serviceName string) (*TrafficSplit, error)
}

// Metrics represents metrics to report to the service mesh
type Metrics struct {
	RequestDuration time.Duration
	RequestSize     int64
	ResponseSize    int64
	StatusCode      int
	Success         bool
	Method          string
}

// TrafficSplit represents traffic splitting configuration
type TrafficSplit struct {
	// Routes define traffic distribution
	Routes []Route
}

// Route represents a single traffic route
type Route struct {
	// Destination service name or version
	Destination string

	// Weight is the percentage of traffic (0-100)
	Weight int

	// Headers to match for this route
	Headers map[string]string

	// Priority for route matching (higher = higher priority)
	Priority int
}

// HeaderKeys defines common header keys used by service meshes
var HeaderKeys = struct {
	// Istio headers
	IstioRequestID       string
	IstioTraceID         string
	IstioSpanID          string
	IstioParentSpanID    string
	IstioSourceWorkload  string
	IstioSourceNamespace string
	IstioDestWorkload    string
	IstioDestNamespace   string

	// Linkerd headers
	LinkerdID            string
	LinkerdContextID     string
	LinkerdTraceID       string
	LinkerdSpanID        string
	LinkerdParentSpanID  string
	LinkerdDtab          string

	// Common headers
	XRequestID           string
	XB3TraceID           string
	XB3SpanID            string
	XB3ParentSpanID      string
	XB3Sampled           string
	XForwardedFor        string
	XForwardedHost       string
	XForwardedProto      string
}{
	// Istio
	IstioRequestID:       "x-request-id",
	IstioTraceID:         "x-b3-traceid",
	IstioSpanID:          "x-b3-spanid",
	IstioParentSpanID:    "x-b3-parentspanid",
	IstioSourceWorkload:  "x-envoy-decorator-operation",
	IstioSourceNamespace: "x-envoy-peer-metadata",
	IstioDestWorkload:    "x-envoy-upstream-service-time",
	IstioDestNamespace:   ":authority",

	// Linkerd
	LinkerdID:           "l5d-dst-override",
	LinkerdContextID:    "l5d-ctx-trace",
	LinkerdTraceID:      "l5d-ctx-traceid",
	LinkerdSpanID:       "l5d-ctx-spanid",
	LinkerdParentSpanID: "l5d-ctx-parentid",
	LinkerdDtab:         "l5d-dtab",

	// Common
	XRequestID:      "x-request-id",
	XB3TraceID:      "x-b3-traceid",
	XB3SpanID:       "x-b3-spanid",
	XB3ParentSpanID: "x-b3-parentspanid",
	XB3Sampled:      "x-b3-sampled",
	XForwardedFor:   "x-forwarded-for",
	XForwardedHost:  "x-forwarded-host",
	XForwardedProto: "x-forwarded-proto",
}

// ExtractHeader extracts a header value from gRPC metadata
func ExtractHeader(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}

	return values[0]
}

// InjectHeader injects a header into gRPC metadata
func InjectHeader(ctx context.Context, key, value string) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}

	md.Set(key, value)
	return metadata.NewOutgoingContext(ctx, md)
}

// InjectHeaders injects multiple headers into gRPC metadata
func InjectHeaders(ctx context.Context, headers map[string]string) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}

	for key, value := range headers {
		md.Set(key, value)
	}

	return metadata.NewOutgoingContext(ctx, md)
}
