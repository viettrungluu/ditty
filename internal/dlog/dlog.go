// Package dlog provides verbose/debug logging for ditty.
//
// Logging is disabled by default and enabled via SetVerbose(true), typically
// in response to the --verbose flag. When disabled, Printf is a no-op.
package dlog

import (
	"log"
	"os"
)

var (
	verbose bool
	logger  = log.New(os.Stderr, "ditty: ", log.LstdFlags)
)

// SetVerbose enables or disables verbose logging.
func SetVerbose(v bool) {
	verbose = v
}

// Verbose returns whether verbose logging is enabled.
func Verbose() bool {
	return verbose
}

// Printf logs a message if verbose logging is enabled.
func Printf(format string, args ...any) {
	if verbose {
		logger.Printf(format, args...)
	}
}
