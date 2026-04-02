// Package preset provides built-in prompt patterns for common REPLs.
//
// When a session is started without an explicit --prompt, the program name
// is matched against known presets to auto-select a prompt regex.
package preset

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Entry is a preset prompt configuration.
type Entry struct {
	// Name is the display name of the preset (e.g., "python").
	Name string
	// PromptRegex is the compiled prompt pattern.
	PromptRegex *regexp.Regexp
}

// presets maps program name prefixes to prompt patterns. The key is matched
// against the basename of the command (after stripping version suffixes).
var presets = map[string]Entry{
	"python": {
		Name:        "python",
		PromptRegex: regexp.MustCompile(`(>>>|\.\.\.) $`),
	},
	"node": {
		Name:        "node",
		PromptRegex: regexp.MustCompile(`> $`),
	},
	"gdb": {
		Name:        "gdb",
		PromptRegex: regexp.MustCompile(`\(gdb\) $`),
	},
	"lldb": {
		Name:        "lldb",
		PromptRegex: regexp.MustCompile(`\(lldb\) $`),
	},
	"irb": {
		Name:        "irb",
		PromptRegex: regexp.MustCompile(`irb.*> $`),
	},
	"sqlite3": {
		Name:        "sqlite3",
		PromptRegex: regexp.MustCompile(`sqlite> $`),
	},
	"mysql": {
		Name:        "mysql",
		PromptRegex: regexp.MustCompile(`mysql> $`),
	},
	"psql": {
		Name:        "psql",
		PromptRegex: regexp.MustCompile(`[=#]> $`),
	},
	"lua": {
		Name:        "lua",
		PromptRegex: regexp.MustCompile(`> $`),
	},
	"R": {
		Name:        "R",
		PromptRegex: regexp.MustCompile(`> $`),
	},
}

// Lookup finds a preset for the given command. It checks the basename of
// the command, stripping common version suffixes (e.g., "python3.12" →
// "python"). Returns nil if no preset matches.
func Lookup(command string) *Entry {
	base := filepath.Base(command)

	// Try exact match first.
	if e, ok := presets[base]; ok {
		return &e
	}

	// Strip version suffixes: "python3.12" → "python3" → "python",
	// "node18" → "node".
	normalized := stripVersion(base)
	if e, ok := presets[normalized]; ok {
		return &e
	}

	return nil
}

// List returns all available preset names.
func List() []string {
	names := make([]string, 0, len(presets))
	for _, e := range presets {
		names = append(names, e.Name)
	}
	return names
}

// stripVersion removes version suffixes from a program name.
// "python3.12" → "python", "python3" → "python", "node18" → "node".
func stripVersion(name string) string {
	// Strip trailing digits and dots: "python3.12" → "python"
	i := len(name)
	for i > 0 && (name[i-1] >= '0' && name[i-1] <= '9' || name[i-1] == '.') {
		i--
	}
	stripped := name[:i]

	// If we stripped everything or got an empty string, try removing
	// just the last segment after a dot.
	if stripped == "" {
		return name
	}

	// Also try without trailing digits on the stripped version:
	// "python3" already became "python" above. But handle "Rscript" etc.
	if idx := strings.LastIndexAny(stripped, "0123456789"); idx > 0 {
		// Only strip if the digits are at the end.
		allDigits := true
		for _, c := range stripped[idx:] {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return stripped[:idx]
		}
	}

	return stripped
}
