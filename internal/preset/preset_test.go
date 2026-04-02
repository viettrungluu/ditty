package preset

import (
	"testing"
)

func TestLookup(t *testing.T) {
	tests := []struct {
		command string
		want    string // expected preset name, or "" for no match
	}{
		{"python3", "python"},
		{"python", "python"},
		{"python3.12", "python"},
		{"/usr/bin/python3", "python"},
		{"/opt/homebrew/bin/python3.12", "python"},
		{"node", "node"},
		{"node18", "node"},
		{"gdb", "gdb"},
		{"lldb", "lldb"},
		{"irb", "irb"},
		{"sqlite3", "sqlite3"},
		{"mysql", "mysql"},
		{"psql", "psql"},
		{"lua", "lua"},
		{"R", "R"},
		{"someunknown", ""},
		{"cat", ""},
		{"bash", ""},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			e := Lookup(tt.command)
			if tt.want == "" {
				if e != nil {
					t.Errorf("expected no match, got %q", e.Name)
				}
				return
			}
			if e == nil {
				t.Fatalf("expected match %q, got nil", tt.want)
			}
			if e.Name != tt.want {
				t.Errorf("expected %q, got %q", tt.want, e.Name)
			}
		})
	}
}

func TestStripVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"python3", "python"},
		{"python3.12", "python"},
		{"python", "python"},
		{"node18", "node"},
		{"node", "node"},
		{"sqlite3", "sqlite"},
		{"gdb", "gdb"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := stripVersion(tt.input)
			if got != tt.want {
				t.Errorf("stripVersion(%q) = %q, want %q",
					tt.input, got, tt.want)
			}
		})
	}
}

func TestPromptRegexes(t *testing.T) {
	// Verify that preset regexes match expected prompts.
	tests := []struct {
		name   string
		prompt string
	}{
		{"python", ">>> "},
		{"python", "... "},
		{"node", "> "},
		{"gdb", "(gdb) "},
		{"lldb", "(lldb) "},
		{"irb", "irb(main):001:0> "},
		{"sqlite3", "sqlite> "},
		{"mysql", "mysql> "},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.prompt, func(t *testing.T) {
			e, ok := presets[tt.name]
			if !ok {
				t.Fatalf("preset %q not found", tt.name)
			}
			if !e.PromptRegex.MatchString(tt.prompt) {
				t.Errorf("regex %v did not match %q",
					e.PromptRegex, tt.prompt)
			}
		})
	}
}
