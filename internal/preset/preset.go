// Package preset provides prompt pattern presets for common REPLs.
//
// Presets are loaded from two sources:
//  1. A user presets file (default: ~/.ditty/presets, overridable with --presets-file).
//  2. Built-in presets compiled into the binary.
//
// Each preset has a name, zero or more command regexes (matched against the
// command line: basename + args), and a string of ditty start flags. Presets
// with no regexes are only selectable via --preset=NAME.
//
// User presets are checked first, then built-ins. First match wins. Explicit
// CLI flags always take precedence over preset flags.
//
// File format: tab-separated triples, one per line. Lines starting with # and
// blank lines are ignored.
//
//	# name	command_regex	flags
//	python	^python\d*(\.\d+)*( |$)	--prompt='(>>>|\.\.\.) $'
//	rails	^rails (console|c)( |$)	--prompt='(irb.*|pry.*)> $' --env=TERM=dumb
//	myrepl		--prompt='> $'
package preset

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Entry is a named preset with optional command regexes and flags.
type Entry struct {
	// Name identifies the preset (e.g., "python", "rails").
	Name string
	// CommandRegexes match against the command line (basename + args,
	// space-separated). If empty, the preset is only selectable via
	// --preset=NAME.
	CommandRegexes []*regexp.Regexp
	// Flags is the raw flags string (e.g., "--prompt='>>> $'").
	Flags string
}

// builtins are the compiled built-in presets.
var builtins = []Entry{
	{
		Name: "python",
		CommandRegexes: []*regexp.Regexp{
			regexp.MustCompile(`^python\d*(\.\d+)*( |$)`),
		},
		Flags: `--prompt='(>>>|\.\.\.) $'`,
	},
	{
		Name: "node",
		CommandRegexes: []*regexp.Regexp{
			regexp.MustCompile(`^node\d*( |$)`),
		},
		Flags: `--prompt='> $'`,
	},
	{
		Name: "gdb",
		CommandRegexes: []*regexp.Regexp{
			regexp.MustCompile(`^gdb( |$)`),
		},
		Flags: `--prompt='\(gdb\) $'`,
	},
	{
		Name: "lldb",
		CommandRegexes: []*regexp.Regexp{
			regexp.MustCompile(`^lldb( |$)`),
		},
		Flags: `--prompt='\(lldb\) $'`,
	},
	{
		Name: "irb",
		CommandRegexes: []*regexp.Regexp{
			regexp.MustCompile(`^irb\d*(\.\d+)*( |$)`),
		},
		Flags: `--prompt='irb.*> $' --env=TERM=dumb`,
	},
	{
		Name: "rails",
		CommandRegexes: []*regexp.Regexp{
			regexp.MustCompile(`^rails (console|c)( |$)`),
		},
		Flags: `--prompt='(irb.*|pry.*)> $' --env=TERM=dumb`,
	},
	{
		Name: "sqlite3",
		CommandRegexes: []*regexp.Regexp{
			regexp.MustCompile(`^sqlite3( |$)`),
		},
		Flags: `--prompt='sqlite> $'`,
	},
	{
		Name: "mysql",
		CommandRegexes: []*regexp.Regexp{
			regexp.MustCompile(`^mysql( |$)`),
		},
		Flags: `--prompt='mysql> $'`,
	},
	{
		Name: "psql",
		CommandRegexes: []*regexp.Regexp{
			regexp.MustCompile(`^psql( |$)`),
		},
		Flags: `--prompt='[=#]> $'`,
	},
	{
		Name: "lua",
		CommandRegexes: []*regexp.Regexp{
			regexp.MustCompile(`^lua\d*(\.\d+)*( |$)`),
		},
		Flags: `--prompt='> $'`,
	},
	{
		Name: "R",
		CommandRegexes: []*regexp.Regexp{
			regexp.MustCompile(`^R( |$)`),
		},
		Flags: `--prompt='> $'`,
	},
}

// Builtins returns a copy of the built-in preset entries.
func Builtins() []Entry {
	out := make([]Entry, len(builtins))
	copy(out, builtins)
	return out
}

// DefaultPresetsFile returns the default path to the user presets file.
func DefaultPresetsFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ditty", "presets"), nil
}

// BuildCommandLine constructs the match string for preset regex matching
// from a command and its arguments. The command is reduced to its basename;
// arguments are appended space-separated.
func BuildCommandLine(args []string) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, len(args))
	parts[0] = filepath.Base(args[0])
	for i := 1; i < len(args); i++ {
		parts[i] = args[i]
	}
	return strings.Join(parts, " ")
}

// Lookup finds the first matching preset. If presetName is non-empty, it
// looks up by name directly. Otherwise, it matches commandLine against
// preset regexes. Entries are checked in order: user presets first, then
// built-ins. First match wins.
//
// Returns the flags string and the matched preset name (for logging).
// Returns an error if presetName is specified but not found.
func Lookup(commandLine string, presetName string, presetsFile string,
	includeBuiltins bool) (string, string, error) {

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

	// If --preset=NAME is specified, find by name.
	if presetName != "" {
		for _, e := range entries {
			if e.Name == presetName {
				return e.Flags, e.Name, nil
			}
		}
		return "", "", fmt.Errorf("preset %q not found", presetName)
	}

	// Auto-detect: match command line against regexes.
	for _, e := range entries {
		for _, re := range e.CommandRegexes {
			if re.MatchString(commandLine) {
				return e.Flags, e.Name, nil
			}
		}
	}
	return "", "", nil
}

// LoadFile parses a presets file. Each non-empty, non-comment line is a
// tab-separated triple: name<TAB>command_regex<TAB>flags. The regex field
// may be empty (preset is only selectable via --preset=NAME).
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

		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			return nil, fmt.Errorf(
				"%s:%d: expected tab-separated name, command_regex, and flags",
				path, lineNum)
		}

		name := parts[0]
		regexStr := parts[1]
		flags := parts[2]

		var regexes []*regexp.Regexp
		if regexStr != "" {
			re, err := regexp.Compile(regexStr)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: invalid command regex: %w",
					path, lineNum, err)
			}
			regexes = append(regexes, re)
		}

		entries = append(entries, Entry{
			Name:           name,
			CommandRegexes: regexes,
			Flags:          flags,
		})
	}
	return entries, scanner.Err()
}

// ParseFlags parses a preset flags string into key-value pairs.
// Supports --key=value and --key (boolean). Returns a map of flag names
// to values. Repeatable flags (like --env) accumulate as NUL-joined values,
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
