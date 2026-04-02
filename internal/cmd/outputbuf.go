package cmd

import (
	"bytes"
	"os"
)

// outputBuffer tracks output and holds back the trailing partial line.
// Complete lines (ending with \n) are flushed to stdout immediately.
// The trailing partial line (the prompt) is retained and can be retrieved
// after prompt detection.
type outputBuffer struct {
	// partial holds the current incomplete line (no trailing \n).
	partial []byte
}

// Write processes output data, printing complete lines to stdout and
// holding back any trailing partial line.
func (b *outputBuffer) Write(data []byte) {
	// Prepend any buffered partial line.
	if len(b.partial) > 0 {
		data = append(b.partial, data...)
		b.partial = nil
	}

	// Find the last newline — everything up to and including it is
	// complete lines that can be printed.
	lastNL := bytes.LastIndexByte(data, '\n')
	if lastNL >= 0 {
		os.Stdout.Write(data[:lastNL+1])
		data = data[lastNL+1:]
	}

	// Anything remaining is a partial line (the prompt).
	if len(data) > 0 {
		b.partial = make([]byte, len(data))
		copy(b.partial, data)
	}
}

// Partial returns the current trailing partial line (the detected prompt).
func (b *outputBuffer) Partial() string {
	return string(b.partial)
}

// Flush writes any remaining partial line to stdout.
func (b *outputBuffer) Flush() {
	if len(b.partial) > 0 {
		os.Stdout.Write(b.partial)
		b.partial = nil
	}
}
