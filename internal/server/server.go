// Package server assembles and runs the proxy, health, and metrics HTTP servers.
package server

import (
	"context"
	"log/slog"

	"coder-policy/internal/config"
)

// Run validates the upstream version, starts every HTTP server, and shuts them down gracefully.
func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	return newRuntime(cfg, logger).run(ctx)
}
