// Package command defines the Cobra command tree for coder-rbac.
package command

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/thomas-illiet/coder-rbac/internal/buildinfo"
	"github.com/thomas-illiet/coder-rbac/internal/config"
)

// Execute runs the root command with the process standard output streams.
func Execute() error {
	return newRootCommand(os.Stdout, os.Stderr).Execute()
}

// newRootCommand builds the complete coder-rbac command tree.
func newRootCommand(stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "coder-rbac",
		Short:         "Policy-enforcing reverse proxy for Coder",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.AddCommand(newServeCommand(), newVersionCommand(), newCompletionCommand(root))
	return root
}

// newVersionCommand builds the command that prints build metadata.
func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "coder-rbac %s (commit %s, built %s)\n", buildinfo.Version, buildinfo.Commit, buildinfo.Date)
			return err
		},
	}
}

// newCompletionCommand builds the command that generates shell completion scripts.
func newCompletionCommand(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:       "completion [bash|zsh|fish|powershell]",
		Short:     "Generate shell completion",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				return root.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return root.GenFishCompletion(cmd.OutOrStdout(), true)
			case "powershell":
				return root.GenPowerShellCompletion(cmd.OutOrStdout())
			default:
				return fmt.Errorf("unsupported shell %q", args[0])
			}
		},
	}
}

// addServeFlags registers every proxy configuration flag on the serve command.
func addServeFlags(flags *pflag.FlagSet) {
	config.RegisterFlags(flags)
}
