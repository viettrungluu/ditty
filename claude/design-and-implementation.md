# ditty — Design and Implementation

## Overview

ditty converts line-interactive programs (REPLs, debuggers, etc.) into command-line programs. It runs an interactive program in the background, manages its I/O through a daemon, and lets users send input and receive output through simple CLI commands.

## Architecture

### Per-session daemon model

Each `ditty start` spawns an independent background daemon process. There is no central master daemon — sessions are fully isolated.

A daemon:
1. Allocates a pseudoterminal (pty) for the child program.
2. Starts the child process attached to the pty.
3. Listens on a Unix domain socket (`~/.ditty/sessions/NAME.sock`).
4. Continuously reads from the pty and either streams output to a connected client or buffers it in a ring buffer.
5. Cleans up (socket, metadata files) when the child exits.

The daemon is launched by re-exec'ing the ditty binary with a hidden `_daemon` subcommand. This avoids the complexity of `fork()` in Go while keeping the daemon as a single static binary.

### Client-daemon protocol

Communication uses a simple binary-framed protocol over Unix domain sockets:

```
[1 byte type][4 bytes big-endian payload length][payload]
```

Message types:
- **Client → Daemon**: `Input`, `Stop`, `Kill`, `Interrupt`
- **Daemon → Client**: `Output`, `BufferedOutput`, `PromptDetected`, `Exited`, `Error`

Only one client can be connected at a time (the daemon serves connections serially).

### Why a pty

Most REPLs detect whether stdin is a TTY and change behavior accordingly. Python suppresses the `>>> ` prompt when stdin isn't a TTY. gdb/lldb may disable line editing or switch to batch mode. ditty allocates a pty by default so programs behave normally.

A `--no-pty` flag is available for programs that work fine with pipes (e.g., `cat`, simple line processors). In pipe mode, stdin/stdout/stderr are connected via pipes, and interrupts are sent via SIGINT rather than `\x03`.

## Prompt Detection

The central challenge: ditty needs to know when the program has finished producing output and is waiting for input. There is no universal "waiting for input" signal from a child process.

### Strategy 1: Regex matching (preferred)

When a prompt regex is available (via `--prompt` or auto-detected preset), the detector accumulates output and checks it against the regex. ANSI escape sequences are stripped before matching, since some programs (notably Python 3.13+) wrap prompts in color codes and bracketed paste mode sequences.

Regex matching fires as soon as the pattern matches, with only a 10ms debounce — no idle timeout delay.

### Strategy 2: Idle timeout (fallback)

When no regex is available, the detector waits for output to go silent for a configurable duration (default 200ms), then checks if the last byte is not `\n`. REPL prompts are nearly always partial lines (`>>> `, `(gdb) `, etc.), while normal output almost always ends with `\n`. This heuristic works for most programs with zero configuration, at the cost of added latency.

### Built-in presets

ditty ships with prompt regexes for common programs: python, node, gdb, lldb, irb, sqlite3, mysql, psql, lua, R. The program name is matched after stripping version suffixes (e.g., `python3.12` → `python`). Presets are applied automatically unless `--prompt` is set or `--no-preset` is passed.

## Background Output

Between `ditty continue` calls, no client is connected. The daemon must keep reading from the pty (otherwise the kernel buffer fills and the child blocks). Output is stored in a bounded ring buffer (default 1MB). When the next client connects, buffered output is delivered first.

If the ring buffer overflows, the oldest output is dropped — the most recent output is almost always what matters.

An alternative is `--suspend`, which sends SIGSTOP to the child between commands. This prevents any background output but some programs react poorly to suspension.

## Socket Path Length

Unix domain sockets have a hard 108-byte path limit (both macOS and Linux). With long `$HOME` paths, `~/.ditty/sessions/NAME.sock` can exceed this. ditty works around this by `chdir`-ing to the sessions directory and using relative paths (`NAME.sock`) for `net.Listen` and `net.Dial`. A process-wide mutex serializes these operations since `os.Chdir` is process-global.

## Echo Control

When a pty is in use, the terminal line discipline echoes input. This means `ditty continue 'print(42)'` shows `print(42)` in the output. The `--echo=false` flag strips this by tracking the expected echo text and removing it from the output stream. This is done at the output level (not via termios) because programs like Python's readline actively manage the ECHO flag and override any changes.

## Signal Handling

- **Daemon**: ignores SIGINT/SIGTERM — it stays alive as long as the child is alive.
- **`ditty continue`**: catches SIGINT and forwards it to the child by writing `\x03` to the pty (or sending SIGINT in pipe mode).
- **`ditty stop`**: sends SIGTERM to the child, with SIGKILL escalation after 5 seconds.

## Session State on Disk

- `~/.ditty/sessions/NAME.sock` — Unix domain socket for the daemon.
- `~/.ditty/sessions/NAME.json` — metadata (PID, command, args, start time).
- `~/.ditty/sessions/.last` — name of the last-used session.

All files are cleaned up when the daemon exits.

## Key Implementation Decisions

1. **Go** as the implementation language: excellent pty support (`creack/pty`), single static binary, fast startup (important since `ditty continue` is invoked repeatedly), goroutines for clean daemon logic.

2. **Cobra** for CLI: handles multi-level flag parsing, subcommand routing, and help text generation.

3. **Re-exec for daemon**: the daemon runs as the same binary with a hidden `_daemon` subcommand, avoiding fork complexity in Go.

4. **Serial client handling**: only one client at a time (no multiplexing). This is correct — one REPL, one operator.

5. **Output-side echo stripping** (not termios): programs like Python's readline re-enable ECHO after every line, making termios-based suppression unreliable.

6. **ANSI stripping for regex matching**: Python 3.13+ wraps prompts in ANSI color and bracketed paste mode sequences. Stripping these before regex matching makes presets work across Python versions.
