// Package session manages ditty session state on disk.
//
// Each session has a Unix domain socket at ~/.ditty/sessions/NAME.sock and
// a metadata directory at ~/.ditty/sessions/NAME/. Session discovery works
// by scanning the sessions directory for socket files and checking liveness.
//
// Unix domain sockets have a hard 108-byte path limit (both macOS and Linux).
// To avoid exceeding this, socket operations chdir to the sessions directory
// and use relative paths (just "NAME.sock").
package session

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// socketMu serializes chdir-based socket operations. Go's os.Chdir changes
// the process-wide working directory, so concurrent socket operations must
// not interleave their chdir calls.
var socketMu sync.Mutex

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

// SocketPath returns the full path to a session's Unix domain socket.
// This is used for file operations (stat, remove) but not for
// net.Listen/net.Dial (use ListenSocket/DialSocket instead).
func SocketPath(name string) (string, error) {
	dir, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".sock"), nil
}

// socketName returns the short relative socket filename for a session.
func socketName(name string) string {
	return name + ".sock"
}

// ListenSocket creates a Unix domain socket listener for the named session.
// It uses chdir to keep the socket path short, avoiding the 108-byte limit.
func ListenSocket(name string) (net.Listener, error) {
	dir, err := EnsureBaseDir()
	if err != nil {
		return nil, err
	}

	socketMu.Lock()
	defer socketMu.Unlock()

	origDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}
	if err := os.Chdir(dir); err != nil {
		return nil, fmt.Errorf("chdir to sessions dir: %w", err)
	}
	ln, listenErr := net.Listen("unix", socketName(name))
	// Always restore the working directory, even if Listen failed.
	if err := os.Chdir(origDir); err != nil {
		if ln != nil {
			ln.Close()
		}
		return nil, fmt.Errorf("restore working directory: %w", err)
	}
	if listenErr != nil {
		return nil, fmt.Errorf("listen on %s: %w", socketName(name), listenErr)
	}
	return ln, nil
}

// DialSocket connects to a session's Unix domain socket.
// It uses chdir to keep the socket path short, avoiding the 108-byte limit.
func DialSocket(name string) (net.Conn, error) {
	dir, err := BaseDir()
	if err != nil {
		return nil, err
	}

	socketMu.Lock()
	defer socketMu.Unlock()

	origDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}
	if err := os.Chdir(dir); err != nil {
		return nil, fmt.Errorf("chdir to sessions dir: %w", err)
	}
	conn, dialErr := net.DialTimeout("unix", socketName(name),
		5*time.Second)
	if err := os.Chdir(origDir); err != nil {
		if conn != nil {
			conn.Close()
		}
		return nil, fmt.Errorf("restore working directory: %w", err)
	}
	return conn, dialErr
}

// maxPrefixLen is the maximum length of the command prefix in generated names.
const maxPrefixLen = 10

// GenerateName produces a session name from the command basename and a short
// random suffix. The command basename is lowercased, truncated to maxPrefixLen
// characters, and non-alphanumeric characters are replaced with hyphens. If
// command is empty, only the random suffix is used.
func GenerateName(command string) (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random name: %w", err)
	}
	suffix := fmt.Sprintf("%x", b)

	prefix := sanitizePrefix(filepath.Base(command))
	if prefix == "" {
		return suffix, nil
	}
	return prefix + "-" + suffix, nil
}

// sanitizePrefix extracts a clean prefix from a command basename. It
// lowercases, replaces non-alphanumeric characters with hyphens, collapses
// runs of hyphens, trims leading/trailing hyphens, and truncates to
// maxPrefixLen.
func sanitizePrefix(base string) string {
	var buf strings.Builder
	for _, c := range strings.ToLower(base) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			buf.WriteRune(c)
		} else {
			buf.WriteByte('-')
		}
	}

	// Collapse runs of hyphens and trim.
	s := buf.String()
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")

	if len(s) > maxPrefixLen {
		s = s[:maxPrefixLen]
		s = strings.TrimRight(s, "-")
	}
	return s
}

// IsAlive checks whether a session is alive by attempting to connect to its
// Unix domain socket with a short timeout.
func IsAlive(name string) bool {
	conn, err := DialSocket(name)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Metadata holds persistent information about a session, written to disk
// by the daemon at startup.
type Metadata struct {
	// PID is the daemon process ID.
	PID int `json:"pid"`
	// Command is the REPL program name.
	Command string `json:"command"`
	// Args are the REPL program arguments.
	Args []string `json:"args,omitempty"`
	// StartedAt is when the session was started.
	StartedAt time.Time `json:"started_at"`
}

// WriteMetadata writes session metadata to disk as NAME.json.
func WriteMetadata(name string, meta Metadata) error {
	dir, err := EnsureBaseDir()
	if err != nil {
		return err
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, name+".json"), data, 0o600)
}

// ReadMetadata reads session metadata from disk. Returns nil if not found.
func ReadMetadata(name string) (*Metadata, error) {
	dir, err := BaseDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}
	return &meta, nil
}

// Info holds metadata about a discovered session.
type Info struct {
	// Name is the session name.
	Name string
	// SocketPath is the full path to the session's Unix socket.
	SocketPath string
	// Alive indicates whether the session's daemon is reachable.
	Alive bool
	// Meta is the session metadata, if available.
	Meta *Metadata
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
		meta, _ := ReadMetadata(sessName)
		sessions = append(sessions, Info{
			Name:       sessName,
			SocketPath: filepath.Join(dir, name),
			Alive:      IsAlive(sessName),
			Meta:       meta,
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

// Cleanup removes a session's socket file and metadata file.
func Cleanup(name string) error {
	dir, err := BaseDir()
	if err != nil {
		return err
	}
	os.Remove(filepath.Join(dir, name+".sock"))
	os.Remove(filepath.Join(dir, name+".json"))
	return nil
}
