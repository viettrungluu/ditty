package preset

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestLookupBuiltins(t *testing.T) {
	tests := []struct {
		command   string
		wantMatch bool
	}{
		{"python3", true},
		{"python", true},
		{"python3.12", true},
		{"/usr/bin/python3", true},
		{"/opt/homebrew/bin/python3.12", true},
		{"node", true},
		{"node18", true},
		{"gdb", true},
		{"lldb", true},
		{"irb", true},
		{"sqlite3", true},
		{"mysql", true},
		{"psql", true},
		{"lua", true},
		{"R", true},
		{"rails", true},
		{"someunknown", false},
		{"cat", false},
		{"bash", false},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			flags, _, err := Lookup(tt.command, "", true)
			if err != nil {
				t.Fatalf("Lookup error: %v", err)
			}
			if tt.wantMatch && flags == "" {
				t.Error("expected match, got empty")
			}
			if !tt.wantMatch && flags != "" {
				t.Errorf("expected no match, got %q", flags)
			}
		})
	}
}

func TestLookupNoBuiltins(t *testing.T) {
	flags, _, err := Lookup("python3", "", false)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if flags != "" {
		t.Errorf("expected no match with builtins disabled, got %q", flags)
	}
}

func TestLookupUserPresets(t *testing.T) {
	dir := t.TempDir()
	presetsFile := filepath.Join(dir, "presets")

	content := "# My presets\n" +
		"^myrepl$\t--prompt='myrepl> $'\n" +
		"^python\\d*$\t--prompt='CUSTOM>>> $' --env=PYTHONDONTWRITEBYTECODE=1\n"
	if err := os.WriteFile(presetsFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// User preset should match.
	flags, _, err := Lookup("myrepl", presetsFile, true)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if flags == "" {
		t.Fatal("expected match for myrepl")
	}
	parsed := ParseFlags(flags)
	if parsed["prompt"] != "myrepl> $" {
		t.Errorf(`expected prompt "myrepl> $", got %q`, parsed["prompt"])
	}

	// User preset should override built-in (first match wins).
	flags, _, err = Lookup("python3", presetsFile, true)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	parsed = ParseFlags(flags)
	if parsed["prompt"] != "CUSTOM>>> $" {
		t.Errorf(`expected user prompt override, got %q`, parsed["prompt"])
	}
	envs := ParseEnvFlags(parsed)
	if len(envs) != 1 || envs[0] != "PYTHONDONTWRITEBYTECODE=1" {
		t.Errorf("expected env var, got %v", envs)
	}
}

func TestLookupMissingFile(t *testing.T) {
	flags, _, err := Lookup("python3", "/nonexistent/presets", true)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if flags == "" {
		t.Error("expected builtin match despite missing file")
	}
}

func TestLoadFileErrors(t *testing.T) {
	dir := t.TempDir()

	// Missing tab separator.
	bad1 := filepath.Join(dir, "bad1")
	os.WriteFile(bad1, []byte("notabhere\n"), 0o644)
	_, err := LoadFile(bad1)
	if err == nil {
		t.Error("expected error for missing tab")
	}

	// Invalid command regex.
	bad2 := filepath.Join(dir, "bad2")
	os.WriteFile(bad2, []byte("[invalid\t--prompt=x\n"), 0o644)
	_, err = LoadFile(bad2)
	if err == nil {
		t.Error("expected error for invalid command regex")
	}
}

func TestLoadFileCommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "presets")

	content := "# comment\n\n  # indented comment\n^test$\t--prompt=test> $\n\n"
	os.WriteFile(f, []byte(content), 0o644)

	entries, err := LoadFile(f)
	if err != nil {
		t.Fatalf("LoadFile error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Flags != "--prompt=test> $" {
		t.Errorf("expected '--prompt=test> $', got %q", entries[0].Flags)
	}
}

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   map[string]string
	}{
		{
			name:  "prompt only",
			input: `--prompt='(>>>|\.\.\.) $'`,
			want:  map[string]string{"prompt": `(>>>|\.\.\.) $`},
		},
		{
			name:  "multiple flags",
			input: `--prompt='> $' --idle-timeout=100ms`,
			want:  map[string]string{"prompt": "> $", "idle-timeout": "100ms"},
		},
		{
			name:  "boolean flag",
			input: "--no-pty --suspend",
			want:  map[string]string{"no-pty": "true", "suspend": "true"},
		},
		{
			name:  "env vars",
			input: "--env=TERM=dumb --env=FOO=bar",
			want:  map[string]string{"env": "TERM=dumb\x00FOO=bar"},
		},
		{
			name:  "echo false",
			input: "--echo=false",
			want:  map[string]string{"echo": "false"},
		},
		{
			name:  "mixed",
			input: `--prompt='\(gdb\) $' --env=TERM=dumb --suspend`,
			want: map[string]string{
				"prompt":  `\(gdb\) $`,
				"env":     "TERM=dumb",
				"suspend": "true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseFlags(tt.input)
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("key %q: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestParseEnvFlags(t *testing.T) {
	parsed := ParseFlags("--env=TERM=dumb --env=FOO=bar")
	envs := ParseEnvFlags(parsed)
	if len(envs) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(envs))
	}
	if envs[0] != "TERM=dumb" || envs[1] != "FOO=bar" {
		t.Errorf("unexpected env vars: %v", envs)
	}
}

func TestBuiltinPromptRegexes(t *testing.T) {
	tests := []struct {
		command string
		prompt  string
	}{
		{"python3", ">>> "},
		{"python3", "... "},
		{"node", "> "},
		{"gdb", "(gdb) "},
		{"lldb", "(lldb) "},
		{"sqlite3", "sqlite> "},
		{"mysql", "mysql> "},
		{"rails", "irb(main):001:0> "},
		{"rails", "[1] pry(main)> "},
	}

	for _, tt := range tests {
		t.Run(tt.command+"/"+tt.prompt, func(t *testing.T) {
			flags, _, err := Lookup(tt.command, "", true)
			if err != nil {
				t.Fatalf("Lookup error: %v", err)
			}
			parsed := ParseFlags(flags)
			pattern, ok := parsed["prompt"]
			if !ok {
				t.Fatalf("no prompt in preset for %q", tt.command)
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				t.Fatalf("invalid prompt regex %q: %v", pattern, err)
			}
			if !re.MatchString(tt.prompt) {
				t.Errorf("regex %v did not match %q", re, tt.prompt)
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"--prompt=>>> $", []string{"--prompt=>>>", "$"}},
		{"--prompt='>>> $'", []string{"--prompt=>>> $"}},
		{`--prompt=">>> $"`, []string{"--prompt=>>> $"}},
		{"--env=TERM=dumb --suspend", []string{"--env=TERM=dumb", "--suspend"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := tokenize(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("token %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
