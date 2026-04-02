package preset

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestLookupBuiltins(t *testing.T) {
	tests := []struct {
		commandLine string
		wantMatch   bool
		wantName    string
	}{
		{"python3", true, "python"},
		{"python", true, "python"},
		{"python3.12", true, "python"},
		{"python3 -i", true, "python"},
		{"python3 script.py", true, "python"},
		{"node", true, "node"},
		{"node18", true, "node"},
		{"gdb", true, "gdb"},
		{"gdb ./a.out", true, "gdb"},
		{"lldb", true, "lldb"},
		{"irb", true, "irb"},
		{"sqlite3", true, "sqlite3"},
		{"sqlite3 test.db", true, "sqlite3"},
		{"mysql", true, "mysql"},
		{"psql", true, "psql"},
		{"lua", true, "lua"},
		{"R", true, "R"},
		{"rails console", true, "rails"},
		{"rails c", true, "rails"},
		{"rails c --sandbox", true, "rails"},
		{"rails server", false, ""},
		{"rails", false, ""},
		{"someunknown", false, ""},
		{"cat", false, ""},
		{"bash", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.commandLine, func(t *testing.T) {
			flags, name, err := Lookup(tt.commandLine, "", "", true)
			if err != nil {
				t.Fatalf("Lookup error: %v", err)
			}
			if tt.wantMatch && flags == "" {
				t.Error("expected match, got empty")
			}
			if !tt.wantMatch && flags != "" {
				t.Errorf("expected no match, got %q", flags)
			}
			if tt.wantName != "" && name != tt.wantName {
				t.Errorf("expected name %q, got %q", tt.wantName, name)
			}
		})
	}
}

func TestLookupByName(t *testing.T) {
	// --preset=python should match regardless of command line.
	flags, name, err := Lookup("someunknown", "python", "", true)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if flags == "" {
		t.Error("expected match by name, got empty")
	}
	if name != "python" {
		t.Errorf("expected name 'python', got %q", name)
	}

	// Unknown preset name should return an error.
	_, _, err = Lookup("", "nonexistent", "", true)
	if err == nil {
		t.Error("expected error for unknown preset name")
	}
}

func TestLookupNoBuiltins(t *testing.T) {
	flags, _, err := Lookup("python3", "", "", false)
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
		"myrepl\t^myrepl( |$)\t--prompt='myrepl> $'\n" +
		"python\t^python\\d*( |$)\t--prompt='CUSTOM>>> $' --env=PYTHONDONTWRITEBYTECODE=1\n" +
		"headless\t\t--prompt='> $' --echo=false\n"
	if err := os.WriteFile(presetsFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// User preset should match.
	flags, name, err := Lookup("myrepl", "", presetsFile, true)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if flags == "" {
		t.Fatal("expected match for myrepl")
	}
	if name != "myrepl" {
		t.Errorf("expected name 'myrepl', got %q", name)
	}
	parsed := ParseFlags(flags)
	if parsed["prompt"] != "myrepl> $" {
		t.Errorf(`expected prompt "myrepl> $", got %q`, parsed["prompt"])
	}

	// User preset should override built-in (first match wins, same name).
	flags, _, err = Lookup("python3", "", presetsFile, true)
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

	// No-regex preset should only be accessible via --preset.
	flags, _, err = Lookup("headless", "", presetsFile, true)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if flags != "" {
		t.Errorf("expected no auto-match for no-regex preset, got %q", flags)
	}

	flags, name, err = Lookup("", "headless", presetsFile, true)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if flags == "" {
		t.Fatal("expected match for headless via --preset")
	}
	if name != "headless" {
		t.Errorf("expected name 'headless', got %q", name)
	}
}

func TestLookupMissingFile(t *testing.T) {
	flags, _, err := Lookup("python3", "", "/nonexistent/presets", true)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if flags == "" {
		t.Error("expected builtin match despite missing file")
	}
}

func TestLoadFileErrors(t *testing.T) {
	dir := t.TempDir()

	// Missing tab separator (only 1 or 2 fields).
	bad1 := filepath.Join(dir, "bad1")
	os.WriteFile(bad1, []byte("notabs\n"), 0o644)
	_, err := LoadFile(bad1)
	if err == nil {
		t.Error("expected error for missing tabs")
	}

	// Two fields (old format) should also error.
	bad2 := filepath.Join(dir, "bad2")
	os.WriteFile(bad2, []byte("^test$\t--prompt=x\n"), 0o644)
	_, err = LoadFile(bad2)
	if err == nil {
		t.Error("expected error for two-field format")
	}

	// Invalid command regex.
	bad3 := filepath.Join(dir, "bad3")
	os.WriteFile(bad3, []byte("test\t[invalid\t--prompt=x\n"), 0o644)
	_, err = LoadFile(bad3)
	if err == nil {
		t.Error("expected error for invalid command regex")
	}
}

func TestLoadFileCommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "presets")

	content := "# comment\n\n  # indented comment\ntest\t^test( |$)\t--prompt=test> $\n\n"
	os.WriteFile(f, []byte(content), 0o644)

	entries, err := LoadFile(f)
	if err != nil {
		t.Fatalf("LoadFile error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "test" {
		t.Errorf("expected name 'test', got %q", entries[0].Name)
	}
	if entries[0].Flags != "--prompt=test> $" {
		t.Errorf("expected '--prompt=test> $', got %q", entries[0].Flags)
	}
}

func TestLoadFileEmptyRegex(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "presets")

	content := "mypreset\t\t--prompt='> $'\n"
	os.WriteFile(f, []byte(content), 0o644)

	entries, err := LoadFile(f)
	if err != nil {
		t.Fatalf("LoadFile error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "mypreset" {
		t.Errorf("expected name 'mypreset', got %q", entries[0].Name)
	}
	if len(entries[0].CommandRegexes) != 0 {
		t.Errorf("expected no regexes, got %d", len(entries[0].CommandRegexes))
	}
}

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  map[string]string
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
		commandLine string
		prompt      string
	}{
		{"python3", ">>> "},
		{"python3", "... "},
		{"node", "> "},
		{"gdb", "(gdb) "},
		{"lldb", "(lldb) "},
		{"sqlite3", "sqlite> "},
		{"mysql", "mysql> "},
		{"rails console", "irb(main):001:0> "},
		{"rails c", "[1] pry(main)> "},
	}

	for _, tt := range tests {
		t.Run(tt.commandLine+"/"+tt.prompt, func(t *testing.T) {
			flags, _, err := Lookup(tt.commandLine, "", "", true)
			if err != nil {
				t.Fatalf("Lookup error: %v", err)
			}
			parsed := ParseFlags(flags)
			pattern, ok := parsed["prompt"]
			if !ok {
				t.Fatalf("no prompt in preset for %q", tt.commandLine)
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

func TestBuildCommandLine(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"empty", nil, ""},
		{"command only", []string{"python3"}, "python3"},
		{"full path", []string{"/usr/bin/python3"}, "python3"},
		{"with args", []string{"rails", "console"}, "rails console"},
		{"path with args", []string{"/usr/bin/gdb", "./a.out"}, "gdb ./a.out"},
		{"multiple args", []string{"bundle", "exec", "rails", "c"}, "bundle exec rails c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildCommandLine(tt.args)
			if got != tt.want {
				t.Errorf("BuildCommandLine(%v) = %q, want %q", tt.args, got, tt.want)
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

func TestBuiltins(t *testing.T) {
	entries := Builtins()
	if len(entries) != len(builtins) {
		t.Fatalf("expected %d builtins, got %d", len(builtins), len(entries))
	}
	// Verify it's a copy, not a reference to the original.
	entries[0].Name = "modified"
	if builtins[0].Name == "modified" {
		t.Error("Builtins() returned a reference, not a copy")
	}
}
