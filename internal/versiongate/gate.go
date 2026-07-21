// Package versiongate fails the proxy closed when the upstream Coder version is incompatible.
package versiongate

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"coder-policy/internal/coder"
)

// buildInfoClient is the Coder API capability required by the version gate.
type buildInfoClient interface {
	BuildInfo(context.Context) (coder.BuildInfo, error)
}

// Gate tracks whether the upstream Coder version matches the configured prefix.
type Gate struct {
	client   buildInfoClient
	expected string
	interval time.Duration
	logger   *slog.Logger

	mu      sync.RWMutex
	ready   bool
	version string
	lastErr error
}

// New creates a version gate that periodically checks the upstream Coder server.
func New(client buildInfoClient, expected string, interval time.Duration, logger *slog.Logger) *Gate {
	return &Gate{client: client, expected: expected, interval: interval, logger: logger}
}

// Check refreshes the version-gate state from the upstream build information.
func (g *Gate) Check(ctx context.Context) error {
	info, err := g.client.BuildInfo(ctx)
	if err == nil && !strings.HasPrefix(info.Version, g.expected) {
		err = fmt.Errorf("coder version %q does not match required prefix %q", info.Version, g.expected)
	}

	g.mu.Lock()
	g.ready = err == nil
	g.version = info.Version
	g.lastErr = err
	g.mu.Unlock()
	return err
}

// Run checks the upstream version periodically until the context is canceled.
func (g *Gate) Run(ctx context.Context) {
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkCtx, cancel := context.WithTimeout(ctx, min(g.interval, 30*time.Second))
			err := g.Check(checkCtx)
			cancel()
			if err != nil {
				g.logger.Error("Coder version gate became unready", "error", err)
			}
		}
	}
}

// Ready returns a synchronized snapshot of readiness, version, and the last error.
func (g *Gate) Ready() (bool, string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.ready, g.version, g.lastErr
}
