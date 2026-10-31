#!/bin/sh
# Fails if a security test was skipped, or if no test ran at all. A skipped
# security test proves nothing, so a skip counts as a failure.
#
#   scripts/security-guard.sh LOGFILE...
#
# Reads the output of `go test -json` and of a Go test binary run with -v.
set -u

if [ "$#" -eq 0 ]; then
	echo "usage: security-guard.sh LOGFILE..." >&2
	exit 2
fi

status=0
for log in "$@"; do
	if [ ! -r "$log" ]; then
		echo "security-guard: cannot read $log" >&2
		status=1
		continue
	fi
	skipped=$(grep -E '"Action":"skip"|^[[:space:]]*--- SKIP|^[[:space:]]*=== SKIP' "$log")
	if [ -n "$skipped" ]; then
		echo "security-guard: tests were skipped in $log:" >&2
		echo "$skipped" | head -20 >&2
		status=1
	fi
	if ! grep -qE '"Action":"pass"|^[[:space:]]*--- PASS' "$log"; then
		echo "security-guard: no test passed in $log (nothing ran?)" >&2
		status=1
	fi
done
exit "$status"
