// Package session manages ditty session state on disk.
//
// Each session has a Unix domain socket at ~/.ditty/sessions/NAME.sock and
// a metadata directory at ~/.ditty/sessions/NAME/. Session discovery works
// by scanning the sessions directory for socket files and checking liveness.
package session

import (
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BaseDir returns the base directory for all ditty sessions.
// Defaults to ~/.ditty/sessions.
func BaseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(home, ".ditty", "sessions"), nil
}

// EnsureBaseDir creates the sessions base directory if it doesn't exist.
func EnsureBaseDir() (string, error) {
	dir, err := BaseDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create sessions directory: %w", err)
	}
	return dir, nil
}

// SocketPath returns the path to a session's Unix domain socket.
func SocketPath(name string) (string, error) {
	dir, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".sock"), nil
}

// GenerateName produces a short random session name.
// The name is 8 hex characters (4 random bytes).
func GenerateName() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random name: %w", err)
	}
	return fmt.Sprintf("%x", b), nil
}

// IsAlive checks whether a session is alive by attempting to connect to its
// Unix domain socket with a short timeout.
func IsAlive(name string) bool {
	sockPath, err := SocketPath(name)
	if err != nil {
		return false
	}
	conn, err := net.DialTimeout("unix", sockPath, 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Info holds metadata about a discovered session.
type Info struct {
	// Name is the session name.
	Name string
	// SocketPath is the full path to the session's Unix socket.
	SocketPath string
	// Alive indicates whether the session's daemon is reachable.
	Alive bool
}

// List discovers all sessions by scanning the sessions directory for socket
// files. It checks liveness for each discovered session.
func List() ([]Info, error) {
	dir, err := BaseDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions directory: %w", err)
	}

	var sessions []Info
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sock") {
			continue
		}
		sessName := strings.TrimSuffix(name, ".sock")
		sessions = append(sessions, Info{
			Name:       sessName,
			SocketPath: filepath.Join(dir, name),
			Alive:      IsAlive(sessName),
		})
	}
	return sessions, nil
}

// SetLast records the given name as the last-used session.
func SetLast(name string) error {
	dir, err := EnsureBaseDir()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".last"), []byte(name), 0o600)
}

// GetLast returns the name of the last-used session, or empty string if none.
func GetLast() (string, error) {
	dir, err := BaseDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, ".last"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read last session: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// Cleanup removes a session's socket file and metadata directory.
func Cleanup(name string) error {
	sockPath, err := SocketPath(name)
	if err != nil {
		return err
	}
	// Remove socket file (ignore if already gone).
	os.Remove(sockPath)
	return nil
}
