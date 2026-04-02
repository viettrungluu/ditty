package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/viettrungluu/ditty/internal/session"
)

// newListCmd creates the `ditty list` subcommand (with `ls` alias).
func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List active sessions",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList()
		},
	}

	return cmd
}

// runList prints a table of sessions and their status.
func runList() error {
	sessions, err := session.List()
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		fmt.Fprintln(os.Stderr, "No active sessions.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tCOMMAND\tPID\tUPTIME")
	for _, s := range sessions {
		status := "alive"
		if !s.Alive {
			status = "stale"
		}
		command := "-"
		pid := "-"
		uptime := "-"
		if s.Meta != nil {
			command = s.Meta.Command
			if len(s.Meta.Args) > 0 {
				command += " " + strings.Join(s.Meta.Args, " ")
			}
			pid = fmt.Sprintf("%d", s.Meta.PID)
			uptime = formatDuration(time.Since(s.Meta.StartedAt))
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			s.Name, status, command, pid, uptime)
	}
	return w.Flush()
}

// formatDuration formats a duration in a human-readable compact form.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
