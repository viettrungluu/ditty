// Package preset provides prompt pattern presets for common REPLs.
//
// Presets are loaded from two sources:
//  1. A user presets file (default: ~/.ditty/presets, overridable with --presets-file).
//  2. Built-in presets compiled into the binary.
//
// Each preset is a pair: a command regex (matched against the program basename)
// and a string of ditty start flags to apply as defaults. User presets are
// checked first, then built-ins. First match wins. Explicit CLI flags always
// take precedence over preset flags.
//
// File format: tab-separated pairs, one per line. Lines starting with # and
// blank lines are ignored.
//
//	# command_regex	flags
//	^python\d*(\.\d+)*$	--prompt=(>>>|\.\.\.) $
//	^irb\d*$	--env=TERM=dumb
//	^rails$	--prompt=(irb.*|pry.*)> $ --env=TERM=dumb
//	^gdb$	--prompt=\(gdb\) $
package preset

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Entry is a preset: a command pattern and flags to apply.
type Entry struct {
	// CommandRegex matches against the command basename.
	CommandRegex *regexp.Regexp
	// Flags is the raw flags string (e.g., "--prompt=(>>>|\\.\\.\\.) $").
	Flags string
}

// builtins are the compiled built-in presets.
var builtins = []Entry{
	{regexp.MustCompile(`^python\d*(\.\d+)*$`), `--prompt='(>>>|\.\.\.) $'`},
	{regexp.MustCompile(`^node\d*$`), `--prompt='> $'`},
	{regexp.MustCompile(`^gdb$`), `--prompt='\(gdb\) $'`},
	{regexp.MustCompile(`^lldb$`), `--prompt='\(lldb\) $'`},
	{regexp.MustCompile(`^irb\d*(\.\d+)*$`), `--prompt='irb.*> $' --env=TERM=dumb`},
	{regexp.MustCompile(`^rails$`), `--prompt='(irb.*|pry.*)> $' --env=TERM=dumb`},
	{regexp.MustCompile(`^sqlite3$`), `--prompt='sqlite> $'`},
	{regexp.MustCompile(`^mysql$`), `--prompt='mysql> $'`},
	{regexp.MustCompile(`^psql$`), `--prompt='[=#]> $'`},
	{regexp.MustCompile(`^lua\d*(\.\d+)*$`), `--prompt='> $'`},
	{regexp.MustCompile(`^R$`), `--prompt='> $'`},
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
// Returns the flags string and the matched command regex (for logging).
func Lookup(command string, presetsFile string, includeBuiltins bool) (string, string, error) {
	base := filepath.Base(command)

	var entries []Entry

	// Load user presets file.
	if presetsFile != "" {
		userEntries, err := LoadFile(presetsFile)
		if err != nil {
			if !os.IsNotExist(err) {
				return "", "", fmt.Errorf("load presets file: %w", err)
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
			return e.Flags, e.CommandRegex.String(), nil
		}
	}
	return "", "", nil
}

// LoadFile parses a presets file. Each non-empty, non-comment line is a
// tab-separated pair: command_regex<TAB>flags.
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
			return nil, fmt.Errorf("%s:%d: expected tab-separated command_regex and flags",
				path, lineNum)
		}

		cmdRe, err := regexp.Compile(parts[0])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: invalid command regex: %w",
				path, lineNum, err)
		}

		entries = append(entries, Entry{
			CommandRegex: cmdRe,
			Flags:        parts[1],
		})
	}
	return entries, scanner.Err()
}

// ParseFlags parses a preset flags string into key-value pairs.
// Supports --key=value and --key (boolean). Returns a map of flag names
// to values. Repeatable flags (like --env) accumulate as comma-joined values,
// but callers should use ParseEnvFlags for --env specifically.
func ParseFlags(flags string) map[string]string {
	result := make(map[string]string)
	tokens := tokenize(flags)

	for _, tok := range tokens {
		if !strings.HasPrefix(tok, "--") {
			continue
		}
		tok = tok[2:] // strip --

		if idx := strings.Index(tok, "="); idx >= 0 {
			key := tok[:idx]
			val := tok[idx+1:]
			if key == "env" {
				// Accumulate env vars.
				if prev, ok := result["env"]; ok {
					result["env"] = prev + "\x00" + val
				} else {
					result["env"] = val
				}
			} else {
				result[key] = val
			}
		} else {
			// Boolean flag.
			result[tok] = "true"
		}
	}
	return result
}

// ParseEnvFlags extracts --env values from a parsed flags map.
func ParseEnvFlags(parsed map[string]string) []string {
	envStr, ok := parsed["env"]
	if !ok {
		return nil
	}
	return strings.Split(envStr, "\x00")
}

// tokenize splits a flags string into tokens, respecting single and double
// quotes. This handles values with spaces like --prompt=(>>>|\.\.\.) $.
func tokenize(s string) []string {
	var tokens []string
	var current strings.Builder
	inSingle := false
	inDouble := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case (c == ' ' || c == '\t') && !inSingle && !inDouble:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}
