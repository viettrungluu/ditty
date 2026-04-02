# ditty Implementation Plan

## Context

ditty converts line-interactive programs (REPLs, debuggers) into CLI programs. Design notes are in `claude/claude-ditty-notes.md`.

## Package Structure

```
ditty/
  go.mod                          # github.com/viettrungluu/ditty
  main.go                         # cobra root command setup
  internal/
    cmd/                          # cobra subcommand definitions
      root.go                     # root command, --verbose flag
      start.go                    # ditty start
      continue.go                 # ditty continue
      stop.go                     # ditty stop
      kill.go                     # ditty kill
      list.go                     # ditty list / ditty ls
      daemon.go                   # hidden _daemon subcommand
    daemon/                       # daemon process logic
      daemon.go                   # main loop: pty, REPL lifecycle, echo stripping
      server.go                   # Unix socket listener, client handler
    dlog/                         # verbose/debug logging
      dlog.go
    protocol/                     # client-daemon wire protocol
      protocol.go                 # message types, framing, read/write helpers
    prompt/                       # prompt detection
      detect.go                   # idle timeout + regex detector
    ringbuf/                      # bounded ring buffer
      ringbuf.go
    session/                      # session directory management
      session.go                  # paths, naming, discovery, liveness, metadata
  integration/
    integration_test.go           # end-to-end tests
```

## Dependencies

- `github.com/creack/pty/v2` — pty allocation
- `github.com/spf13/cobra` — CLI framework

## What's Done (MVP + enhancements)

All items below are implemented, tested, committed, and pushed.

### MVP (Steps 1-10)
- [x] Project scaffolding (go module, cobra root command)
- [x] Session management (paths, naming, discovery, liveness checks)
- [x] Ring buffer for background output buffering
- [x] Prompt detection (idle timeout + no-trailing-newline heuristic)
- [x] Wire protocol (binary-framed messages)
- [x] Daemon (pty management, socket server, hidden _daemon subcommand)
- [x] CLI: `ditty start` and `ditty continue`
- [x] CLI: `ditty stop`, `ditty kill`, `ditty list`/`ls`
- [x] Signal handling (SIGINT → \x03 forwarding)
- [x] Integration tests (python3 end-to-end)

### Post-MVP enhancements (done)
- [x] `--verbose` / `-v` flag for daemon debug logging
- [x] Graceful stop sends SIGTERM first (not pty close), with SIGKILL escalation
- [x] Install/build instructions in README
- [x] Socket path length fix (chdir + relative paths to avoid 108-byte limit)
- [x] Clean error messages for missing/stale sessions
- [x] `--idle-timeout` flag on `ditty start`
- [x] `--echo` flag (default: off) — strips echoed input from output
- [x] `--buffer-size` flag for ring buffer configuration
- [x] Session metadata on disk (PID, command, start time; shown in `ditty list`)
- [x] `--prompt=REGEX` for precise, zero-latency prompt detection
- [x] `--echo` default changed to true (echo on)
- [x] Smoke test script (`scripts/smoke-test.sh`)
- [x] TERM inherited from environment (configurable via `TERM=... ditty start`)

## What's Left (future work)

### Features
- **Built-in presets**: Auto-detect prompt patterns for python, node, gdb, lldb, etc.
- **`--multi` flag on continue**: Each positional arg is sent as a separate line, waiting for the prompt between each. Useful for multi-line sequences (imports, function defs, etc.)
- **`--suspend` flag**: SIGSTOP/SIGCONT between commands for programs that tolerate it
- **`--no-pty` flag**: Pipe mode for programs that don't need a pty
- **Reconnect / attach**: `ditty attach` to get a live interactive session with the REPL
- **Scrollback / history**: `ditty history --name=NAME` to see past interactions
- **Verbose daemon log file**: Write daemon logs to a file in the session dir so they're available even when not started with `-v`
