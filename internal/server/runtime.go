package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"coder-policy/internal/coder"
	"coder-policy/internal/config"
	proxyhandler "coder-policy/internal/proxy"
	"coder-policy/internal/versiongate"
)

// readinessReader is the version-gate state required by health and metrics handlers.
type readinessReader interface {
	Ready() (bool, string, error)
}

// versionGate is the lifecycle capability required by the server runtime.
type versionGate interface {
	readinessReader
	Check(context.Context) error
	Run(context.Context)
}

// runtime contains the constructed services and their lifecycle dependencies.
type runtime struct {
	gate                  versionGate
	transport             *http.Transport
	servers               []namedServer
	upstreamTimeout       time.Duration
	shutdownTimeout       time.Duration
	expectedVersionPrefix string
	logger                *slog.Logger
}

// newRuntime constructs the shared transport, policy handler, and three HTTP services.
func newRuntime(cfg config.Config, logger *slog.Logger) *runtime {
	transport := newTransport()
	policyHTTPClient := &http.Client{Transport: transport, Timeout: cfg.UpstreamTimeout}
	coderClient := coder.NewClient(cfg.UpstreamURL, policyHTTPClient)
	gate := versiongate.New(coderClient, cfg.ExpectedVersionPrefix, cfg.VersionCheckInterval, logger)
	reverseProxy := newReverseProxy(cfg.UpstreamURL, transport, logger)
	classifier := proxyhandler.NewHostClassifier(cfg.PublicURL, cfg.WildcardAccessHost, cfg.InternalHosts)
	policy := proxyhandler.NewPolicy(coderClient, cfg.AdminUsernames, cfg.AllowedUsernames, cfg.MaxInspectionBodyBytes)
	registry, proxyMetrics := newMetricsRegistry(gate)
	handler := proxyhandler.NewHandler(classifier, policy, gate, reverseProxy, logger, proxyMetrics)

	return &runtime{
		gate:                  gate,
		transport:             transport,
		servers:               newHTTPServers(cfg, handler, healthHandler(gate), metricsHandler(registry)),
		upstreamTimeout:       cfg.UpstreamTimeout,
		shutdownTimeout:       cfg.ShutdownTimeout,
		expectedVersionPrefix: cfg.ExpectedVersionPrefix,
		logger:                logger,
	}
}
