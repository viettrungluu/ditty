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
      attach.go                   # ditty attach
      daemon.go                   # hidden _daemon subcommand
    daemon/                       # daemon process logic
      daemon.go                   # main loop: pty, REPL lifecycle, echo stripping
      server.go                   # Unix socket listener, client handler
    dlog/                         # verbose/debug logging
      dlog.go
    preset/                       # built-in prompt presets
      preset.go
    protocol/                     # client-daemon wire protocol
      protocol.go                 # message types, framing, read/write helpers
    prompt/                       # prompt detection
      detect.go                   # idle timeout + regex detector (ANSI-aware)
    ringbuf/                      # bounded ring buffer
      ringbuf.go
    session/                      # session directory management
      session.go                  # paths, naming, discovery, liveness, metadata
  integration/
    integration_test.go           # end-to-end tests
  scripts/
    smoke-test.sh                 # 29 smoke tests
  .github/workflows/
    ci.yml                        # CI on Ubuntu + macOS
```

## Dependencies

- `github.com/creack/pty/v2` — pty allocation
- `github.com/spf13/cobra` — CLI framework

## What's Done

All items below are implemented, tested, committed, and pushed.

### MVP
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

### Post-MVP enhancements
- [x] `--verbose` / `-v` flag for daemon debug logging
- [x] Graceful stop: SIGTERM first, SIGKILL escalation after 5s
- [x] Install/build instructions in README
- [x] Socket path length fix (chdir + relative paths to avoid 108-byte limit)
- [x] Clean error messages for missing/stale sessions
- [x] `--idle-timeout` flag on `ditty start`
- [x] `--echo` flag (default: on) with output-side echo stripping
- [x] `--buffer-size` flag for ring buffer configuration
- [x] Session metadata on disk (PID, command, start time; shown in `ditty list`)
- [x] `--prompt=REGEX` for precise, zero-latency prompt detection
- [x] Smoke test script (`scripts/smoke-test.sh`, 29 tests)
- [x] TERM inherited from environment (configurable via `TERM=... ditty start`)
- [x] `--no-pty` flag for pipe mode
- [x] `--suspend` flag (SIGSTOP/SIGCONT between commands)
- [x] `--multi` flag on continue (each arg as a separate line)
- [x] `ditty attach` (interactive line-by-line session)
- [x] Built-in presets (python, node, gdb, lldb, irb, rails, sqlite3, mysql, psql, lua, R)
- [x] Presets redesigned: flag-based format (presets can set any start flag, not just prompt)
- [x] `--no-builtin-presets` flag to disable built-in presets
- [x] `--env` flag (repeatable) to set environment variables for the child
- [x] ANSI escape stripping in regex prompt detection
- [x] Terminal state reset after streaming (fixes irb cruft)
- [x] `--no-terminal-reset` flag
- [x] Program flag passthrough fix (SetInterspersed)
- [x] GitHub Actions CI (Ubuntu + macOS)
- [x] Release workflow (cross-compile, .tar.gz, checksums)
- [x] Homebrew tap (viettrungluu/tap/ditty)

## What's Left (future work)

- **`ditty history`**: scrollback of past interactions
- **Verbose daemon log file**: persist logs to a file even without `-v`
