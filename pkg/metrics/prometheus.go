package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// PrometheusCollector implements MetricsCollector for Prometheus
type PrometheusCollector struct {
	config   *Config
	registry *prometheus.Registry

	// Request metrics
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	activeRequests  *prometheus.GaugeVec

	// Error metrics
	errorsTotal *prometheus.CounterVec

	// Message size metrics
	messageSent     *prometheus.HistogramVec
	messageReceived *prometheus.HistogramVec
}

// NewPrometheusCollector creates a new Prometheus metrics collector
func NewPrometheusCollector(opts ...ConfigOption) (*PrometheusCollector, error) {
	config := DefaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	registry := prometheus.NewRegistry()
	collector := &PrometheusCollector{
		config:   config,
		registry: registry,
	}

	// Initialize metrics
	if err := collector.initMetrics(); err != nil {
		return nil, err
	}

	return collector, nil
}

// initMetrics initializes all Prometheus metrics
func (p *PrometheusCollector) initMetrics() error {
	labels := []string{"method", "code"}
	if !p.config.EnablePerMethodMetrics {
		labels = []string{"code"}
	}

	// Total requests counter
	p.requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   p.config.Namespace,
			Subsystem:   p.config.Subsystem,
			Name:        "requests_total",
			Help:        "Total number of gRPC requests handled",
			ConstLabels: p.config.ConstLabels,
		},
		labels,
	)

	// Request duration histogram
	if p.config.EnableHistogram {
		p.requestDuration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   p.config.Namespace,
				Subsystem:   p.config.Subsystem,
				Name:        "request_duration_seconds",
				Help:        "Histogram of gRPC request duration in seconds",
				Buckets:     p.config.HistogramBuckets,
				ConstLabels: p.config.ConstLabels,
			},
			labels,
		)
	}

	// Active requests gauge
	gaugeLabels := []string{"method"}
	if !p.config.EnablePerMethodMetrics {
		gaugeLabels = []string{}
	}

	p.activeRequests = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   p.config.Namespace,
			Subsystem:   p.config.Subsystem,
			Name:        "active_requests",
			Help:        "Number of active gRPC requests",
			ConstLabels: p.config.ConstLabels,
		},
		gaugeLabels,
	)

	// Total errors counter
	errorLabels := []string{"method", "error_type"}
	if !p.config.EnablePerMethodMetrics {
		errorLabels = []string{"error_type"}
	}

	p.errorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   p.config.Namespace,
			Subsystem:   p.config.Subsystem,
			Name:        "errors_total",
			Help:        "Total number of gRPC errors",
			ConstLabels: p.config.ConstLabels,
		},
		errorLabels,
	)

	// Message size histograms
	messageLabels := []string{"method", "direction"}
	if !p.config.EnablePerMethodMetrics {
		messageLabels = []string{"direction"}
	}

	sizeBuckets := []float64{
		64, 256, 1024, 4096, 16384, 65536, 262144, 1048576, 4194304,
	} // 64B, 256B, 1KB, 4KB, 16KB, 64KB, 256KB, 1MB, 4MB

	p.messageSent = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   p.config.Namespace,
			Subsystem:   p.config.Subsystem,
			Name:        "message_sent_bytes",
			Help:        "Histogram of message sizes sent (bytes)",
			Buckets:     sizeBuckets,
			ConstLabels: p.config.ConstLabels,
		},
		messageLabels,
	)

	p.messageReceived = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   p.config.Namespace,
			Subsystem:   p.config.Subsystem,
			Name:        "message_received_bytes",
			Help:        "Histogram of message sizes received (bytes)",
			Buckets:     sizeBuckets,
			ConstLabels: p.config.ConstLabels,
		},
		messageLabels,
	)

	// Register all metrics
	p.registry.MustRegister(
		p.requestsTotal,
		p.activeRequests,
		p.errorsTotal,
		p.messageSent,
		p.messageReceived,
	)

	if p.config.EnableHistogram {
		p.registry.MustRegister(p.requestDuration)
	}

	return nil
}

// RecordRequest records a completed request
func (p *PrometheusCollector) RecordRequest(method string, code string, duration time.Duration) {
	if p.config.EnablePerMethodMetrics {
		p.requestsTotal.WithLabelValues(method, code).Inc()
		if p.config.EnableHistogram {
			p.requestDuration.WithLabelValues(method, code).Observe(duration.Seconds())
		}
	} else {
		p.requestsTotal.WithLabelValues(code).Inc()
		if p.config.EnableHistogram {
			p.requestDuration.WithLabelValues(code).Observe(duration.Seconds())
		}
	}
}

// RecordError records an error occurrence
func (p *PrometheusCollector) RecordError(method string, errorType string) {
	if p.config.EnablePerMethodMetrics {
		p.errorsTotal.WithLabelValues(method, errorType).Inc()
	} else {
		p.errorsTotal.WithLabelValues(errorType).Inc()
	}
}

// RecordActiveRequests updates the active requests gauge
func (p *PrometheusCollector) RecordActiveRequests(method string, delta int) {
	if p.config.EnablePerMethodMetrics {
		p.activeRequests.WithLabelValues(method).Add(float64(delta))
	} else {
		// Use empty label for global active requests
		p.activeRequests.WithLabelValues().Add(float64(delta))
	}
}

// RecordMessageSize records message sizes
func (p *PrometheusCollector) RecordMessageSize(method string, direction string, size int) {
	labels := []string{method, direction}
	if !p.config.EnablePerMethodMetrics {
		labels = []string{direction}
	}

	if direction == "sent" {
		p.messageSent.WithLabelValues(labels...).Observe(float64(size))
	} else {
		p.messageReceived.WithLabelValues(labels...).Observe(float64(size))
	}
}

// GetRegistry returns the Prometheus registry
func (p *PrometheusCollector) GetRegistry() *prometheus.Registry {
	return p.registry
}

// MustRegister registers a custom collector
func (p *PrometheusCollector) MustRegister(collectors ...prometheus.Collector) {
	p.registry.MustRegister(collectors...)
}

// Unregister unregisters a collector
func (p *PrometheusCollector) Unregister(collector prometheus.Collector) bool {
	return p.registry.Unregister(collector)
}
