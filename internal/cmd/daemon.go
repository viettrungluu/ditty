package cmd

import (
	"fmt"
	"regexp"
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
	var echo bool
	var bufSize int
	var promptPattern string
	var noPty bool
	var suspend bool
	var envVars []string

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
				Echo:        echo,
				BufSize:     bufSize,
				NoPty:       noPty,
				Suspend:     suspend,
				Env:         envVars,
			}
			if promptPattern != "" {
				re, err := regexp.Compile(promptPattern)
				if err != nil {
					return fmt.Errorf("invalid --prompt regex: %w", err)
				}
				cfg.PromptRegex = re
			}
			return daemon.Run(cfg)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "session name")
	cmd.Flags().DurationVar(&idleTimeout, "idle-timeout",
		prompt.DefaultIdleTimeout, "prompt detection idle timeout")
	cmd.Flags().BoolVar(&echo, "echo", true, "echo input back in output")
	cmd.Flags().IntVar(&bufSize, "buffer-size", 0,
		"ring buffer size in bytes (0 means default)")
	cmd.Flags().StringVar(&promptPattern, "prompt", "",
		"regex pattern for prompt detection")
	cmd.Flags().BoolVar(&noPty, "no-pty", false,
		"use pipes instead of a pty")
	cmd.Flags().BoolVar(&suspend, "suspend", false,
		"SIGSTOP the child between commands")
	cmd.Flags().StringArrayVar(&envVars, "env", nil,
		"set environment variable (KEY=VALUE, repeatable)")

	cmd.Flags().SetInterspersed(false)

	return cmd
}
