#!/bin/sh
# Tests scripts/security-guard.sh.
set -u
guard="$(dirname "$0")/security-guard.sh"
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
fail=0

expect() {
	want=$1 name=$2 file=$3
	"$guard" "$file" >/dev/null 2>&1
	got=$?
	if [ "$got" -ne "$want" ]; then
		echo "FAIL: $name: exit $got, want $want" >&2
		fail=1
	fi
}

printf '{"Action":"run","Test":"TestA"}\n{"Action":"pass","Test":"TestA"}\n' >"$dir/pass.json"
printf '{"Action":"pass","Test":"TestA"}\n{"Action":"skip","Test":"TestB"}\n' >"$dir/skip.json"
printf '=== RUN   TestA\n--- PASS: TestA (0.00s)\nPASS\n' >"$dir/pass.txt"
printf -- '--- PASS: TestA (0.00s)\n    --- SKIP: TestA/sub (0.00s)\n' >"$dir/skip.txt"
printf '{"Action":"output","Output":"no tests to run\\n"}\n' >"$dir/empty.json"

expect 0 "json pass" "$dir/pass.json"
expect 1 "json skip" "$dir/skip.json"
expect 0 "text pass" "$dir/pass.txt"
expect 1 "text skip" "$dir/skip.txt"
expect 1 "nothing ran" "$dir/empty.json"
expect 1 "missing log" "$dir/absent.json"
[ "$fail" -eq 0 ] && echo "security-guard tests passed"
exit "$fail"
