// Package prompt implements REPL prompt detection.
//
// Two strategies are supported:
//
//  1. Idle timeout (default): if the REPL stops producing output for a
//     configurable duration and the last byte is not '\n', we assume a
//     prompt has been printed.
//
//  2. Regex match: if a --prompt regex is provided, the detector checks
//     whether accumulated output (since the last reset) matches the
//     pattern at the end. This fires immediately on match, with no
//     timeout delay.
package prompt

import (
	"regexp"
	"sync"
	"time"
)

// DefaultIdleTimeout is the default duration to wait for output to quiesce
// before declaring that a prompt has been detected.
const DefaultIdleTimeout = 200 * time.Millisecond

// regexDebounce is a short debounce for regex-based detection, to avoid
// running the regex on every single byte of output.
const regexDebounce = 10 * time.Millisecond

// Config configures a prompt detector.
type Config struct {
	// IdleTimeout is the idle timeout for the default strategy.
	IdleTimeout time.Duration
	// PromptRegex, if non-nil, enables regex-based prompt detection.
	PromptRegex *regexp.Regexp
}

// Detector watches a stream of output chunks and signals when a prompt is
// likely present. It is safe for concurrent use.
type Detector struct {
	mu       sync.Mutex
	cfg      Config
	timer    *time.Timer
	buf      []byte // accumulated output for regex matching
	lastByte byte
	hasData  bool
	onPrompt func() // called (once) when prompt is detected
	stopped  bool
}

// NewDetector creates a prompt detector with the given config.
// The onPrompt callback is called at most once when a prompt is detected.
// After firing, the detector must be Reset before it can fire again.
func NewDetector(cfg Config, onPrompt func()) *Detector {
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = DefaultIdleTimeout
	}
	return &Detector{
		cfg:      cfg,
		onPrompt: onPrompt,
	}
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

	if d.cfg.PromptRegex != nil {
		d.buf = append(d.buf, data...)
	}

	// Reset or start the timer.
	timeout := d.cfg.IdleTimeout
	if d.cfg.PromptRegex != nil {
		timeout = regexDebounce
	}
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(timeout, d.check)
}

// check is called when the timer fires.
func (d *Detector) check() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped || !d.hasData {
		return
	}

	if d.cfg.PromptRegex != nil {
		// Regex mode: check if the accumulated output matches.
		if d.cfg.PromptRegex.Match(d.buf) {
			d.stopped = true
			d.onPrompt()
		}
		// No match yet — wait for more data.
		return
	}

	// Default mode: a prompt is a partial line — doesn't end with newline.
	if d.lastByte != '\n' {
		d.stopped = true
		d.onPrompt()
	}
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
	d.buf = d.buf[:0]
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
