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

run_ditty() {
	# shellcheck disable=SC2086
	gtimeout 10 $DITTY "$@" 2>&1 || true
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
assert_contains "auto name shows session" "$out" "session"
assert_contains "auto name shows prompt" "$out" ">>>"

# Kill via last-used.
run_ditty kill >/dev/null

# ---------------------------------------------------------------------------
echo "=== --echo=false ==="

run_ditty start --name=noecho --echo=false python3 >/dev/null

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
