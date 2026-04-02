package cmd

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/viettrungluu/ditty/internal/dlog"
	"github.com/viettrungluu/ditty/internal/protocol"
	"github.com/viettrungluu/ditty/internal/session"
)

// newStartCmd creates the `ditty start` subcommand.
func newStartCmd() *cobra.Command {
	var name string
	var idleTimeout time.Duration
	var echo bool
	var bufSize int
	var promptPattern string

	cmd := &cobra.Command{
		Use:   "start [flags] PROGRAM [ARGS...]",
		Short: "Start a new REPL session",
		Long: `Launches the given interactive program in the background and streams
its initial output until the first prompt appears.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStart(name, idleTimeout, echo, bufSize, promptPattern, args)
		},
	}

	cmd.Flags().StringVar(&name, "name", "",
		"session name (generated if omitted)")
	cmd.Flags().DurationVar(&idleTimeout, "idle-timeout", 0,
		"prompt detection idle timeout (e.g., 200ms, 1s); 0 means default")
	cmd.Flags().BoolVar(&echo, "echo", false,
		"echo input back in output (default: no echo)")
	cmd.Flags().IntVar(&bufSize, "buffer-size", 0,
		"background output ring buffer size in bytes (default: 1MB)")
	cmd.Flags().StringVar(&promptPattern, "prompt", "",
		"regex pattern for prompt detection (overrides idle timeout)")

	return cmd
}

// runStart launches the daemon and streams initial output.
func runStart(name string, idleTimeout time.Duration, echo bool, bufSize int, promptPattern string, args []string) error {
	// Generate a name if not provided.
	if name == "" {
		var err error
		name, err = session.GenerateName()
		if err != nil {
			return err
		}
	}

	// Ensure sessions directory exists.
	if _, err := session.EnsureBaseDir(); err != nil {
		return err
	}

	// Check that no session with this name already exists.
	if session.IsAlive(name) {
		return fmt.Errorf("session %q already exists", name)
	}

	// Find the ditty binary path for re-exec.
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}

	// Build the _daemon command.
	daemonArgs := []string{}
	if dlog.Verbose() {
		daemonArgs = append(daemonArgs, "--verbose")
	}
	daemonArgs = append(daemonArgs, "_daemon", "--name", name)
	if idleTimeout > 0 {
		daemonArgs = append(daemonArgs, "--idle-timeout",
			idleTimeout.String())
	}
	if echo {
		daemonArgs = append(daemonArgs, "--echo")
	}
	if bufSize > 0 {
		daemonArgs = append(daemonArgs, "--buffer-size",
			fmt.Sprintf("%d", bufSize))
	}
	if promptPattern != "" {
		daemonArgs = append(daemonArgs, "--prompt", promptPattern)
	}
	daemonArgs = append(daemonArgs, "--")
	daemonArgs = append(daemonArgs, args...)

	daemonCmd := exec.Command(self, daemonArgs...)
	// Detach stdin/stdout so the daemon runs independently.
	daemonCmd.Stdin = nil
	daemonCmd.Stdout = nil
	// When verbose, inherit stderr so daemon logs are visible to the
	// user. Otherwise detach completely.
	if dlog.Verbose() {
		daemonCmd.Stderr = os.Stderr
	} else {
		daemonCmd.Stderr = nil
	}
	// Start in a new process group so the daemon survives the parent.
	daemonCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := daemonCmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	// Release the child process so we don't become a zombie parent.
	daemonCmd.Process.Release()

	// Wait for the daemon to start accepting connections.
	conn, err := waitForSocket(name, 5*time.Second)
	if err != nil {
		return fmt.Errorf("waiting for daemon: %w", err)
	}
	defer conn.Close()

	// Record as last-used session.
	session.SetLast(name)

	fmt.Fprintf(os.Stderr, "ditty: session %q started\n", name)

	// Stream initial output until prompt is detected.
	return streamUntilPrompt(conn)
}

// waitForSocket polls for the session's Unix socket to become available.
func waitForSocket(name string, timeout time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(timeout)
	for {
		conn, err := session.DialSocket(name)
		if err == nil {
			return conn, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout waiting for session %q", name)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// streamUntilPrompt reads protocol messages from conn and writes output to
// stdout until a PromptDetected or Exited message is received.
func streamUntilPrompt(conn net.Conn) error {
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
			return nil
		case protocol.MsgError:
			return fmt.Errorf("daemon error: %s", msg.Payload)
		}
	}
}
