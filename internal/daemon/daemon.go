// Package daemon implements the per-session ditty daemon.
//
// Each daemon allocates a pty, starts a REPL child process, and serves
// clients over a Unix domain socket. The daemon continuously reads from
// the pty; output is either streamed to a connected client or buffered in
// a ring buffer.
package daemon

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty/v2"
	"github.com/viettrungluu/ditty/internal/dlog"
	"github.com/viettrungluu/ditty/internal/prompt"
	"github.com/viettrungluu/ditty/internal/ringbuf"
	"github.com/viettrungluu/ditty/internal/session"
)

// Config holds configuration for a daemon instance.
type Config struct {
	// Name is the session name.
	Name string
	// Command is the REPL program to run.
	Command string
	// Args are the arguments to the REPL program.
	Args []string
	// IdleTimeout is the prompt detection idle timeout.
	IdleTimeout time.Duration
	// BufSize is the ring buffer capacity in bytes.
	BufSize int
	// Echo controls whether the pty echoes input back. When false (the
	// default), input sent via continue is not echoed in the output.
	Echo bool
	// PromptRegex, if set, is a compiled regex for prompt detection.
	PromptRegex *regexp.Regexp
}

// Daemon manages a single REPL session.
type Daemon struct {
	cfg    Config
	ptmx   *os.File // pty master
	cmd    *exec.Cmd
	buf    *ringbuf.RingBuf
	server *Server

	// mu protects client, detector, and echoStrip.
	mu        sync.Mutex
	client    *clientConn // currently connected client, or nil
	detector  *prompt.Detector
	echoStrip string // when non-empty, strip this from the next output chunk

	done chan struct{} // closed when the daemon should exit
}

// Run starts the daemon: launches the REPL, starts the socket server, and
// blocks until the REPL exits or a fatal error occurs. It cleans up the
// session on return.
func Run(cfg Config) error {
	if cfg.BufSize <= 0 {
		cfg.BufSize = ringbuf.DefaultCapacity
	}

	d := &Daemon{
		cfg:  cfg,
		buf:  ringbuf.New(cfg.BufSize),
		done: make(chan struct{}),
	}

	dlog.Printf("daemon: starting session %q, command=%s args=%v",
		cfg.Name, cfg.Command, cfg.Args)

	// Start the REPL on a pty.
	if err := d.startREPL(); err != nil {
		return fmt.Errorf("start REPL: %w", err)
	}

	// Start the Unix socket server.
	var err error
	d.server, err = NewServer(cfg.Name, d)
	if err != nil {
		d.ptmx.Close()
		d.cmd.Process.Kill()
		return fmt.Errorf("start server: %w", err)
	}

	// Write session metadata to disk.
	if err := session.WriteMetadata(cfg.Name, session.Metadata{
		PID:       os.Getpid(),
		Command:   cfg.Command,
		Args:      cfg.Args,
		StartedAt: time.Now(),
	}); err != nil {
		dlog.Printf("daemon: failed to write metadata: %v", err)
	}

	// Read pty output in the background.
	go d.readLoop()

	// Wait for child to exit.
	go d.waitChild()

	// Ignore SIGINT/SIGTERM — the daemon stays alive as long as the REPL
	// is alive. Signals are forwarded to the REPL by the client.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for range sigCh {
			// Ignore.
		}
	}()

	<-d.done

	d.server.Close()
	session.Cleanup(cfg.Name)
	return nil
}

// startREPL launches the child process on a pty.
func (d *Daemon) startREPL() error {
	cmd := exec.Command(d.cfg.Command, d.cfg.Args...)
	// Inherit the user's environment, including TERM. Only set TERM if
	// it's not already present (e.g., when running detached).
	cmd.Env = os.Environ()
	if os.Getenv("TERM") == "" {
		cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("pty.Start: %w", err)
	}

	d.cmd = cmd
	d.ptmx = ptmx
	return nil
}

// readLoop continuously reads from the pty master and dispatches output.
func (d *Daemon) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := d.ptmx.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			d.handleOutput(data)
		}
		if err != nil {
			if err != io.EOF {
				dlog.Printf("pty read error: %v", err)
			}
			return
		}
	}
}

// handleOutput sends data to the connected client or buffers it.
func (d *Daemon) handleOutput(data []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()

	dlog.Printf("daemon: output %d bytes, client=%v, detector=%v",
		len(data), d.client != nil, d.detector != nil)

	if d.client != nil {
		// Strip echoed input if configured.
		data = d.stripEcho(data)

		// Stream to connected client (if any data remains).
		if len(data) > 0 {
			if err := d.client.sendOutput(data); err != nil {
				dlog.Printf("daemon: client send error: %v", err)
			}
		}
		// Feed the prompt detector.
		if d.detector != nil && len(data) > 0 {
			d.detector.Feed(data)
		}
	} else {
		// No client connected — buffer the output.
		d.buf.Write(data)
	}
}

// waitChild waits for the REPL process to exit and signals the daemon to
// shut down.
func (d *Daemon) waitChild() {
	err := d.cmd.Wait()

	d.mu.Lock()
	if d.client != nil {
		exitCode := exitCodeFrom(err)
		d.client.sendExited(exitCode)
	}
	d.mu.Unlock()

	d.ptmx.Close()
	close(d.done)
}

// HandleInput processes an Input message from a client: writes the input
// to the pty and begins prompt detection on the output.
func (d *Daemon) HandleInput(c *clientConn, data []byte) {
	d.mu.Lock()

	// Reset the prompt detector for new input.
	if d.detector != nil {
		d.detector.Stop()
	}
	d.detector = prompt.NewDetector(d.promptConfig(), func() {
		d.mu.Lock()
		if d.client == c {
			c.sendPromptDetected()
		}
		d.mu.Unlock()
	})

	d.mu.Unlock()

	// When echo suppression is active, set up a filter to strip the
	// echoed input from the output.
	if !d.cfg.Echo {
		plainInput := string(data)
		if len(plainInput) > 0 && plainInput[len(plainInput)-1] == '\n' {
			plainInput = plainInput[:len(plainInput)-1]
		}
		d.echoStrip = plainInput
	}

	// Write the input to the pty (with a trailing newline if not present).
	input := data
	if len(input) == 0 || input[len(input)-1] != '\n' {
		input = append(input, '\n')
	}
	if _, err := d.ptmx.Write(input); err != nil {
		dlog.Printf("pty write error: %v", err)
		d.mu.Lock()
		c.sendError(fmt.Sprintf("pty write: %v", err))
		d.mu.Unlock()
	}
}

// HandleInterrupt writes Ctrl-C (\x03) to the pty.
func (d *Daemon) HandleInterrupt() {
	if _, err := d.ptmx.Write([]byte{0x03}); err != nil {
		dlog.Printf("pty write interrupt error: %v", err)
	}
}

// HandleStop gracefully terminates the REPL by sending SIGTERM and waiting
// for the process to exit. Output continues to be piped to the client
// during shutdown. If the child doesn't exit within the timeout, it is
// force-killed.
func (d *Daemon) HandleStop(c *clientConn) {
	dlog.Printf("daemon: stopping session, sending SIGTERM")
	d.cmd.Process.Signal(syscall.SIGTERM)

	// If the child doesn't exit in time, escalate to SIGKILL.
	go func() {
		select {
		case <-d.done:
			return
		case <-time.After(5 * time.Second):
			dlog.Printf("daemon: child didn't exit after SIGTERM, sending SIGKILL")
			d.cmd.Process.Kill()
		}
	}()
}

// HandleKill forcibly terminates the REPL.
func (d *Daemon) HandleKill(c *clientConn) {
	// Try SIGTERM first.
	d.cmd.Process.Signal(syscall.SIGTERM)

	go func() {
		select {
		case <-d.done:
			return
		case <-time.After(3 * time.Second):
			// Force kill after timeout.
			d.cmd.Process.Kill()
		}
	}()
}

// SetClient registers a client connection. Any buffered output is flushed
// to the client and a prompt detector is started. Returns an error if a
// client is already connected.
func (d *Daemon) SetClient(c *clientConn) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.client != nil {
		dlog.Printf("daemon: rejecting client, another already connected")
		return fmt.Errorf("another client is already connected")
	}
	dlog.Printf("daemon: client connected")
	d.client = c

	// Start a prompt detector so that initial output (or buffered output)
	// can trigger prompt detection even before the client sends input.
	if d.detector != nil {
		d.detector.Stop()
	}
	d.detector = prompt.NewDetector(d.promptConfig(), func() {
		d.mu.Lock()
		if d.client == c {
			c.sendPromptDetected()
		}
		d.mu.Unlock()
	})

	// Flush buffered output to the new client and feed the detector.
	buffered := d.buf.ReadAll()
	if len(buffered) > 0 {
		c.sendBuffered(buffered)
		d.detector.Feed(buffered)
	}

	return nil
}

// ClearClient unregisters the current client.
func (d *Daemon) ClearClient(c *clientConn) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.client == c {
		dlog.Printf("daemon: client disconnected")
		if d.detector != nil {
			d.detector.Stop()
			d.detector = nil
		}
		d.client = nil
	}
}

// stripEcho removes the echoed input from the beginning of output data.
// The pty echoes each input line followed by \r\n. This handles the echo
// arriving across multiple output chunks. Caller must hold d.mu.
func (d *Daemon) stripEcho(data []byte) []byte {
	if d.echoStrip == "" {
		return data
	}

	expect := d.echoStrip
	s := string(data)

	// Match the expected echo prefix.
	i := 0
	for i < len(s) && i < len(expect) {
		if s[i] != expect[i] {
			// Mismatch — abandon echo stripping.
			d.echoStrip = ""
			return data
		}
		i++
	}

	if i < len(expect) {
		// Partial match — entire chunk is echo, wait for more.
		d.echoStrip = expect[i:]
		return nil
	}

	// Full match. Clear the strip target and skip trailing \r\n.
	d.echoStrip = ""
	rest := s[i:]
	for len(rest) > 0 && (rest[0] == '\r' || rest[0] == '\n') {
		rest = rest[1:]
	}

	if len(rest) == 0 {
		return nil
	}
	return []byte(rest)
}

// promptConfig builds a prompt.Config from the daemon configuration.
func (d *Daemon) promptConfig() prompt.Config {
	return prompt.Config{
		IdleTimeout: d.cfg.IdleTimeout,
		PromptRegex: d.cfg.PromptRegex,
	}
}

// exitCodeFrom extracts an exit code from a process wait error.
func exitCodeFrom(err error) byte {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return byte(exitErr.ExitCode())
	}
	return 1
}
