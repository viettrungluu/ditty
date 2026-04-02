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
	var multi bool

	cmd := &cobra.Command{
		Use:   "continue [flags] INPUT [INPUT...]",
		Short: "Send input to a running session",
		Long: `Sends the given input string to the named session's REPL and
streams output until the next prompt appears.

With --multi, each argument is sent as a separate line, waiting for
the prompt between each one. Without --multi, all arguments are
joined with a space and sent as a single line.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if multi {
				return runContinueMulti(name, args)
			}
			return runContinueSingle(name, strings.Join(args, " "))
		},
	}

	cmd.Flags().StringVar(&name, "name", "",
		"session name (defaults to last-used session)")
	cmd.Flags().BoolVar(&multi, "multi", false,
		"send each argument as a separate line, waiting for the prompt between each")

	return cmd
}

// runContinueSingle sends a single input and streams output.
func runContinueSingle(name string, input string) error {
	conn, err := setupContinue(name)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := sendAndWait(conn, input); err != nil {
		return err
	}
	return nil
}

// runContinueMulti sends each arg as a separate line, waiting for the
// prompt between each.
func runContinueMulti(name string, inputs []string) error {
	conn, err := setupContinue(name)
	if err != nil {
		return err
	}
	defer conn.Close()

	for _, input := range inputs {
		if err := sendAndWait(conn, input); err != nil {
			return err
		}
	}
	return nil
}

// setupContinue resolves the session, connects, and sets up SIGINT
// forwarding. The caller must close the returned connection.
func setupContinue(name string) (net.Conn, error) {
	var err error
	name, err = resolveName(name)
	if err != nil {
		return nil, err
	}

	conn, err := dialSession(name)
	if err != nil {
		return nil, err
	}

	// Record as last-used session.
	session.SetLast(name)

	// Forward SIGINT to the REPL as an interrupt message.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	go forwardInterrupts(conn, sigCh)

	return conn, nil
}

// sendAndWait sends one line of input and streams output until the next
// prompt or exit.
func sendAndWait(conn net.Conn, input string) error {
	if err := protocol.WriteMessage(conn, protocol.Message{
		Type:    protocol.MsgInput,
		Payload: []byte(input),
	}); err != nil {
		return fmt.Errorf("send input: %w", err)
	}

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
