package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateName(t *testing.T) {
	name, err := GenerateName()
	if err != nil {
		t.Fatalf("GenerateName() error: %v", err)
	}
	if len(name) != 8 {
		t.Errorf("expected 8-char name, got %q (len %d)", name, len(name))
	}

	// Names should be unique.
	name2, err := GenerateName()
	if err != nil {
		t.Fatalf("GenerateName() error: %v", err)
	}
	if name == name2 {
		t.Errorf("expected unique names, got %q twice", name)
	}
}

func TestSocketPath(t *testing.T) {
	path, err := SocketPath("test-session")
	if err != nil {
		t.Fatalf("SocketPath() error: %v", err)
	}
	if filepath.Base(path) != "test-session.sock" {
		t.Errorf("expected test-session.sock, got %s", filepath.Base(path))
	}
}

func TestSetGetLast(t *testing.T) {
	// Use a temporary directory as home.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Initially, no last session.
	last, err := GetLast()
	if err != nil {
		t.Fatalf("GetLast() error: %v", err)
	}
	if last != "" {
		t.Errorf("expected empty last, got %q", last)
	}

	// Set and retrieve.
	if err := SetLast("my-session"); err != nil {
		t.Fatalf("SetLast() error: %v", err)
	}
	last, err = GetLast()
	if err != nil {
		t.Fatalf("GetLast() error: %v", err)
	}
	if last != "my-session" {
		t.Errorf("expected my-session, got %q", last)
	}
}

func TestListEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	sessions, err := List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected no sessions, got %d", len(sessions))
	}
}

func TestListWithSockets(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir, err := EnsureBaseDir()
	if err != nil {
		t.Fatalf("EnsureBaseDir() error: %v", err)
	}

	// Create a socket file that has a listener (alive), using
	// ListenSocket to handle the path length limit.
	ln, err := ListenSocket("alive")
	if err != nil {
		t.Fatalf("ListenSocket() error: %v", err)
	}
	defer ln.Close()

	// Create a stale socket file (no listener).
	stalePath := filepath.Join(dir, "stale.sock")
	if err := os.WriteFile(stalePath, nil, 0o600); err != nil {
		t.Fatalf("create stale socket: %v", err)
	}

	// Create a non-socket file (should be ignored).
	if err := os.WriteFile(filepath.Join(dir, "not-a-socket.txt"), nil, 0o600); err != nil {
		t.Fatalf("create non-socket: %v", err)
	}

	sessions, err := List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Find each session by name.
	found := map[string]Info{}
	for _, s := range sessions {
		found[s.Name] = s
	}

	alive, ok := found["alive"]
	if !ok {
		t.Fatal("alive not found")
	}
	if !alive.Alive {
		t.Error("expected alive to be alive")
	}

	stale, ok := found["stale"]
	if !ok {
		t.Fatal("stale not found")
	}
	if stale.Alive {
		t.Error("expected stale to be not alive")
	}
}

func TestListenDialSocket(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	ln, err := ListenSocket("roundtrip")
	if err != nil {
		t.Fatalf("ListenSocket() error: %v", err)
	}
	defer ln.Close()

	// DialSocket should connect.
	conn, err := DialSocket("roundtrip")
	if err != nil {
		t.Fatalf("DialSocket() error: %v", err)
	}
	conn.Close()

	// DialSocket to nonexistent should fail.
	_, err = DialSocket("nonexistent")
	if err == nil {
		t.Error("expected error dialing nonexistent socket")
	}
}

func TestCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir, err := EnsureBaseDir()
	if err != nil {
		t.Fatalf("EnsureBaseDir() error: %v", err)
	}

	sockPath := filepath.Join(dir, "cleanup-test.sock")
	if err := os.WriteFile(sockPath, nil, 0o600); err != nil {
		t.Fatalf("create socket: %v", err)
	}

	if err := Cleanup("cleanup-test"); err != nil {
		t.Fatalf("Cleanup() error: %v", err)
	}

	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Error("expected socket to be removed after cleanup")
	}

	// Cleanup of nonexistent session should not error.
	if err := Cleanup("nonexistent"); err != nil {
		t.Errorf("Cleanup(nonexistent) error: %v", err)
	}
}
