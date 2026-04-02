package cmd

import "os"

// resetTerminal writes escape sequences to undo common terminal state
// changes that REPLs make during startup or operation. This prevents
// mode changes from persisting after ditty returns to the shell.
//
// Common offenders:
//   - Bracketed paste mode (\e[?2004h) — irb, python with reline
//   - Application cursor keys (\e[?1h) — irb, some readline configs
//   - Cursor visibility changes (\e[?25l)
//   - Text attribute changes (colors, bold, etc.)
func resetTerminal() {
	// Only reset if stdout is a terminal.
	if !isTerminal(os.Stdout) {
		return
	}

	os.Stdout.WriteString("" +
		"\x1b[?2004l" + // disable bracketed paste mode
		"\x1b[?1l" + // disable application cursor keys
		"\x1b[?25h" + // show cursor
		"\x1b[0m", // reset text attributes
	)
}
