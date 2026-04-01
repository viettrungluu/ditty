// Package integration contains end-to-end tests for ditty.
//
// These tests require python3 to be available in PATH.
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// dittyBin is the path to the compiled ditty binary.
var dittyBin string

func TestMain(m *testing.M) {
	// Build ditty into a temporary location.
	tmp, err := os.MkdirTemp("", "ditty-integration-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)

	dittyBin = filepath.Join(tmp, "ditty")
	cmd := exec.Command("go", "build", "-o", dittyBin, "..")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("failed to build ditty: " + err.Error())
	}

	os.Exit(m.Run())
}

// shortTempDir creates a short temporary directory under /tmp to keep Unix
// socket paths within the 108-byte limit on macOS.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "dt-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// runDitty runs ditty with the given arguments and returns stdout.
// It uses a temporary HOME to isolate sessions.
func runDitty(t *testing.T, home string, timeout time.Duration, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(dittyBin, args...)
	cmd.Env = append(os.Environ(), "HOME="+home)

	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return "", err
	}
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return out.String(), err
	case <-time.After(timeout):
		cmd.Process.Kill()
		<-done
		t.Fatalf("ditty %v timed out after %v\noutput: %s",
			args, timeout, out.String())
		return "", nil // unreachable
	}
}

func TestStartContinueStop(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found in PATH")
	}

	home := shortTempDir(t)
	timeout := 10 * time.Second

	// Start a Python session.
	out, err := runDitty(t, home, timeout, "start", "--name=inttest", "python3")
	if err != nil {
		t.Fatalf("start failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, ">>>") {
		t.Errorf("start output should contain prompt, got: %s", out)
	}

	// Send a command.
	out, err = runDitty(t, home, timeout, "continue", "--name=inttest",
		"print(40 + 2)")
	if err != nil {
		t.Fatalf("continue failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "42") {
		t.Errorf("continue output should contain 42, got: %s", out)
	}

	// State should persist across continues.
	out, err = runDitty(t, home, timeout, "continue", "--name=inttest", "x = 99")
	if err != nil {
		t.Fatalf("continue (assign) failed: %v\noutput: %s", err, out)
	}
	out, err = runDitty(t, home, timeout, "continue", "--name=inttest",
		"print(x)")
	if err != nil {
		t.Fatalf("continue (print) failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "99") {
		t.Errorf("expected 99, got: %s", out)
	}

	// List should show the session.
	out, err = runDitty(t, home, timeout, "list")
	if err != nil {
		t.Fatalf("list failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "inttest") {
		t.Errorf("list should contain session name, got: %s", out)
	}
	if !strings.Contains(out, "alive") {
		t.Errorf("list should show session as alive, got: %s", out)
	}

	// Stop the session.
	out, err = runDitty(t, home, timeout, "stop", "--name=inttest")
	if err != nil {
		t.Fatalf("stop failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "stopped") {
		t.Errorf("stop output should confirm stop, got: %s", out)
	}

	// List should be empty after stop.
	time.Sleep(500 * time.Millisecond) // give daemon time to clean up
	out, err = runDitty(t, home, timeout, "list")
	if err != nil {
		t.Fatalf("list after stop failed: %v\noutput: %s", err, out)
	}
	if strings.Contains(out, "inttest") {
		t.Errorf("list should not contain session after stop, got: %s", out)
	}
}

func TestKill(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found in PATH")
	}

	home := shortTempDir(t)
	timeout := 10 * time.Second

	// Start and then kill.
	out, err := runDitty(t, home, timeout, "start", "--name=killme", "python3")
	if err != nil {
		t.Fatalf("start failed: %v\noutput: %s", err, out)
	}

	out, err = runDitty(t, home, timeout, "kill", "--name=killme")
	if err != nil {
		t.Fatalf("kill failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "killed") {
		t.Errorf("kill output should confirm kill, got: %s", out)
	}

	// Session should be gone.
	time.Sleep(500 * time.Millisecond)
	out, err = runDitty(t, home, timeout, "list")
	if err != nil {
		t.Fatalf("list after kill failed: %v\noutput: %s", err, out)
	}
	if strings.Contains(out, "killme") {
		t.Errorf("list should not contain session after kill, got: %s", out)
	}
}

func TestLastUsedSession(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found in PATH")
	}

	home := shortTempDir(t)
	timeout := 10 * time.Second

	// Start a session (sets it as last-used).
	_, err := runDitty(t, home, timeout, "start", "--name=last", "python3")
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Continue without --name should use last-used.
	out, err := runDitty(t, home, timeout, "continue", "print(123)")
	if err != nil {
		t.Fatalf("continue failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "123") {
		t.Errorf("expected 123, got: %s", out)
	}

	// Clean up.
	runDitty(t, home, timeout, "kill", "--name=last")
}

func TestAutoGenerateName(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found in PATH")
	}

	home := shortTempDir(t)
	timeout := 10 * time.Second

	// Start without --name should generate and print a name.
	out, err := runDitty(t, home, timeout, "start", "python3")
	if err != nil {
		t.Fatalf("start failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "session") {
		t.Errorf("start output should mention session name, got: %s", out)
	}

	// List should show the auto-generated session.
	out, err = runDitty(t, home, timeout, "list")
	if err != nil {
		t.Fatalf("list failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "alive") {
		t.Errorf("expected alive session, got: %s", out)
	}

	// Clean up by killing (use continue without --name to verify it's
	// the last-used session, then kill).
	runDitty(t, home, timeout, "kill")
}
