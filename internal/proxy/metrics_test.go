package proxy

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestMetricsRecordBoundedRequestAndRejectionLabels verifies custom methods are normalized.
func TestMetricsRecordBoundedRequestAndRejectionLabels(t *testing.T) {
	metrics := NewMetrics(prometheus.NewRegistry())
	metrics.observeRequest("CUSTOM", "main", "forwarded", 10*time.Millisecond)
	metrics.observeRejection("workspace_rename_forbidden")

	if got := testutil.ToFloat64(metrics.requests.WithLabelValues("OTHER", "main", "forwarded")); got != 1 {
		t.Fatalf("request counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.rejections.WithLabelValues("workspace_rename_forbidden")); got != 1 {
		t.Fatalf("rejection counter = %v, want 1", got)
	}
}
