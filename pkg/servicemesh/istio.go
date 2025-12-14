package servicemesh

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// IstioMesh implements ServiceMesh interface for Istio
type IstioMesh struct {
	config *Config
}

// NewIstioMesh creates a new Istio service mesh integration
func NewIstioMesh(config *Config) (*IstioMesh, error) {
	if config == nil {
		return nil, errors.New("config cannot be nil")
	}

	config.Provider = ProviderIstio

	return &IstioMesh{
		config: config,
	}, nil
}

// ExtractMetadata extracts Istio metadata from gRPC context
func (i *IstioMesh) ExtractMetadata(ctx context.Context) (*MeshMetadata, error) {
	metadata := &MeshMetadata{
		CustomLabels: make(map[string]string),
	}

	// Extract Istio headers
	metadata.RequestID = ExtractHeader(ctx, HeaderKeys.IstioRequestID)
	metadata.TraceID = ExtractHeader(ctx, HeaderKeys.IstioTraceID)
	metadata.SpanID = ExtractHeader(ctx, HeaderKeys.IstioSpanID)
	metadata.ParentSpanID = ExtractHeader(ctx, HeaderKeys.IstioParentSpanID)

	// Extract Envoy metadata
	envoyMeta := ExtractHeader(ctx, HeaderKeys.IstioSourceNamespace)
	if envoyMeta != "" {
		if err := i.parseEnvoyMetadata(envoyMeta, metadata); err == nil {
			// Metadata parsed successfully
		}
	}

	// Extract source workload information
	sourceOp := ExtractHeader(ctx, HeaderKeys.IstioSourceWorkload)
	if sourceOp != "" {
		metadata.SourceWorkload = sourceOp
	}

	// Extract custom Istio labels
	i.extractCustomLabels(ctx, metadata)

	return metadata, nil
}

// parseEnvoyMetadata parses Envoy peer metadata
func (i *IstioMesh) parseEnvoyMetadata(encodedMeta string, metadata *MeshMetadata) error {
	// Envoy metadata is base64 encoded JSON
	decoded, err := base64.StdEncoding.DecodeString(encodedMeta)
	if err != nil {
		return err
	}

	var envoyMeta map[string]interface{}
	if err := json.Unmarshal(decoded, &envoyMeta); err != nil {
		return err
	}

	// Extract workload information
	if workload, ok := envoyMeta["WORKLOAD_NAME"].(string); ok {
		metadata.SourceWorkload = workload
	}

	if namespace, ok := envoyMeta["NAMESPACE"].(string); ok {
		metadata.SourceNamespace = namespace
	}

	if version, ok := envoyMeta["VERSION"].(string); ok {
		metadata.ServiceVersion = version
	}

	// Extract labels
	if labels, ok := envoyMeta["LABELS"].(map[string]interface{}); ok {
		for k, v := range labels {
			if str, ok := v.(string); ok {
				metadata.CustomLabels[k] = str
			}
		}
	}

	return nil
}

// extractCustomLabels extracts custom Istio labels from context
func (i *IstioMesh) extractCustomLabels(ctx context.Context, metadata *MeshMetadata) {
	for _, header := range i.config.CustomHeaders {
		value := ExtractHeader(ctx, header)
		if value != "" {
			metadata.CustomLabels[header] = value
		}
	}
}

// InjectMetadata injects Istio metadata into gRPC context
func (i *IstioMesh) InjectMetadata(ctx context.Context, metadata *MeshMetadata) context.Context {
	headers := make(map[string]string)

	// Inject trace context (B3 propagation)
	if metadata.TraceID != "" {
		headers[HeaderKeys.XB3TraceID] = metadata.TraceID
	}
	if metadata.SpanID != "" {
		headers[HeaderKeys.XB3SpanID] = metadata.SpanID
	}
	if metadata.ParentSpanID != "" {
		headers[HeaderKeys.XB3ParentSpanID] = metadata.ParentSpanID
	}
	headers[HeaderKeys.XB3Sampled] = "1" // Always sample for now

	// Inject request ID
	if metadata.RequestID != "" {
		headers[HeaderKeys.XRequestID] = metadata.RequestID
	}

	// Inject custom labels as headers
	for key, value := range metadata.CustomLabels {
		headers[key] = value
	}

	return InjectHeaders(ctx, headers)
}

// ValidateMTLS validates mutual TLS certificates in Istio
func (i *IstioMesh) ValidateMTLS(ctx context.Context) error {
	if !i.config.EnableMTLS {
		return nil // mTLS not enabled
	}

	// Extract peer information
	p, ok := peer.FromContext(ctx)
	if !ok {
		return errors.New("no peer information in context")
	}

	// Check TLS auth info
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return errors.New("peer not using TLS")
	}

	// Validate certificate chains
	if len(tlsInfo.State.VerifiedChains) == 0 {
		return errors.New("no verified certificate chains")
	}

	// Validate SPIFFE ID (Istio uses SPIFFE for service identity)
	for _, chain := range tlsInfo.State.VerifiedChains {
		for _, cert := range chain {
			if err := i.validateSPIFFEID(cert); err != nil {
				return fmt.Errorf("SPIFFE ID validation failed: %w", err)
			}
		}
	}

	return nil
}

// validateSPIFFEID validates SPIFFE ID in certificate
func (i *IstioMesh) validateSPIFFEID(cert *x509.Certificate) error {
	// Check for SPIFFE ID in SAN URIs
	for _, uri := range cert.URIs {
		if strings.HasPrefix(uri.String(), "spiffe://") {
			// SPIFFE ID format: spiffe://<trust-domain>/ns/<namespace>/sa/<service-account>
			parts := strings.Split(uri.String(), "/")
			if len(parts) >= 6 {
				namespace := parts[4]
				serviceAccount := parts[6]

				// Validate namespace if configured
				if i.config.Namespace != "" && namespace != i.config.Namespace {
					return fmt.Errorf("namespace mismatch: expected %s, got %s",
						i.config.Namespace, namespace)
				}

				// Additional validation can be added here
				_ = serviceAccount // Use service account for further validation if needed
			}
			return nil
		}
	}

	return errors.New("no valid SPIFFE ID found in certificate")
}

// GetServiceEndpoints returns service endpoints from Istio service registry
func (i *IstioMesh) GetServiceEndpoints(serviceName string) ([]string, error) {
	// In a real implementation, this would query Istio's service discovery
	// For now, return a placeholder
	// This would typically integrate with Pilot/Istiod API
	return nil, errors.New("service discovery not yet implemented")
}

// ReportMetrics reports metrics to Istio telemetry
func (i *IstioMesh) ReportMetrics(ctx context.Context, metrics *Metrics) error {
	// Istio automatically collects metrics via Envoy sidecar
	// This is a placeholder for custom metric reporting
	// In production, you might send additional metrics to Mixer or Prometheus

	// Custom metric attributes could be added to context for Envoy to pick up
	// via gRPC metadata

	return nil
}

// ShouldRetry determines if a request should be retried based on Istio policy
func (i *IstioMesh) ShouldRetry(err error) bool {
	if err == nil {
		return false
	}

	// Check error type and decide if retry is appropriate
	// This would integrate with Istio's retry policies
	errStr := err.Error()

	// Common retryable errors in Istio
	retryableErrors := []string{
		"unavailable",
		"deadline exceeded",
		"connect: connection refused",
		"reset",
	}

	for _, retryable := range retryableErrors {
		if strings.Contains(strings.ToLower(errStr), retryable) {
			return true
		}
	}

	return false
}

// GetTrafficSplit returns traffic split configuration from Istio VirtualService
func (i *IstioMesh) GetTrafficSplit(serviceName string) (*TrafficSplit, error) {
	if !i.config.EnableTrafficSplitting {
		return nil, errors.New("traffic splitting not enabled")
	}

	// In a real implementation, this would query Istio's VirtualService configuration
	// via the Kubernetes API or Istio config API
	// For now, return a placeholder

	return &TrafficSplit{
		Routes: []Route{
			{
				Destination: serviceName + "-v1",
				Weight:      90,
				Priority:    1,
			},
			{
				Destination: serviceName + "-v2",
				Weight:      10,
				Headers: map[string]string{
					"x-canary": "true",
				},
				Priority: 2,
			},
		},
	}, nil
}

// GetConfig returns the Istio configuration
func (i *IstioMesh) GetConfig() *Config {
	return i.config
}

// EnableFaultInjection enables Istio fault injection
func (i *IstioMesh) EnableFaultInjection() {
	i.config.EnableFaultInjection = true
}

// DisableFaultInjection disables Istio fault injection
func (i *IstioMesh) DisableFaultInjection() {
	i.config.EnableFaultInjection = false
}

// IsFaultInjectionEnabled returns whether fault injection is enabled
func (i *IstioMesh) IsFaultInjectionEnabled() bool {
	return i.config.EnableFaultInjection
}
