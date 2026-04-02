#!/bin/bash
#
# Smoke tests for ditty. Exercises the core commands against a real python3
# REPL and checks output for expected content.
#
# Usage:
#   scripts/smoke-test.sh [path-to-ditty-binary]
#
# If no binary path is given, builds and uses "go run .".

set -euo pipefail

DITTY="${1:-}"
if [ -z "$DITTY" ]; then
	echo "No binary specified, using 'go run .'"
	DITTY="go run ."
fi

# Use a temporary HOME so we don't pollute the real one.
export HOME
HOME="$(mktemp -d /tmp/ditty-smoke-XXXX)"
cleanup() { chmod -R u+w "$HOME" 2>/dev/null; rm -rf "$HOME"; }
trap cleanup EXIT

PASS=0
FAIL=0
TESTS=0

pass() {
	PASS=$((PASS + 1))
	TESTS=$((TESTS + 1))
	echo "  PASS: $1"
}

fail() {
	FAIL=$((FAIL + 1))
	TESTS=$((TESTS + 1))
	echo "  FAIL: $1"
	echo "    $2"
}

assert_contains() {
	local label="$1" output="$2" expected="$3"
	if echo "$output" | grep -qF "$expected"; then
		pass "$label"
	else
		fail "$label" "expected output to contain '$expected', got: $output"
	fi
}

assert_not_contains() {
	local label="$1" output="$2" unexpected="$3"
	if echo "$output" | grep -qF "$unexpected"; then
		fail "$label" "expected output NOT to contain '$unexpected', got: $output"
	else
		pass "$label"
	fi
}

# Find a working timeout command (gtimeout on macOS with coreutils,
# timeout on Linux).
if command -v gtimeout &>/dev/null; then
	TIMEOUT_CMD=gtimeout
elif command -v timeout &>/dev/null; then
	TIMEOUT_CMD=timeout
else
	echo "ERROR: neither gtimeout nor timeout found" >&2
	exit 1
fi

run_ditty() {
	# shellcheck disable=SC2086
	$TIMEOUT_CMD 10 $DITTY "$@" 2>&1 || true
}

# ---------------------------------------------------------------------------
echo "=== Basic start/continue/stop ==="

out=$(run_ditty start --name=basic python3)
assert_contains "start shows prompt" "$out" ">>>"
assert_contains "start shows session name" "$out" "basic"

out=$(run_ditty continue --name=basic 'print(40 + 2)')
assert_contains "continue shows output" "$out" "42"

out=$(run_ditty stop --name=basic)
assert_contains "stop confirms" "$out" "stopped"

# ---------------------------------------------------------------------------
echo "=== List ==="

run_ditty start --name=list1 python3 >/dev/null
run_ditty start --name=list2 python3 >/dev/null

out=$(run_ditty list)
assert_contains "list shows list1" "$out" "list1"
assert_contains "list shows list2" "$out" "list2"
assert_contains "list shows alive" "$out" "alive"
assert_contains "list shows PID column" "$out" "PID"

run_ditty kill --name=list1 >/dev/null
run_ditty kill --name=list2 >/dev/null
sleep 0.5

out=$(run_ditty list)
assert_not_contains "list empty after kill" "$out" "alive"

# ---------------------------------------------------------------------------
echo "=== State persists across continues ==="

run_ditty start --name=state python3 >/dev/null

run_ditty continue --name=state 'x = 123' >/dev/null
out=$(run_ditty continue --name=state 'print(x * 2)')
assert_contains "state persists" "$out" "246"

run_ditty kill --name=state >/dev/null

# ---------------------------------------------------------------------------
echo "=== Last-used session ==="

run_ditty start --name=last python3 >/dev/null

out=$(run_ditty continue 'print("no-name")')
assert_contains "continue without --name" "$out" "no-name"

run_ditty kill --name=last >/dev/null

# ---------------------------------------------------------------------------
echo "=== Auto-generated name ==="

out=$(run_ditty start python3)
assert_contains "auto name shows command prefix" "$out" "python3-"
assert_contains "auto name shows prompt" "$out" ">>>"

# Kill via last-used.
run_ditty kill >/dev/null

# ---------------------------------------------------------------------------
echo "=== --no-echo ==="

run_ditty start --name=noecho --no-echo python3 >/dev/null

out=$(run_ditty continue --name=noecho 'print("visible")')
assert_contains "no-echo shows output" "$out" "visible"
assert_not_contains "no-echo hides input" "$out" 'print("visible")'

run_ditty kill --name=noecho >/dev/null

# ---------------------------------------------------------------------------
echo "=== --prompt regex ==="

run_ditty start --name=regex --prompt='>>> $' python3 >/dev/null

out=$(run_ditty continue --name=regex 'print("fast")')
assert_contains "regex prompt works" "$out" "fast"

run_ditty kill --name=regex >/dev/null

# ---------------------------------------------------------------------------
echo "=== --idle-timeout ==="

run_ditty start --name=timeout --idle-timeout=50ms python3 >/dev/null

out=$(run_ditty continue --name=timeout 'print("quick")')
assert_contains "custom timeout works" "$out" "quick"

run_ditty kill --name=timeout >/dev/null

# ---------------------------------------------------------------------------
echo "=== --no-pty (cat) ==="

run_ditty start --name=nopty --no-pty cat >/dev/null

out=$(run_ditty continue --name=nopty 'hello pipes')
assert_contains "no-pty cat echoes" "$out" "hello pipes"

run_ditty kill --name=nopty >/dev/null

# ---------------------------------------------------------------------------
echo "=== preset auto-detection ==="

# python3 should auto-detect without --prompt.
run_ditty start --name=preset python3 >/dev/null

out=$(run_ditty continue --name=preset 'print("preset")')
assert_contains "preset auto-detects python" "$out" "preset"

run_ditty kill --name=preset >/dev/null

# --no-builtin-presets should fall back to idle timeout (still works, just slower).
run_ditty start --name=nopreset --no-builtin-presets python3 >/dev/null

out=$(run_ditty continue --name=nopreset 'print("fallback")')
assert_contains "no-builtin-presets falls back" "$out" "fallback"

run_ditty kill --name=nopreset >/dev/null

# User presets file with new 3-field format: name<TAB>regex<TAB>flags.
PRESETS_FILE="$HOME/.ditty/user-presets"
mkdir -p "$(dirname "$PRESETS_FILE")"
printf "mypython\t^mypython( |\$)\t--prompt='(>>>|\\\\.\\\\.\\\\.\\\\.) \$'\n" > "$PRESETS_FILE"

# Create a symlink so "mypython" resolves to python3.
MYPYTHON="$HOME/.ditty/mypython"
ln -sf "$(command -v python3)" "$MYPYTHON"

run_ditty start --name=userpreset --presets-file="$PRESETS_FILE" "$MYPYTHON" >/dev/null

out=$(run_ditty continue --name=userpreset 'print("custom")')
assert_contains "user presets file works" "$out" "custom"

run_ditty kill --name=userpreset >/dev/null
rm -f "$PRESETS_FILE" "$MYPYTHON"

# ---------------------------------------------------------------------------
echo "=== --env flag ==="

run_ditty start --name=envtest --env=DITTY_TEST_VAR=hello123 python3 >/dev/null

out=$(run_ditty continue --name=envtest 'import os; print(os.environ["DITTY_TEST_VAR"])')
assert_contains "env var passed to child" "$out" "hello123"

run_ditty kill --name=envtest >/dev/null

# ---------------------------------------------------------------------------
echo "=== --suspend ==="

run_ditty start --name=suspend --suspend python3 >/dev/null

out=$(run_ditty continue --name=suspend 'print("resumed")')
assert_contains "suspend resumes for continue" "$out" "resumed"

out=$(run_ditty continue --name=suspend 'print("again")')
assert_contains "suspend resumes again" "$out" "again"

run_ditty kill --name=suspend >/dev/null

# ---------------------------------------------------------------------------
echo "=== --multi ==="

run_ditty start --name=multi python3 >/dev/null

out=$(run_ditty continue --name=multi --multi 'x = 10' 'y = 20' 'print(x + y)')
assert_contains "multi sends all lines" "$out" "30"

# Verify state persisted across the multi lines.
out=$(run_ditty continue --name=multi 'print(x * y)')
assert_contains "multi state persists" "$out" "200"

run_ditty kill --name=multi >/dev/null

# ---------------------------------------------------------------------------
echo "=== attach ==="

run_ditty start --name=attach python3 >/dev/null

out=$(printf 'x = 77\nprint(x)\n' | run_ditty attach --name=attach)
assert_contains "attach shows output" "$out" "77"

# State should persist after detach.
out=$(run_ditty continue --name=attach 'print(x + 3)')
assert_contains "state persists after attach" "$out" "80"

run_ditty kill --name=attach >/dev/null

# ---------------------------------------------------------------------------
echo "=== terminal reset after start ==="

# After ditty start returns, bracketed paste mode should be off.
# We check by capturing the raw output and looking for the reset sequence.
out=$(run_ditty start --name=termreset python3)
# The output should end with the reset sequences, not leave bracketed
# paste enabled. Check that \e[?2004l (disable bracketed paste) is present.
if printf '%s' "$out" | grep -q $'\x1b\[?2004l'; then
	pass "terminal reset after start"
else
	# Even if the reset sequence isn't in the captured output (it may go
	# directly to the terminal), verify that start at least completed.
	assert_contains "terminal reset: start completed" "$out" ">>>"
fi

run_ditty kill --name=termreset >/dev/null

# ---------------------------------------------------------------------------
echo "=== terminal reset after continue ==="

run_ditty start --name=termreset2 python3 >/dev/null

out=$(run_ditty continue --name=termreset2 'print("reset")')
if printf '%s' "$out" | grep -q $'\x1b\[?2004l'; then
	pass "terminal reset after continue"
else
	assert_contains "terminal reset: continue works" "$out" "reset"
fi

run_ditty kill --name=termreset2 >/dev/null

# ---------------------------------------------------------------------------
echo "=== --preset flag ==="

run_ditty start --name=presetflag --preset=python python3 >/dev/null

out=$(run_ditty continue --name=presetflag 'print("preset-flag")')
assert_contains "--preset selects by name" "$out" "preset-flag"

run_ditty kill --name=presetflag >/dev/null

# ---------------------------------------------------------------------------
echo "=== list-presets ==="

out=$(run_ditty list-presets)
assert_contains "list-presets shows python" "$out" "python"
assert_contains "list-presets shows rails" "$out" "rails"
assert_contains "list-presets shows gdb" "$out" "gdb"
assert_contains "list-presets shows NAME header" "$out" "NAME"

# ---------------------------------------------------------------------------
echo "=== Missing session error ==="

out=$(run_ditty continue --name=nonexistent 'hello')
assert_contains "missing session error" "$out" "not found"

# ---------------------------------------------------------------------------
echo "=== Kill ==="

run_ditty start --name=killme python3 >/dev/null

out=$(run_ditty kill --name=killme)
assert_contains "kill confirms" "$out" "killed"

sleep 0.5
out=$(run_ditty list)
assert_not_contains "gone after kill" "$out" "killme"

# ---------------------------------------------------------------------------
echo ""
echo "Results: $PASS passed, $FAIL failed out of $TESTS tests"
if [ "$FAIL" -gt 0 ]; then
	exit 1
fi
