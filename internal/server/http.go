package server

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"coder-policy/internal/config"
)

// newTransport constructs the shared upstream HTTP transport.
func newTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   128,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

// newReverseProxy constructs the upstream proxy and its stable JSON error response.
func newReverseProxy(upstreamURL *url.URL, transport http.RoundTripper, logger *slog.Logger) *httputil.ReverseProxy {
	reverseProxy := &httputil.ReverseProxy{
		Rewrite: func(proxyRequest *httputil.ProxyRequest) {
			proxyRequest.SetURL(upstreamURL)
			proxyRequest.Out.Host = proxyRequest.In.Host
			proxyRequest.SetXForwarded()
		},
		Transport:     transport,
		FlushInterval: -1,
	}
	reverseProxy.ErrorHandler = func(response http.ResponseWriter, request *http.Request, err error) {
		logger.Error("upstream proxy request failed", "method", request.Method, "host", request.Host, "path", request.URL.Path, "error", err)
		response.Header().Set("Content-Type", "application/json; charset=utf-8")
		response.Header().Set("Cache-Control", "no-store")
		response.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(response).Encode(map[string]string{"message": "Coder upstream is unavailable.", "code": "upstream_unavailable"})
	}
	return reverseProxy
}

// newHTTPServers constructs the proxy, health, and metrics HTTP server definitions.
func newHTTPServers(cfg config.Config, proxyHandler, healthHandler, metricsHandler http.Handler) []namedServer {
	return []namedServer{
		{
			name: "proxy",
			server: &http.Server{
				Addr:              cfg.ListenAddress,
				Handler:           proxyHandler,
				ReadHeaderTimeout: 10 * time.Second,
				IdleTimeout:       120 * time.Second,
			},
		},
		{
			name: "health",
			server: &http.Server{
				Addr:              cfg.HealthListenAddress,
				Handler:           healthHandler,
				ReadHeaderTimeout: 5 * time.Second,
				IdleTimeout:       30 * time.Second,
			},
		},
		{
			name: "metrics",
			server: &http.Server{
				Addr:              cfg.MetricsListenAddress,
				Handler:           metricsHandler,
				ReadHeaderTimeout: 5 * time.Second,
				IdleTimeout:       30 * time.Second,
			},
		},
	}
}

// healthHandler exposes liveness and version-gated readiness endpoints.
func healthHandler(gate readinessReader) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(response).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(response http.ResponseWriter, _ *http.Request) {
		ready, version, _ := gate.Ready()
		response.Header().Set("Content-Type", "application/json; charset=utf-8")
		if !ready {
			response.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(response).Encode(map[string]string{"status": "unready"})
			return
		}
		_ = json.NewEncoder(response).Encode(map[string]string{"status": "ready", "coder_version": version})
	})
	return mux
}

// metricsHandler exposes the registry only on the Prometheus /metrics path.
func metricsHandler(registry prometheus.Gatherer) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{EnableOpenMetrics: true}))
	return mux
}
