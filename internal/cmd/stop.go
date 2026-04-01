package cmd

import (
	"fmt"
	"net"
	"os"

	"github.com/spf13/cobra"
	"github.com/viettrungluu/ditty/internal/protocol"
	"github.com/viettrungluu/ditty/internal/session"
)

// newStopCmd creates the `ditty stop` subcommand.
func newStopCmd() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "stop [flags]",
		Short: "Gracefully stop a session",
		Long:  `Sends EOF to the REPL and waits for it to exit.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStop(name)
		},
	}

	cmd.Flags().StringVar(&name, "name", "",
		"session name (defaults to last-used session)")

	return cmd
}

// runStop gracefully terminates a session.
func runStop(name string) error {
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
		Type: protocol.MsgStop,
	}); err != nil {
		return fmt.Errorf("send stop: %w", err)
	}

	// Wait for the Exited message.
	for {
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			// Connection closed — daemon exited.
			break
		}
		switch msg.Type {
		case protocol.MsgOutput, protocol.MsgBufferedOutput:
			os.Stdout.Write(msg.Payload)
		case protocol.MsgExited:
			fmt.Fprintf(os.Stderr, "ditty: session %q stopped\n", name)
			return nil
		case protocol.MsgError:
			return fmt.Errorf("daemon error: %s", msg.Payload)
		}
	}

	fmt.Fprintf(os.Stderr, "ditty: session %q stopped\n", name)
	return nil
}

// resolveName resolves a session name, falling back to the last-used session.
func resolveName(name string) (string, error) {
	if name != "" {
		return name, nil
	}
	last, err := session.GetLast()
	if err != nil {
		return "", err
	}
	if last == "" {
		return "", fmt.Errorf("no session name given and no last-used session")
	}
	return last, nil
}

// dialSession connects to a session's Unix domain socket.
// It checks for the socket file first and gives a clear error if the
// session doesn't exist.
func dialSession(name string) (net.Conn, error) {
	sockPath, err := session.SocketPath(name)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(sockPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("session %q not found", name)
	}
	conn, err := session.DialSocket(name)
	if err != nil {
		return nil, fmt.Errorf("session %q exists but is not responding "+
			"(stale socket?): %w", name, err)
	}
	return conn, nil
}
