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
	"github.com/viettrungluu/ditty/internal/preset"
	"github.com/viettrungluu/ditty/internal/protocol"
	"github.com/viettrungluu/ditty/internal/session"
)

// newStartCmd creates the `ditty start` subcommand.
func newStartCmd() *cobra.Command {
	var name string
	var idleTimeout time.Duration
	var noEcho bool
	var bufSize int
	var promptPattern string
	var noPty bool
	var suspend bool
	var noPresets bool
	var noBuiltinPresets bool
	var presetsFile string
	var presetName string
	var envVars []string

	cmd := &cobra.Command{
		Use:   "start [flags] PROGRAM [ARGS...]",
		Short: "Start a new REPL session",
		Long: `Launches the given interactive program in the background and streams
its initial output until the first prompt appears.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Look up preset and apply defaults for unset flags.
			if !noPresets {
				if presetsFile == "" {
					if p, err := preset.DefaultPresetsFile(); err == nil {
						presetsFile = p
					}
				}

				commandLine := preset.BuildCommandLine(args)
				flags, matched, err := preset.Lookup(
					commandLine, presetName, presetsFile,
					!noBuiltinPresets)
				if err != nil {
					return fmt.Errorf("preset lookup: %w", err)
				}
				if flags != "" {
					dlog.Printf("start: preset %q matched, flags: %s",
						matched, flags)
					parsed := preset.ParseFlags(flags)
					applyPresetDefaults(cmd, parsed,
						&promptPattern, &idleTimeout, &noEcho,
						&noPty, &suspend, &bufSize, &envVars)
				}
			}

			return runStart(name, idleTimeout, noEcho, bufSize,
				promptPattern, noPty, suspend, envVars, args)
		},
	}

	cmd.Flags().StringVar(&name, "name", "",
		"session name (generated if omitted)")
	cmd.Flags().DurationVar(&idleTimeout, "idle-timeout", 0,
		"prompt detection idle timeout (e.g., 200ms, 1s); 0 means default")
	cmd.Flags().BoolVar(&noEcho, "no-echo", false,
		"suppress input echo in output (cleaner for scripting)")
	cmd.Flags().IntVar(&bufSize, "buffer-size", 0,
		"background output ring buffer size in bytes (default: 1MB)")
	cmd.Flags().StringVar(&promptPattern, "prompt", "",
		"regex pattern for prompt detection (overrides presets)")
	cmd.Flags().BoolVar(&noPty, "no-pty", false,
		"use pipes instead of a pty (for programs that don't need a terminal)")
	cmd.Flags().BoolVar(&suspend, "suspend", false,
		"SIGSTOP the child between commands (some programs handle this poorly)")
	cmd.Flags().BoolVar(&noPresets, "no-presets", false,
		"disable all presets (built-in and user)")
	cmd.Flags().BoolVar(&noBuiltinPresets, "no-builtin-presets", false,
		"disable built-in presets (user presets file still applies)")
	cmd.Flags().StringVar(&presetsFile, "presets-file", "",
		"path to presets file (default: ~/.ditty/presets)")
	cmd.Flags().StringVar(&presetName, "preset", "",
		"use a named preset (e.g., python, rails, gdb)")
	cmd.Flags().StringArrayVar(&envVars, "env", nil,
		"set environment variable for the child (KEY=VALUE, repeatable)")

	// Stop parsing flags after the first positional arg (the program name),
	// so that flags intended for the program (e.g., python3 -i) aren't
	// consumed by ditty.
	cmd.Flags().SetInterspersed(false)

	return cmd
}

// applyPresetDefaults applies preset flags as defaults for any CLI flags
// that the user did not explicitly set.
func applyPresetDefaults(cmd *cobra.Command, parsed map[string]string,
	promptPattern *string, idleTimeout *time.Duration, noEcho *bool,
	noPty *bool, suspend *bool, bufSize *int, envVars *[]string) {

	if v, ok := parsed["prompt"]; ok && !cmd.Flags().Changed("prompt") {
		*promptPattern = v
	}
	if v, ok := parsed["idle-timeout"]; ok && !cmd.Flags().Changed("idle-timeout") {
		if d, err := time.ParseDuration(v); err == nil {
			*idleTimeout = d
		}
	}
	if _, ok := parsed["no-echo"]; ok && !cmd.Flags().Changed("no-echo") {
		*noEcho = true
	}
	if _, ok := parsed["no-pty"]; ok && !cmd.Flags().Changed("no-pty") {
		*noPty = true
	}
	if _, ok := parsed["suspend"]; ok && !cmd.Flags().Changed("suspend") {
		*suspend = true
	}
	if v, ok := parsed["buffer-size"]; ok && !cmd.Flags().Changed("buffer-size") {
		if n, err := fmt.Sscanf(v, "%d", bufSize); n == 1 && err == nil {
			// applied
		}
	}
	// Env vars from presets are always added (they don't replace CLI --env).
	presetEnv := preset.ParseEnvFlags(parsed)
	if len(presetEnv) > 0 {
		*envVars = append(presetEnv, *envVars...)
	}
}

// runStart launches the daemon and streams initial output.
func runStart(name string, idleTimeout time.Duration, noEcho bool, bufSize int, promptPattern string, noPty bool, suspend bool, envVars []string, args []string) error {
	// Generate a name if not provided.
	if name == "" {
		command := ""
		if len(args) > 0 {
			command = args[0]
		}
		var err error
		name, err = session.GenerateName(command)
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
	if noEcho {
		daemonArgs = append(daemonArgs, "--no-echo")
	}
	if bufSize > 0 {
		daemonArgs = append(daemonArgs, "--buffer-size",
			fmt.Sprintf("%d", bufSize))
	}
	if promptPattern != "" {
		daemonArgs = append(daemonArgs, "--prompt", promptPattern)
	}
	if noPty {
		daemonArgs = append(daemonArgs, "--no-pty")
	}
	if suspend {
		daemonArgs = append(daemonArgs, "--suspend")
	}
	for _, e := range envVars {
		daemonArgs = append(daemonArgs, "--env", e)
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
	err = streamUntilPrompt(conn)

	// Reset terminal state that the REPL's startup may have changed.
	resetTerminal()

	return err
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
			os.Stdout.WriteString("\n")
			return nil
		case protocol.MsgExited:
			return nil
		case protocol.MsgError:
			return fmt.Errorf("daemon error: %s", msg.Payload)
		}
	}
}
