# ditty

**ditty** ("de-TTY") converts line-interactive programs (REPLs, debuggers, etc.) into command-line programs.

It runs an interactive program in the background and lets you send input and receive output through simple CLI commands, making it easy to script interactions with programs like `python3`, `gdb`, `lldb`, and others.

## Install

```
go install github.com/viettrungluu/ditty@latest
```

Or build from source:

```
git clone https://github.com/viettrungluu/ditty.git
cd ditty
go build -o ditty .
```

## Usage

```
# Start a REPL session
ditty start --name=py python3

# Send commands and get output
ditty continue --name=py 'print("hello")'

# List active sessions
ditty list

# End the session
ditty stop --name=py
```

## Testing

Run the smoke tests (requires `python3` and `gtimeout`):

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

Each `ditty start` spawns a per-session daemon that allocates a pseudoterminal (so the REPL behaves as if attached to a real terminal), holds the REPL process, and listens on a Unix domain socket. `ditty continue` is a thin client that connects to the socket, sends input, streams output until the next prompt, and disconnects.
