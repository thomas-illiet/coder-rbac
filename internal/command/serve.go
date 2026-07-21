package command

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"coder-policy/internal/config"
	"coder-policy/internal/server"
)

// newServeCommand builds the command that validates configuration and runs the proxy.
func newServeCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "serve",
		Short: "Run the Coder policy proxy",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			v, err := config.NewViper(cmd.Flags())
			if err != nil {
				return err
			}
			cfg, err := config.Load(v)
			if err != nil {
				return err
			}
			logger := slog.New(slog.NewJSONHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: slog.LevelInfo}))
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			return server.Run(ctx, cfg, logger)
		},
	}
	addServeFlags(command.Flags())
	return command
}
