package preset

import (
	"os"
	"path/filepath"
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
		{"someunknown", false},
		{"cat", false},
		{"bash", false},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			re, _, err := Lookup(tt.command, "", true)
			if err != nil {
				t.Fatalf("Lookup error: %v", err)
			}
			if tt.wantMatch && re == nil {
				t.Error("expected match, got nil")
			}
			if !tt.wantMatch && re != nil {
				t.Errorf("expected no match, got %v", re)
			}
		})
	}
}

func TestLookupNoBuiltins(t *testing.T) {
	re, _, err := Lookup("python3", "", false)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if re != nil {
		t.Error("expected no match with builtins disabled")
	}
}

func TestLookupUserPresets(t *testing.T) {
	dir := t.TempDir()
	presetsFile := filepath.Join(dir, "presets")

	// Write a user presets file.
	content := "# My presets\n" +
		"^myrepl$\tmyrepl> $\n" +
		"^python\\d*$\tCUSTOM>>> $\n" // override built-in python
	if err := os.WriteFile(presetsFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// User preset should match.
	re, _, err := Lookup("myrepl", presetsFile, true)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if re == nil {
		t.Fatal("expected match for myrepl")
	}
	if !re.MatchString("myrepl> ") {
		t.Errorf("expected regex to match 'myrepl> ', got %v", re)
	}

	// User preset should override built-in (first match wins).
	re, _, err = Lookup("python3", presetsFile, true)
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if re == nil {
		t.Fatal("expected match for python3")
	}
	if !re.MatchString("CUSTOM>>> ") {
		t.Errorf("expected user preset to override builtin, got %v", re)
	}
}

func TestLookupMissingFile(t *testing.T) {
	// Missing file should not error — it's optional.
	re, _, err := Lookup("python3", "/nonexistent/presets", true)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	// Should still match via builtins.
	if re == nil {
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
	os.WriteFile(bad2, []byte("[invalid\tprompt$\n"), 0o644)
	_, err = LoadFile(bad2)
	if err == nil {
		t.Error("expected error for invalid command regex")
	}

	// Invalid prompt regex.
	bad3 := filepath.Join(dir, "bad3")
	os.WriteFile(bad3, []byte("valid$\t[invalid\n"), 0o644)
	_, err = LoadFile(bad3)
	if err == nil {
		t.Error("expected error for invalid prompt regex")
	}
}

func TestLoadFileCommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "presets")

	content := "# comment\n\n  # indented comment\n^test$\ttest> $\n\n"
	os.WriteFile(f, []byte(content), 0o644)

	entries, err := LoadFile(f)
	if err != nil {
		t.Fatalf("LoadFile error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if !entries[0].CommandRegex.MatchString("test") {
		t.Error("expected command regex to match 'test'")
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
		{"irb", "irb(main):001:0> "},
		{"sqlite3", "sqlite> "},
		{"mysql", "mysql> "},
	}

	for _, tt := range tests {
		t.Run(tt.command+"/"+tt.prompt, func(t *testing.T) {
			re, _, err := Lookup(tt.command, "", true)
			if err != nil {
				t.Fatalf("Lookup error: %v", err)
			}
			if re == nil {
				t.Fatalf("no preset for %q", tt.command)
			}
			if !re.MatchString(tt.prompt) {
				t.Errorf("regex %v did not match %q", re, tt.prompt)
			}
		})
	}
}
