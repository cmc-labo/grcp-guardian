package servicemesh

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// LinkerdMesh implements ServiceMesh interface for Linkerd
type LinkerdMesh struct {
	config *Config
}

// NewLinkerdMesh creates a new Linkerd service mesh integration
func NewLinkerdMesh(config *Config) (*LinkerdMesh, error) {
	if config == nil {
		return nil, errors.New("config cannot be nil")
	}

	config.Provider = ProviderLinkerd

	return &LinkerdMesh{
		config: config
	}, nil
}

// ExtractMetadata extracts Linkerd metadata from gRPC context
func (l *LinkerdMesh) ExtractMetadata(ctx context.Context) (*MeshMetadata, error) {
	metadata := &MeshMetadata{
		CustomLabels: make(map[string]string),
	}

	// Extract Linkerd context headers
	metadata.RequestID = ExtractHeader(ctx, HeaderKeys.LinkerdContextID)
	metadata.TraceID = ExtractHeader(ctx, HeaderKeys.LinkerdTraceID)
	metadata.SpanID = ExtractHeader(ctx, HeaderKeys.LinkerdSpanID)
	metadata.ParentSpanID = ExtractHeader(ctx, HeaderKeys.LinkerdParentSpanID)

	// Extract Linkerd-specific headers
	l.extractLinkerdHeaders(ctx, metadata)

	// Extract custom headers
	for _, header := range l.config.CustomHeaders {
		value := ExtractHeader(ctx, header)
		if value != "" {
			metadata.CustomLabels[header] = value
		}
	}

	return metadata, nil
}

// extractLinkerdHeaders extracts Linkerd-specific headers
func (l *LinkerdMesh) extractLinkerdHeaders(ctx context.Context, metadata *MeshMetadata) {
	// Extract Dtab (Delegation Tables) - Linkerd's dynamic routing
	dtab := ExtractHeader(ctx, HeaderKeys.LinkerdDtab)
	if dtab != "" {
		metadata.CustomLabels["dtab"] = dtab
	}

	// Extract destination override
	dstOverride := ExtractHeader(ctx, HeaderKeys.LinkerdID)
	if dstOverride != "" {
		metadata.DestinationWorkload = dstOverride
	}
}

// InjectMetadata injects Linkerd metadata into gRPC context
func (l *LinkerdMesh) InjectMetadata(ctx context.Context, metadata *MeshMetadata) context.Context {
	headers := make(map[string]string)

	// Inject Linkerd context headers
	if metadata.TraceID != "" {
		headers[HeaderKeys.LinkerdTraceID] = metadata.TraceID
	}
	if metadata.SpanID != "" {
		headers[HeaderKeys.LinkerdSpanID] = metadata.SpanID
	}
	if metadata.ParentSpanID != "" {
		headers[HeaderKeys.LinkerdParentSpanID] = metadata.ParentSpanID
	}

	// Inject context ID (similar to request ID)
	if metadata.RequestID != "" {
		headers[HeaderKeys.LinkerdContextID] = metadata.RequestID
	}

	// Inject destination override if specified
	if metadata.DestinationWorkload != "" {
		headers[HeaderKeys.LinkerdID] = metadata.DestinationWorkload
	}

	// Inject custom labels
	for key, value := range metadata.CustomLabels {
		if key == "dtab" {
			headers[HeaderKeys.LinkerdDtab] = value
		} else {
			headers[key] = value
		}
	}

	return InjectHeaders(ctx, headers)
}

// ValidateMTLS validates mutual TLS certificates in Linkerd
func (l *LinkerdMesh) ValidateMTLS(ctx context.Context) error {
	if !l.config.EnableMTLS {
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

	// Validate Linkerd identity
	for _, chain := range tlsInfo.State.VerifiedChains {
		for _, cert := range chain {
			if err := l.validateLinkerdIdentity(cert); err != nil {
				return fmt.Errorf("Linkerd identity validation failed: %w", err)
			}
		}
	}

	return nil
}

// validateLinkerdIdentity validates Linkerd identity in certificate
func (l *LinkerdMesh) validateLinkerdIdentity(cert *x509.Certificate) error {
	// Linkerd uses the format: <service>.<namespace>.serviceaccount.identity.linkerd.cluster.local
	for _, dnsName := range cert.DNSNames {
		if strings.Contains(dnsName, ".serviceaccount.identity.linkerd.") {
			parts := strings.Split(dnsName, ".")
			if len(parts) >= 2 {
				serviceAccount := parts[0]
				namespace := parts[1]

				// Validate namespace if configured
				if l.config.Namespace != "" && namespace != l.config.Namespace {
					return fmt.Errorf("namespace mismatch: expected %s, got %s",
						l.config.Namespace, namespace)
				}

				// Service account validated
				_ = serviceAccount // Use for further validation if needed
			}
			return nil
		}
	}

	return errors.New("no valid Linkerd identity found in certificate")
}

// GetServiceEndpoints returns service endpoints from Linkerd
func (l *LinkerdMesh) GetServiceEndpoints(serviceName string) ([]string, error) {
	// In a real implementation, this would query Linkerd's destination API
	// Linkerd provides a Destination gRPC API for service discovery
	// For now, return a placeholder
	return nil, errors.New("service discovery not yet implemented")
}

// ReportMetrics reports metrics to Linkerd
func (l *LinkerdMesh) ReportMetrics(ctx context.Context, metrics *Metrics) error {
	// Linkerd automatically collects metrics via its proxy
	// This is a placeholder for custom metric reporting
	// Metrics are exposed in Prometheus format by default

	// Custom metrics could be added via Linkerd's Tap API or
	// by exposing additional Prometheus metrics

	return nil
}

// ShouldRetry determines if a request should be retried based on Linkerd policy
func (l *LinkerdMesh) ShouldRetry(err error) bool {
	if err == nil {
		return false
	}

	// Check error type and decide if retry is appropriate
	// Linkerd has automatic retries for certain error types
	errStr := err.Error()

	// Common retryable errors in Linkerd
	retryableErrors := []string{
		"unavailable",
		"deadline exceeded",
		"connect: connection refused",
		"connection reset",
		"broken pipe",
	}

	for _, retryable := range retryableErrors {
		if strings.Contains(strings.ToLower(errStr), retryable) {
			return true
		}
	}

	return false
}

// GetTrafficSplit returns traffic split configuration from Linkerd TrafficSplit resource
func (l *LinkerdMesh) GetTrafficSplit(serviceName string) (*TrafficSplit, error) {
	if !l.config.EnableTrafficSplitting {
		return nil, errors.New("traffic splitting not enabled")
	}

	// In a real implementation, this would query Linkerd's TrafficSplit CRD
	// via the Kubernetes API
	// Linkerd uses the SMI (Service Mesh Interface) TrafficSplit spec

	// Placeholder implementation
	return &TrafficSplit{
		Routes: []Route{
			{
				Destination: serviceName,
				Weight:      100,
				Priority:    1,
			},
		},
	}, nil
}

// GetConfig returns the Linkerd configuration
func (l *LinkerdMesh) GetConfig() *Config {
	return l.config
}

// EnableRetryBudget enables Linkerd's retry budget feature
func (l *LinkerdMesh) EnableRetryBudget() {
	// Linkerd has a retry budget to prevent retry storms
	// This would configure the retry budget via ServiceProfile
	// For now, this is a placeholder
}

// SetServiceProfile sets the Linkerd ServiceProfile for advanced features
func (l *LinkerdMesh) SetServiceProfile(profile interface{}) error {
	// ServiceProfile is a Linkerd CRD that defines:
	// - Routes and their properties
	// - Retry budgets
	// - Timeout policies
	// This would integrate with Kubernetes API to manage ServiceProfiles
	return errors.New("ServiceProfile management not yet implemented")
}

// GetTapStream enables Linkerd Tap API for real-time traffic observation
func (l *LinkerdMesh) GetTapStream(ctx context.Context) error {
	// Linkerd's Tap API allows real-time observation of requests
	// This would integrate with Linkerd's Tap gRPC API
	return errors.New("Tap API not yet implemented")
}

// EnablePerRouteMetrics enables per-route metrics in Linkerd
func (l *LinkerdMesh) EnablePerRouteMetrics() {
	// Linkerd can provide per-route metrics when ServiceProfile is configured
	// This would set the necessary labels for detailed metrics
}
