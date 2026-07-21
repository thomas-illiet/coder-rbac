package server

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"coder-policy/internal/buildinfo"
	proxyhandler "coder-policy/internal/proxy"
)

// newMetricsRegistry registers process, Go runtime, build, readiness, and proxy metrics.
func newMetricsRegistry(gate readinessReader) (*prometheus.Registry, *proxyhandler.Metrics) {
	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	buildMetric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coder_policy",
		Name:      "build_info",
		Help:      "Build information for the running coder-policy binary.",
	}, []string{"version", "commit"})
	buildMetric.WithLabelValues(buildinfo.Version, buildinfo.Commit).Set(1)
	registry.MustRegister(buildMetric)

	registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "coder_policy",
		Name:      "coder_version_ready",
		Help:      "Whether the upstream Coder version currently satisfies the configured version policy.",
	}, func() float64 {
		ready, _, _ := gate.Ready()
		if ready {
			return 1
		}
		return 0
	}))

	return registry, proxyhandler.NewMetrics(registry)
}
