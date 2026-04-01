package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/viettrungluu/ditty/internal/protocol"
)

// newKillCmd creates the `ditty kill` subcommand.
func newKillCmd() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "kill [flags]",
		Short: "Forcibly kill a session",
		Long:  `Sends SIGTERM to the REPL, then SIGKILL after a timeout.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKill(name)
		},
	}

	cmd.Flags().StringVar(&name, "name", "",
		"session name (defaults to last-used session)")

	return cmd
}

// runKill forcibly terminates a session.
func runKill(name string) error {
	name, err := resolveName(name)
	if err != nil {
		return err
	}

	conn, err := dialSession(name)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := protocol.WriteMessage(conn, protocol.Message{
		Type: protocol.MsgKill,
	}); err != nil {
		return fmt.Errorf("send kill: %w", err)
	}

	// Wait for the Exited message.
	for {
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			break
		}
		switch msg.Type {
		case protocol.MsgOutput, protocol.MsgBufferedOutput:
			os.Stdout.Write(msg.Payload)
		case protocol.MsgExited:
			fmt.Fprintf(os.Stderr, "ditty: session %q killed\n", name)
			return nil
		case protocol.MsgError:
			return fmt.Errorf("daemon error: %s", msg.Payload)
		}
	}

	fmt.Fprintf(os.Stderr, "ditty: session %q killed\n", name)
	return nil
}
