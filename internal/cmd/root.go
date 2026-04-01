// Package cmd implements the ditty CLI commands.
package cmd

import (
	"github.com/spf13/cobra"
)

// NewRootCmd creates the root cobra command for ditty.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ditty",
		Short: "Convert line-interactive programs into command-line programs",
		Long: `ditty runs interactive programs (REPLs, debuggers, etc.) in the background
and lets you send input and receive output through simple CLI commands.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	return cmd
}
