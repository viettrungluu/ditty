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
	var noShowPrompt bool

	cmd := &cobra.Command{
		Use:   "continue [flags] INPUT [INPUT...]",
		Short: "Send input to a running session",
		Long: `Sends the given input string to the named session's REPL and
streams output until the next prompt appears.

The detected prompt from the previous interaction is printed before
the output, so the display looks like a normal terminal session.
Use --no-show-prompt to suppress this.

With --multi, each argument is sent as a separate line, waiting for
the prompt between each one. Without --multi, all arguments are
joined with a space and sent as a single line.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if multi {
				return runContinueMulti(name, noShowPrompt, args)
			}
			return runContinueSingle(name, noShowPrompt,
				strings.Join(args, " "))
		},
	}

	cmd.Flags().StringVar(&name, "name", "",
		"session name (defaults to last-used session)")
	cmd.Flags().BoolVar(&multi, "multi", false,
		"send each argument as a separate line, waiting for the prompt between each")
	cmd.Flags().BoolVar(&noShowPrompt, "no-show-prompt", false,
		"don't print the prompt before output")

	return cmd
}

// runContinueSingle sends a single input and streams output.
func runContinueSingle(name string, noShowPrompt bool, input string) error {
	var err error
	name, err = resolveName(name)
	if err != nil {
		return err
	}

	conn, err := setupContinue(name)
	if err != nil {
		return err
	}
	defer conn.Close()
	defer resetTerminal()

	// Print the saved prompt from the previous interaction.
	if !noShowPrompt {
		if p := session.LoadPrompt(name); p != "" {
			os.Stdout.WriteString(p)
		}
	}

	return sendAndWait(conn, name, input)
}

// runContinueMulti sends each arg as a separate line, waiting for the
// prompt between each.
func runContinueMulti(name string, noShowPrompt bool, inputs []string) error {
	var err error
	name, err = resolveName(name)
	if err != nil {
		return err
	}

	conn, err := setupContinue(name)
	if err != nil {
		return err
	}
	defer conn.Close()
	defer resetTerminal()

	for _, input := range inputs {
		// Print the saved prompt before each input line.
		if !noShowPrompt {
			if p := session.LoadPrompt(name); p != "" {
				os.Stdout.WriteString(p)
			}
		}
		if err := sendAndWait(conn, name, input); err != nil {
			return err
		}
	}
	return nil
}

// setupContinue connects to the session and sets up SIGINT forwarding.
// The caller must close the returned connection.
func setupContinue(name string) (net.Conn, error) {
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
// prompt or exit. The trailing partial line (the prompt) is held back
// and saved to disk for the next continue.
func sendAndWait(conn net.Conn, name string, input string) error {
	if err := protocol.WriteMessage(conn, protocol.Message{
		Type:    protocol.MsgInput,
		Payload: []byte(input),
	}); err != nil {
		return fmt.Errorf("send input: %w", err)
	}

	var buf outputBuffer

	for {
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			buf.Flush()
			return fmt.Errorf("read from daemon: %w", err)
		}

		switch msg.Type {
		case protocol.MsgOutput, protocol.MsgBufferedOutput:
			buf.Write(msg.Payload)
		case protocol.MsgPromptDetected:
			// Save the trailing partial line (the prompt) for the
			// next continue to print.
			if p := buf.Partial(); p != "" {
				session.SavePrompt(name, p)
			}
			return nil
		case protocol.MsgExited:
			buf.Flush()
			code := 0
			if len(msg.Payload) > 0 {
				code = int(msg.Payload[0])
			}
			if code != 0 {
				return fmt.Errorf("REPL exited with code %d", code)
			}
			return nil
		case protocol.MsgError:
			buf.Flush()
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
