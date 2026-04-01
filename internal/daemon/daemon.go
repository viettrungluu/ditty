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
	"log"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty/v2"
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
}

// Daemon manages a single REPL session.
type Daemon struct {
	cfg    Config
	ptmx   *os.File // pty master
	cmd    *exec.Cmd
	buf    *ringbuf.RingBuf
	server *Server

	// mu protects client and detector.
	mu       sync.Mutex
	client   *clientConn // currently connected client, or nil
	detector *prompt.Detector

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
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

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
				log.Printf("pty read error: %v", err)
			}
			return
		}
	}
}

// handleOutput sends data to the connected client or buffers it.
func (d *Daemon) handleOutput(data []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.client != nil {
		// Stream to connected client.
		if err := d.client.sendOutput(data); err != nil {
			log.Printf("client send error: %v", err)
		}
		// Feed the prompt detector.
		if d.detector != nil {
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
	d.detector = prompt.NewDetector(d.cfg.IdleTimeout, func() {
		d.mu.Lock()
		if d.client == c {
			c.sendPromptDetected()
		}
		d.mu.Unlock()
	})

	d.mu.Unlock()

	// Write the input to the pty (with a trailing newline if not present).
	input := data
	if len(input) == 0 || input[len(input)-1] != '\n' {
		input = append(input, '\n')
	}
	if _, err := d.ptmx.Write(input); err != nil {
		log.Printf("pty write error: %v", err)
		d.mu.Lock()
		c.sendError(fmt.Sprintf("pty write: %v", err))
		d.mu.Unlock()
	}
}

// HandleInterrupt writes Ctrl-C (\x03) to the pty.
func (d *Daemon) HandleInterrupt() {
	if _, err := d.ptmx.Write([]byte{0x03}); err != nil {
		log.Printf("pty write interrupt error: %v", err)
	}
}

// HandleStop gracefully terminates the REPL by closing the pty (sending
// EOF) and waiting for the process to exit.
func (d *Daemon) HandleStop(c *clientConn) {
	d.ptmx.Close()
	// waitChild will send the Exited message and close d.done.
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
		return fmt.Errorf("another client is already connected")
	}
	d.client = c

	// Start a prompt detector so that initial output (or buffered output)
	// can trigger prompt detection even before the client sends input.
	if d.detector != nil {
		d.detector.Stop()
	}
	d.detector = prompt.NewDetector(d.cfg.IdleTimeout, func() {
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
		if d.detector != nil {
			d.detector.Stop()
			d.detector = nil
		}
		d.client = nil
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
