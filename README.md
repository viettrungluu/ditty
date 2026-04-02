# ditty

[![CI](https://github.com/viettrungluu/ditty/actions/workflows/ci.yml/badge.svg)](https://github.com/viettrungluu/ditty/actions/workflows/ci.yml)

**ditty** ("de-TTY") converts line-interactive programs (REPLs, debuggers, etc.) into command-line programs.

It runs an interactive program in the background and lets you send input and receive output through simple CLI commands, making it easy to script interactions with programs like `python3`, `gdb`, `lldb`, and others.

## Install

### Homebrew (macOS/Linux)

```
brew install viettrungluu/tap/ditty
```

### Go

```
go install github.com/viettrungluu/ditty@latest
```

### Pre-built binaries

Download from the [releases page](https://github.com/viettrungluu/ditty/releases). On macOS, you'll need to remove the quarantine attribute after extracting:

```
tar xzf ditty-darwin-arm64.tar.gz
xattr -d com.apple.quarantine ditty-darwin-arm64/ditty
```

### Build from source

```
git clone https://github.com/viettrungluu/ditty.git
cd ditty
go build -o ditty .
```

## Quick start

```bash
# Start a Python session
ditty start --name=py python3

# Send commands
ditty continue --name=py 'print("hello, world")'
ditty continue --name=py 'x = 42'
ditty continue --name=py 'print(x * 2)'

# List active sessions
ditty list

# Stop the session
ditty stop --name=py
```

## Commands

### `ditty start [flags] PROGRAM [ARGS...]`

Launches a program in the background and streams its initial output until the first prompt appears.

```bash
ditty start --name=py python3
ditty start --name=debug gdb ./myprogram
ditty start myrepl                          # auto-generates a session name
```

If `--name` is omitted, a random name is generated and printed.

### `ditty continue [flags] INPUT [INPUT...]`

Sends input to a running session and streams output until the next prompt.

```bash
ditty continue --name=py 'print(42)'
ditty continue 'print(42)'                  # uses last-used session
```

With `--multi`, each argument is sent as a separate line, waiting for the prompt between each:

```bash
ditty continue --multi 'import os' 'import sys' 'print(os.getcwd())'
```

### `ditty attach [flags]`

Connects to a session interactively. Input is read line-by-line from stdin. Detach with Ctrl-D.

```bash
ditty attach --name=py
```

### `ditty stop [flags]` / `ditty kill [flags]`

`stop` sends SIGTERM and waits for the program to exit (escalates to SIGKILL after 5 seconds). `kill` sends SIGTERM immediately with a shorter escalation.

### `ditty list` / `ditty ls`

Lists active sessions with their status, command, PID, and uptime.

## Prompt detection

ditty needs to know when the program has finished producing output and is waiting for input (i.e., showing a prompt). It supports three strategies, in order of precedence:

### 1. Explicit regex (`--prompt`)

You provide a regex that matches the prompt:

```bash
ditty start --name=py --prompt='>>> $' python3
ditty start --name=db --prompt='\(gdb\) $' gdb ./a.out
```

This is the most precise — ditty returns as soon as the regex matches, with no delay.

### 2. Presets (automatic + custom)

ditty ships with built-in presets for common programs: python, node, gdb, lldb, irb, rails, sqlite3, mysql, psql, lua, R. These are applied automatically based on the command name (version suffixes like `python3.12` are handled).

Presets can set any `ditty start` flag as a default — not just `--prompt`, but also `--env`, `--idle-timeout`, `--echo`, etc. Explicit CLI flags always take precedence over preset values.

You can define your own presets in `~/.ditty/presets` (or a custom path via `--presets-file`). The file format is tab-separated pairs: a command regex and a string of flags. First match wins.

```
# command_regex<TAB>flags
# First match wins. Lines starting with # are comments.
^myrepl$	--prompt='myrepl> $'
^irb\d*$	--prompt='irb.*> $' --env=TERM=dumb
^python\d*(\.\d+)*$	--prompt='(>>>|\.\.\.) $' --idle-timeout=100ms
```

Values with spaces must be quoted (single or double quotes). User presets are checked before built-ins, so you can override built-in patterns.

```bash
ditty start python3                              # auto-detects ">>> "
ditty start --no-builtin-presets python3          # skip builtins, use idle timeout
ditty start --presets-file=./my-presets python3   # use custom presets file
```

### 3. Idle timeout (fallback)

If no regex is available, ditty waits for output to go silent for 200ms and checks if the last byte is not a newline (since prompts are typically partial lines like `>>> `). This works for most programs with no configuration, at the cost of a small delay per interaction.

The timeout is configurable:

```bash
ditty start --idle-timeout=100ms python3
ditty start --idle-timeout=500ms slow-repl
```

## Other options

### `--env`

Sets environment variables for the child process. Repeatable:

```bash
ditty start --env=TERM=dumb --env=PYTHONDONTWRITEBYTECODE=1 python3
```

This is particularly useful in presets — for example, the built-in irb preset sets `TERM=dumb` to avoid terminal control sequence issues.

### `--echo` / `--echo=false`

By default, the pty echoes input back in the output (like a real terminal). Use `--echo=false` to strip the echoed input, which is cleaner for scripting:

```bash
ditty start --echo=false python3
ditty continue 'print(42)'
# Output: just "42" and the prompt, no "print(42)" echo
```

### `--no-pty`

Uses pipes instead of a pseudoterminal. Useful for programs that don't need terminal features:

```bash
ditty start --no-pty cat
```

Note: many programs (Python, gdb, etc.) change behavior without a pty — they may suppress prompts or disable line editing. Use this only for programs that work well with pipes.

### `--suspend`

Sends SIGSTOP to the child process between commands and SIGCONT when a client connects. This prevents any background output but some programs handle suspend poorly:

```bash
ditty start --suspend python3
```

### `--buffer-size`

Configures the ring buffer size for background output (output produced while no client is connected). Default is 1MB:

```bash
ditty start --buffer-size=4194304 python3   # 4MB buffer
```

### `-v` / `--verbose`

Enables debug logging on stderr, including daemon internals (output flow, client connections, protocol messages):

```bash
ditty -v start --name=debug python3
```

## Terminal state

Some REPLs (notably irb, and Python with reline) emit terminal control sequences during startup — bracketed paste mode, application cursor keys, cursor position queries — that can leave your terminal in a modified state. ditty resets common terminal modes after `start` and `continue` return, which handles most cases.

If you still see issues (garbled prompts, unexpected characters), you can work around it by setting `TERM=dumb`:

```bash
TERM=dumb ditty start --name=rb irb
```

This tells the REPL not to use advanced terminal features. The downside is loss of colors and some line-editing features within the REPL, but since ditty is typically used for scripting (not interactive use), this is usually fine.

## Testing

Run the smoke tests (requires `python3`):

```
scripts/smoke-test.sh ./ditty
```

Or without a pre-built binary (uses `go run .`):

```
scripts/smoke-test.sh
```

Unit and integration tests:

```
go test ./...
```

## How it works

Each `ditty start` spawns a per-session daemon that allocates a pseudoterminal (so the REPL behaves as if attached to a real terminal), holds the REPL process, and listens on a Unix domain socket. `ditty continue` is a thin client that connects to the socket, sends input, streams output until the next prompt, and disconnects. See [claude/design-and-implementation.md](claude/design-and-implementation.md) for details.

## License

MIT
