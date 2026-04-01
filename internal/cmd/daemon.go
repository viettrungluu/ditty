package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/viettrungluu/ditty/internal/daemon"
	"github.com/viettrungluu/ditty/internal/prompt"
)

// newDaemonCmd creates the hidden _daemon subcommand. This is not intended
// to be called by users directly — it is exec'd by `ditty start`.
func newDaemonCmd() *cobra.Command {
	var name string
	var idleTimeout time.Duration

	cmd := &cobra.Command{
		Use:    "_daemon",
		Hidden: true,
		Args:   cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			cfg := daemon.Config{
				Name:        name,
				Command:     args[0],
				Args:        args[1:],
				IdleTimeout: idleTimeout,
			}
			return daemon.Run(cfg)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "session name")
	cmd.Flags().DurationVar(&idleTimeout, "idle-timeout",
		prompt.DefaultIdleTimeout, "prompt detection idle timeout")

	return cmd
}
