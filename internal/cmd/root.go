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
		Long: `ditty ("de-TTY") runs interactive programs (REPLs, debuggers, etc.) in the
background and lets you send input and receive output through simple CLI commands.

Examples:
  ditty start --name=py python3
  ditty continue --name=py 'print("hello")'
  ditty continue --name=py 'x = 42'
  ditty continue --name=py 'print(x * 2)'
  ditty list
  ditty stop --name=py

  ditty start --name=db gdb ./a.out
  ditty continue --name=db 'break main'
  ditty continue --name=db 'run'

Full documentation: https://github.com/viettrungluu/ditty/blob/main/README.md`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			dlog.SetVerbose(verbose)
		},
	}

	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false,
		"enable verbose/debug logging")
	cmd.PersistentFlags().BoolVar(&noTerminalReset, "no-terminal-reset", false,
		"don't reset terminal state after streaming output")

	cmd.AddCommand(newDaemonCmd())
	cmd.AddCommand(newStartCmd())
	cmd.AddCommand(newContinueCmd())
	cmd.AddCommand(newStopCmd())
	cmd.AddCommand(newKillCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newAttachCmd())
	cmd.AddCommand(newListPresetsCmd())

	return cmd
}
