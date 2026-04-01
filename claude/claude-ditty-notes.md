# ditty — Design Notes

ditty converts line-interactive programs (REPLs, debuggers, etc.) into command-line programs.

## Core Commands

### `ditty start`

```
ditty start [--name=NAME] PROGRAM [ARGS...]
```

- Launches the given interactive program (e.g., `python3`, `gdb`, `lldb`) in the background.
- Streams the program's initial output (stdout/stderr) to the caller's stdout until the first prompt appears (e.g., `>>> `).
- Then returns control to the shell.
- `--name=NAME` gives the session a name for later reference. If omitted, a name is generated and printed.

### `ditty continue`

```
ditty continue [--name=NAME] 'INPUT'
```

- Sends the given input string to the named background session's REPL.
- Streams the REPL's output until the next prompt appears, then returns control.
- If `--name` is omitted, defaults to the last-used session.

### `ditty stop` / `ditty kill`

- `ditty stop` — gracefully terminates the session (send EOF, wait for exit).
- `ditty kill` — sends SIGTERM, then SIGKILL after a timeout.

### `ditty list` / `ditty ls`

- Lists active sessions.

## Signal Handling

- When `ditty continue` is running and the user hits Ctrl-C, ditty catches SIGINT and forwards it to the REPL by writing `\x03` to the pty (faithful to real terminal behavior).

## Architecture

### One daemon per session (no master daemon)

Each `ditty start` spawns its own background daemon process that:
- Allocates a pty for the REPL (so the REPL thinks it's talking to a real terminal).
- Holds the REPL process and its pty.
- Listens on a Unix domain socket (e.g., `~/.ditty/sessions/NAME.sock`).

`ditty continue` is a thin client: connects to the socket, sends input, reads output, disconnects.

Benefits of per-session daemons:
- No single point of failure.
- Simpler code — no multiplexing.
- Sessions are fully independent.
- Natural cleanup — when the REPL exits, the daemon exits.

`ditty list` discovers sessions by scanning `~/.ditty/sessions/` for socket files and checking liveness.

### Why a pty (not pipes)

Most REPLs detect whether stdin is a TTY and change behavior:
- Python suppresses the `>>> ` prompt when stdin isn't a TTY.
- gdb/lldb may disable line editing, suppress prompts, or switch to batch mode.

ditty allocates a pseudoterminal so the REPL behaves normally. A `--no-pty` flag could be offered for programs that work fine with pipes, but pty-by-default is correct.

### Session state on disk

- Socket: `~/.ditty/sessions/NAME.sock`
- Metadata (PID, original command, etc.): `~/.ditty/sessions/NAME/`

## Implementation Language

**Go** is the best fit:
- Excellent pty support (`creack/pty`).
- Single static binary — important for a CLI tool.
- Unix socket and signal handling are straightforward.
- Goroutines make daemon logic clean.
- Fast startup — matters since `ditty continue` is invoked repeatedly.

## Prompt Detection

The central challenge: ditty needs to know when the REPL has finished producing output and is waiting for input, so it can return control to the caller. There is no universal "waiting for input" signal from a child process.

### Approaches considered

**1. Idle timeout + incomplete line (default)**

Wait for output to go silent for a configurable duration (e.g., 200ms), then check whether the last chunk of output ends without a trailing newline. REPL prompts are nearly always partial lines (`>>> `, `(gdb) `, `mysql> `, etc.), while normal program output almost always ends with `\n`.

- Pros: Works for any REPL with zero configuration.
- Cons: Adds latency (the timeout) to every round-trip. A program that pauses mid-line during output could trigger a false positive. A prompt that ends with `\n` (very rare) would be missed.

**2. Explicit prompt pattern (`--prompt=REGEX`)**

The user supplies a regex that matches the prompt. ditty watches output and returns as soon as the pattern matches at the end of the accumulated output.

- Pros: Precise, no timeout delay.
- Cons: Requires per-REPL configuration. Must handle changing prompts (e.g., Python `>>> ` vs. `... `, or debugger prompts that include state). The regex applies to raw pty output, which may contain ANSI escape codes — users may need to account for that.

**3. Built-in presets**

Ship known prompt patterns for popular REPLs (python, node, gdb, lldb, etc.), auto-selected from the program name passed to `ditty start`.

- Pros: Zero-config for common cases with no timeout penalty.
- Cons: Maintenance burden. Doesn't cover niche programs.

**4. Process state inspection**

Detect that the child process is blocked in `read()` — via procfs, ptrace, or kevent.

- Pros: Ground truth; no heuristics.
- Cons: Platform-specific, invasive, may require elevated privileges, fragile across OS versions. Not practical as a default.

### Recommendation

Layer the approaches:

1. **Default**: idle timeout + no trailing newline. The heuristic is strong — false positives (output pausing mid-line for 200ms+) are rare in practice, and false negatives (prompts ending with `\n`) are near zero.
2. **Override**: `--prompt=REGEX` for users who want precision and zero latency.
3. **Nice-to-have**: built-in presets for common REPLs, auto-detected from the command name.

The idle timeout should be configurable (`--idle-timeout=MS`, default 200ms) since some REPLs are slow to emit their prompt (e.g., after loading a large project) and some use cases demand snappier response.

## Background Output

Between `ditty continue` calls (or after `ditty start` returns), no client is connected to the daemon. The REPL may still produce output — async events, breakpoints firing, timers, background jobs, late output after prompt detection returned.

The daemon holds the master side of the pty. If it stops reading, the pty kernel buffer fills (~4KB on Linux, ~8KB on macOS) and the child blocks on `write()`. So the daemon **must** keep reading at all times. The question is what to do with the bytes.

### Approaches considered

**1. Bounded ring buffer in the daemon (default)**

Daemon always reads from the pty and stores output in a capped in-memory ring buffer (e.g., 1MB). When the next `ditty continue` connects, it receives buffered output first, then live output.

- Pros: Child never blocks. Between-command output is usually small, so the buffer rarely overflows. If it does, oldest output is dropped (least valuable). Clean mental model — the user sees everything the REPL printed, as if they'd been watching.
- Cons: Potential memory usage, though bounded. Overflow drops output silently.

**2. Buffer to disk**

Spill to a file after a memory threshold.

- Pros: Handles arbitrary output volume.
- Cons: Added complexity, disk I/O, cleanup. Overkill for the common case.

**3. Suspend the child (`SIGSTOP` / `SIGCONT`)**

Freeze the child after prompt detection returns, resume on next `ditty continue`.

- Pros: Guarantees no background output to manage.
- Cons: Many programs react poorly — terminal state corruption, broken network connections, timer drift, signal handlers that interpret `SIGCONT` as a resume-from-background event. Shells with job control may intercept it. Completely wrong for programs that should keep running (e.g., a server with a REPL).

**4. Let OS buffers fill (do nothing)**

Don't read the pty master when no client is connected.

- Pros: Simplest possible implementation.
- Cons: Once the ~4KB buffer fills, the child blocks on `write()`. This is accidental, uncontrolled suspension — worse than `SIGSTOP` because the child may be blocked mid-write, mid-line, or holding locks.

**5. Read and discard**

Daemon reads (keeping the child unblocked) but throws away output when no client is connected.

- Pros: Simple. Child never blocks.
- Cons: Output is silently lost.

### Recommendation

Default to **(1) bounded ring buffer**. The child never blocks, the common case (little or no between-command output) costs almost nothing, and overflow degrades gracefully.

Offer `--suspend` as an opt-in flag for users who know their program tolerates `SIGSTOP`/`SIGCONT` and want a hard guarantee that nothing happens between commands.

The daemon should **not** run prompt detection on buffered output while no client is connected. Prompt detection only matters during an active `ditty continue`. Buffered bytes are delivered as-is when the next client connects; prompt detection runs on whatever comes after the new input is sent.
