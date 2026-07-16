package proxy

import (
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics contains the bounded-cardinality Prometheus metrics emitted by the proxy handler.
type Metrics struct {
	requests   *prometheus.CounterVec
	durations  *prometheus.HistogramVec
	rejections *prometheus.CounterVec
}

// NewMetrics registers and returns the coder-rbac proxy metrics.
func NewMetrics(registerer prometheus.Registerer) *Metrics {
	metrics := &Metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder_rbac",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of requests handled by coder-rbac.",
		}, []string{"method", "host_kind", "outcome"}),
		durations: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coder_rbac",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "Duration of requests handled by coder-rbac.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "host_kind", "outcome"}),
		rejections: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder_rbac",
			Subsystem: "policy",
			Name:      "rejections_total",
			Help:      "Total number of requests rejected by coder-rbac, grouped by stable policy code.",
		}, []string{"code"}),
	}
	registerer.MustRegister(metrics.requests, metrics.durations, metrics.rejections)
	return metrics
}

// observeRequest records the outcome and duration of one proxy request.
func (m *Metrics) observeRequest(method, hostKind, outcome string, duration time.Duration) {
	method = normalizeMetricMethod(method)
	m.requests.WithLabelValues(method, hostKind, outcome).Inc()
	m.durations.WithLabelValues(method, hostKind, outcome).Observe(duration.Seconds())
}

// observeRejection increments the counter for a stable rejection code.
func (m *Metrics) observeRejection(code string) {
	m.rejections.WithLabelValues(code).Inc()
}

// normalizeMetricMethod bounds method label cardinality while retaining common HTTP methods.
func normalizeMetricMethod(method string) string {
	method = strings.ToUpper(method)
	switch method {
	case http.MethodConnect, http.MethodDelete, http.MethodGet, http.MethodHead,
		http.MethodOptions, http.MethodPatch, http.MethodPost, http.MethodPut, http.MethodTrace:
		return method
	default:
		return "OTHER"
	}
}
