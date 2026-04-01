# ditty MVP Implementation Plan

## Context

ditty converts line-interactive programs (REPLs, debuggers) into CLI programs. Design notes are in `claude/claude-ditty-notes.md`. This plan covers a functional MVP — enough to `start` a REPL, `continue` sending commands, `stop`/`kill` it, and `list` sessions. Follow-up items (prompt regex, presets, --suspend, etc.) are listed at the end for future planning.

## Package Structure

```
ditty/
  go.mod                          # github.com/viettrungluu/ditty
  main.go                         # cobra root command setup
  internal/
    cmd/                          # cobra subcommand definitions
      root.go                     # root command, common flags
      start.go                    # ditty start
      continue.go                 # ditty continue
      stop.go                     # ditty stop
      kill.go                     # ditty kill
      list.go                     # ditty list / ditty ls
    daemon/                       # daemon process logic
      daemon.go                   # main loop: pty management, REPL lifecycle
      server.go                   # Unix socket listener, client handler
    protocol/                     # client-daemon wire protocol
      protocol.go                 # message types, framing, read/write helpers
    prompt/                       # prompt detection
      detect.go                   # idle timeout + no-trailing-newline detector
    ringbuf/                      # bounded ring buffer
      ringbuf.go                  # ring buffer implementation
    session/                      # session directory management
      session.go                  # paths, naming, discovery, liveness checks
```

## Dependencies

- `github.com/creack/pty` — pty allocation
- `github.com/spf13/cobra` — CLI framework

## Commit Strategy

Each step below is one self-contained commit. Every commit should compile (`go build ./...`) and pass tests (`go test ./...`). Tests are written alongside the code they test, in the same commit — not deferred to a later step.

## Implementation Steps (MVP)

### Step 1: Project scaffolding
- `go mod init`, add dependencies (`creack/pty`, `cobra`)
- `main.go` with cobra root command
- `internal/cmd/root.go` — root command, version flag
- Commit: compiles, `ditty --help` works

### Step 2: Session management
- `internal/session/session.go` — session dir paths (`~/.ditty/sessions/`), name generation (short random), discovery (scan dir for sockets), liveness check (dial socket)
- `internal/session/session_test.go` — tests for naming, path construction, discovery
- Commit: compiles, tests pass

### Step 3: Ring buffer
- `internal/ringbuf/ringbuf.go` — fixed-capacity byte ring buffer
  - `Write([]byte)` — appends, overwrites oldest if full
  - `ReadAll() []byte` — returns buffered content and clears
  - Capacity configurable, default 1MB
- `internal/ringbuf/ringbuf_test.go` — tests for write, read, overflow, edge cases
- Commit: compiles, tests pass

### Step 4: Prompt detection
- `internal/prompt/detect.go` — idle timeout detector
  - Receives output chunks, resets a timer on each chunk
  - When timer fires: checks if accumulated output since last input ends without `\n`
  - If yes → prompt detected; if no → reset and keep waiting
  - Configurable idle timeout (default 200ms)
- `internal/prompt/detect_test.go` — tests for detection, timeout, edge cases
- Commit: compiles, tests pass

### Step 5: Wire protocol
- `internal/protocol/protocol.go` — simple binary framing
  - Frame: `[1 byte type][4 bytes big-endian length][payload]`
  - Client → Daemon message types: `Input`, `Stop`, `Kill`
  - Daemon → Client message types: `Output`, `PromptDetected`, `Exited` (with exit code), `Error`
  - Helper functions: `WriteMessage(conn, msg)`, `ReadMessage(conn) msg`
- `internal/protocol/protocol_test.go` — round-trip encode/decode tests
- Commit: compiles, tests pass

### Step 6: Daemon
- `internal/daemon/daemon.go` — daemon main loop
  - Allocates pty, starts REPL child process
  - Goroutine reads from pty master continuously → ring buffer (when no client) or streams to client (when connected)
  - Monitors child process exit
  - Cleans up socket and session dir on exit
- `internal/daemon/server.go` — Unix socket server
  - Listens on `~/.ditty/sessions/NAME.sock`
  - Accepts one client at a time (serial, not concurrent — one REPL, one operator)
  - On `Input` message: writes to pty, streams output back until prompt detected
  - On `Stop`: sends EOF to pty, waits for child exit, responds with `Exited`
  - On `Kill`: sends SIGTERM, waits briefly, SIGKILL if needed, responds with `Exited`
- Hidden `_daemon` subcommand in `internal/cmd/daemon.go`
- Commit: compiles, daemon can be launched manually for testing

### Step 7: CLI commands — start & continue
- `internal/cmd/start.go` — `--name` flag, launches daemon via re-exec, waits for socket, streams initial output
- `internal/cmd/continue.go` — `--name` flag (default: last-used session), sends input, streams output
  - "Last-used session" tracked via a symlink or small file `~/.ditty/sessions/.last`
- Commit: `ditty start python3` and `ditty continue` work end-to-end

### Step 8: CLI commands — stop, kill, list
- `internal/cmd/stop.go` — `--name` flag, sends Stop message
- `internal/cmd/kill.go` — `--name` flag, sends Kill message
- `internal/cmd/list.go` — scans session dir, checks liveness, prints table (alias `ls`)
- Commit: full command set works

### Step 9: Signal handling
- `ditty continue` catches SIGINT and sends `\x03` to the daemon (new protocol message type `Interrupt`, or just send `Input` with `\x03`)
- Daemon writes `\x03` to pty master
- Commit: Ctrl-C during `ditty continue` forwards to REPL

### Step 10: Integration tests
- Integration test using python3: start session, send commands, verify output, stop session
- Verify cleanup (no leftover sockets/dirs)
- Commit: integration tests pass, 90%+ coverage

## Verification

1. `go build ./...` compiles cleanly
2. `go test ./...` passes with 90%+ coverage
3. Manual smoke test:
   ```
   ditty start --name=test python3
   ditty continue --name=test 'print("hello world")'
   # should print: hello world
   ditty list
   # should show "test" session
   ditty stop --name=test
   ditty list
   # should show no sessions
   ```
4. `go vet ./...` and `golint` clean

## Follow-up Items (post-MVP)

### Near-term fixes
- **Socket path length**: Unix domain sockets have a hard 108-byte path limit (both macOS and Linux). Workaround: `chdir` to `~/.ditty/sessions/` and use relative paths (just `NAME.sock`) when creating/connecting to sockets. This keeps paths short regardless of `$HOME` length.
- **Pty echo control**: `continue` output includes the echoed input, which is noise for scripting. Add a `--echo`/`--no-echo` flag on `ditty start` (default: echo off) to control the pty's echo setting.
- **Clean error for missing sessions**: `continue`/`stop`/`kill` on a nonexistent session gives a raw "connection refused" error. Should check for socket existence first and give a clear "session not found" message.

### Features
- **`--prompt=REGEX`**: Explicit prompt pattern for precise, zero-latency detection
- **Built-in presets**: Auto-detect prompt patterns for python, node, gdb, lldb, etc.
- **`--suspend` flag**: SIGSTOP/SIGCONT between commands for programs that tolerate it
- **`--no-pty` flag**: Pipe mode for programs that don't need a pty
- **`--idle-timeout` flag**: Already designed for, just needs CLI plumbing
- **Ring buffer size config**: `--buffer-size` flag on `ditty start`
- **Session metadata on disk**: Store PID, command, start time in session dir for richer `ditty list` output
- **Reconnect / attach**: `ditty attach` to get a live interactive session with the REPL
- **Configurable TERM**: Set the TERM environment variable for the pty (default `xterm-256color` or similar)
- **Scrollback / history**: `ditty history --name=NAME` to see past interactions
- **Verbose daemon log file**: Write daemon logs to a file in the session dir so they're available even when not started with `-v`
- **Install instructions**: Add `go install` / build instructions to README
