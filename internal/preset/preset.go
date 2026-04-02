// Package preset provides prompt pattern presets for common REPLs.
//
// Presets are loaded from two sources:
//  1. A user presets file (default: ~/.ditty/presets, overridable with --presets-file).
//  2. Built-in presets compiled into the binary.
//
// User presets are checked first, then built-ins. First match on the command
// regex wins.
//
// File format: tab-separated pairs of regexes, one per line. Lines starting
// with # and blank lines are ignored.
//
//	# command_regex	prompt_regex
//	python\d*(\.\d+)*$	(>>>|\.\.\.) $
//	node\d*$	> $
package preset

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Entry is a preset: a command pattern and a prompt pattern.
type Entry struct {
	// CommandRegex matches against the command basename.
	CommandRegex *regexp.Regexp
	// PromptRegex is the prompt pattern to use when CommandRegex matches.
	PromptRegex *regexp.Regexp
}

// builtins are the compiled built-in presets.
var builtins = []Entry{
	{regexp.MustCompile(`^python\d*(\.\d+)*$`), regexp.MustCompile(`(>>>|\.\.\.) $`)},
	{regexp.MustCompile(`^node\d*$`), regexp.MustCompile(`> $`)},
	{regexp.MustCompile(`^gdb$`), regexp.MustCompile(`\(gdb\) $`)},
	{regexp.MustCompile(`^lldb$`), regexp.MustCompile(`\(lldb\) $`)},
	{regexp.MustCompile(`^irb\d*(\.\d+)*$`), regexp.MustCompile(`irb.*> $`)},
	{regexp.MustCompile(`^sqlite3$`), regexp.MustCompile(`sqlite> $`)},
	{regexp.MustCompile(`^mysql$`), regexp.MustCompile(`mysql> $`)},
	{regexp.MustCompile(`^psql$`), regexp.MustCompile(`[=#]> $`)},
	{regexp.MustCompile(`^lua\d*(\.\d+)*$`), regexp.MustCompile(`> $`)},
	{regexp.MustCompile(`^R$`), regexp.MustCompile(`> $`)},
}

// DefaultPresetsFile returns the default path to the user presets file.
func DefaultPresetsFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ditty", "presets"), nil
}

// Lookup finds the first matching preset for the given command. It checks
// entries in order: user presets first, then built-ins. The command is
// matched against the basename.
//
// If presetsFile is non-empty, user presets are loaded from it. If
// includeBuiltins is true, built-in presets are appended after user presets.
func Lookup(command string, presetsFile string, includeBuiltins bool) (*regexp.Regexp, string, error) {
	base := filepath.Base(command)

	var entries []Entry

	// Load user presets file.
	if presetsFile != "" {
		userEntries, err := LoadFile(presetsFile)
		if err != nil {
			// File not existing is fine — it's optional.
			if !os.IsNotExist(err) {
				return nil, "", fmt.Errorf("load presets file: %w", err)
			}
		}
		entries = append(entries, userEntries...)
	}

	// Append built-ins.
	if includeBuiltins {
		entries = append(entries, builtins...)
	}

	// First match wins.
	for _, e := range entries {
		if e.CommandRegex.MatchString(base) {
			return e.PromptRegex, e.CommandRegex.String(), nil
		}
	}
	return nil, "", nil
}

// LoadFile parses a presets file. Each non-empty, non-comment line is a
// tab-separated pair: command_regex<TAB>prompt_regex.
func LoadFile(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("%s:%d: expected tab-separated command_regex and prompt_regex",
				path, lineNum)
		}

		cmdRe, err := regexp.Compile(parts[0])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: invalid command regex: %w",
				path, lineNum, err)
		}
		promptRe, err := regexp.Compile(parts[1])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: invalid prompt regex: %w",
				path, lineNum, err)
		}

		entries = append(entries, Entry{
			CommandRegex: cmdRe,
			PromptRegex:  promptRe,
		})
	}
	return entries, scanner.Err()
}
