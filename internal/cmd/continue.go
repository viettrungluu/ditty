package cmd

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/viettrungluu/ditty/internal/dlog"
	"github.com/viettrungluu/ditty/internal/protocol"
	"github.com/viettrungluu/ditty/internal/session"
)

// newContinueCmd creates the `ditty continue` subcommand.
func newContinueCmd() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "continue [flags] INPUT",
		Short: "Send input to a running session",
		Long: `Sends the given input string to the named session's REPL and
streams output until the next prompt appears.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runContinue(name, strings.Join(args, " "))
		},
	}

	cmd.Flags().StringVar(&name, "name", "",
		"session name (defaults to last-used session)")

	return cmd
}

// runContinue sends input to a session and streams output.
func runContinue(name string, input string) error {
	var err error
	name, err = resolveName(name)
	if err != nil {
		return err
	}

	conn, err := dialSession(name)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Record as last-used session.
	session.SetLast(name)

	// Forward SIGINT to the REPL as an interrupt message.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	defer signal.Stop(sigCh)
	go forwardInterrupts(conn, sigCh)

	// Send the input.
	if err := protocol.WriteMessage(conn, protocol.Message{
		Type:    protocol.MsgInput,
		Payload: []byte(input),
	}); err != nil {
		return fmt.Errorf("send input: %w", err)
	}

	// Stream output until prompt.
	for {
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			return fmt.Errorf("read from daemon: %w", err)
		}

		switch msg.Type {
		case protocol.MsgOutput, protocol.MsgBufferedOutput:
			os.Stdout.Write(msg.Payload)
		case protocol.MsgPromptDetected:
			return nil
		case protocol.MsgExited:
			code := 0
			if len(msg.Payload) > 0 {
				code = int(msg.Payload[0])
			}
			if code != 0 {
				return fmt.Errorf("REPL exited with code %d", code)
			}
			return nil
		case protocol.MsgError:
			return fmt.Errorf("daemon error: %s", msg.Payload)
		}
	}
}

// forwardInterrupts watches for SIGINT and sends interrupt messages to the
// daemon. It runs until the signal channel is closed or the connection fails.
func forwardInterrupts(conn net.Conn, sigCh <-chan os.Signal) {
	for range sigCh {
		dlog.Printf("continue: forwarding SIGINT as interrupt")
		err := protocol.WriteMessage(conn, protocol.Message{
			Type: protocol.MsgInterrupt,
		})
		if err != nil {
			return
		}
	}
}
