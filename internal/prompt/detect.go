// Package prompt implements REPL prompt detection.
//
// The default strategy uses an idle timeout combined with a trailing-newline
// check: if the REPL stops producing output for a configurable duration and
// the last byte received is not '\n', we assume a prompt has been printed and
// the REPL is waiting for input.
package prompt

import (
	"sync"
	"time"
)

// DefaultIdleTimeout is the default duration to wait for output to quiesce
// before declaring that a prompt has been detected.
const DefaultIdleTimeout = 200 * time.Millisecond

// Detector watches a stream of output chunks and signals when a prompt is
// likely present. It is safe for concurrent use.
type Detector struct {
	mu          sync.Mutex
	idleTimeout time.Duration
	timer       *time.Timer
	lastByte    byte
	hasData     bool
	onPrompt    func() // called (once) when prompt is detected
	stopped     bool
}

// NewDetector creates a prompt detector with the given idle timeout.
// The onPrompt callback is called at most once when a prompt is detected.
// After firing, the detector must be Reset before it can fire again.
func NewDetector(idleTimeout time.Duration, onPrompt func()) *Detector {
	if idleTimeout <= 0 {
		idleTimeout = DefaultIdleTimeout
	}
	d := &Detector{
		idleTimeout: idleTimeout,
		onPrompt:    onPrompt,
	}
	return d
}

// Feed provides a chunk of output to the detector. Each call resets the idle
// timer. Feed must not be called after Stop.
func (d *Detector) Feed(data []byte) {
	if len(data) == 0 {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped {
		return
	}

	d.lastByte = data[len(data)-1]
	d.hasData = true

	// Reset or start the idle timer.
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.idleTimeout, d.check)
}

// check is called when the idle timer fires.
func (d *Detector) check() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped || !d.hasData {
		return
	}

	// A prompt is a partial line — it doesn't end with a newline.
	if d.lastByte != '\n' {
		d.stopped = true
		d.onPrompt()
	}
	// If the last byte is '\n', this is probably normal output that just
	// paused briefly. We do nothing and wait for more data (or another
	// idle period).
}

// Reset re-arms the detector so it can detect another prompt. This should
// be called after sending new input to the REPL.
func (d *Detector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	d.lastByte = 0
	d.hasData = false
	d.stopped = false
}

// Stop cancels any pending timer and prevents further callbacks. It is safe
// to call multiple times.
func (d *Detector) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stopped = true
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
}
