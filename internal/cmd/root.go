// Package cmd implements the ditty CLI commands.
package cmd

import (
	"github.com/spf13/cobra"
	"github.com/viettrungluu/ditty/internal/dlog"
)

// NewRootCmd creates the root cobra command for ditty.
func NewRootCmd() *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "ditty",
		Short: "Convert line-interactive programs into command-line programs",
		Long: `ditty runs interactive programs (REPLs, debuggers, etc.) in the background
and lets you send input and receive output through simple CLI commands.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			dlog.SetVerbose(verbose)
		},
	}

	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false,
		"enable verbose/debug logging")

	cmd.AddCommand(newDaemonCmd())
	cmd.AddCommand(newStartCmd())
	cmd.AddCommand(newContinueCmd())
	cmd.AddCommand(newStopCmd())
	cmd.AddCommand(newKillCmd())
	cmd.AddCommand(newListCmd())

	return cmd
}
