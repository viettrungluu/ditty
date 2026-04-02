package cmd

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/viettrungluu/ditty/internal/dlog"
	"github.com/viettrungluu/ditty/internal/protocol"
	"github.com/viettrungluu/ditty/internal/session"
)

// newAttachCmd creates the `ditty attach` subcommand.
func newAttachCmd() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "attach [flags]",
		Short: "Attach to a session interactively",
		Long: `Connects to a running session and provides an interactive terminal.
Input is read line-by-line from stdin and sent to the REPL. Output is
streamed to stdout. Detach with Ctrl-D (EOF) or when the REPL exits.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAttach(name)
		},
	}

	cmd.Flags().StringVar(&name, "name", "",
		"session name (defaults to last-used session)")

	return cmd
}

// runAttach connects to a session and relays stdin/stdout interactively.
func runAttach(name string) error {
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

	session.SetLast(name)

	// Forward SIGINT to the REPL.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	defer signal.Stop(sigCh)
	go forwardInterrupts(conn, sigCh)

	fmt.Fprintf(os.Stderr, "ditty: attached to %q (Ctrl-D to detach)\n", name)

	// Read output from daemon in background.
	done := make(chan error, 1)
	go func() {
		done <- attachReadLoop(conn)
	}()

	// Read lines from stdin and send as input. When stdin reaches EOF,
	// we stop sending but keep the connection open so remaining output
	// can be read. We wait a short time for final output, then close.
	stdinDone := make(chan struct{})
	go func() {
		attachStdinLoop(conn)
		close(stdinDone)
	}()

	// Wait for either the read loop to finish (REPL exit) or stdin EOF.
	select {
	case err = <-done:
		// Read loop ended (REPL exited or error).
	case <-stdinDone:
		// Stdin EOF — wait briefly for any pending output, then detach.
		select {
		case err = <-done:
		case <-time.After(500 * time.Millisecond):
			conn.Close()
			err = <-done
		}
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "\nditty: detached from %q\n", name)
	return nil
}

// attachReadLoop reads protocol messages from the daemon and writes output
// to stdout. It returns when the connection closes or the REPL exits.
func attachReadLoop(conn net.Conn) error {
	for {
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			// Connection closed — daemon exited or we detached.
			return nil
		}

		switch msg.Type {
		case protocol.MsgOutput, protocol.MsgBufferedOutput:
			os.Stdout.Write(msg.Payload)
		case protocol.MsgPromptDetected:
			// In attach mode, ignore prompt detection — stay connected.
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

// attachStdinLoop reads lines from stdin and sends them to the daemon.
// It returns on EOF (Ctrl-D) or read error.
func attachStdinLoop(conn net.Conn) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		err := protocol.WriteMessage(conn, protocol.Message{
			Type:    protocol.MsgInput,
			Payload: []byte(line),
		})
		if err != nil {
			dlog.Printf("attach: write error: %v", err)
			return
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		dlog.Printf("attach: stdin read error: %v", err)
	}
}
