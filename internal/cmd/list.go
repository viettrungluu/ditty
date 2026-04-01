package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

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
	fmt.Fprintln(w, "NAME\tSTATUS")
	for _, s := range sessions {
		status := "alive"
		if !s.Alive {
			status = "stale"
		}
		fmt.Fprintf(w, "%s\t%s\n", s.Name, status)
	}
	return w.Flush()
}
