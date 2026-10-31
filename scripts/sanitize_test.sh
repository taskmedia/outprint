#!/usr/bin/env bash
# Tests the sanitize gate: clean tree passes, planted strings fail with a location.
set -uo pipefail

# Forbidden words are assembled at runtime so this file itself stays clean.
w_generic="foer""der""ver""ein"
w_hist="hand""baller"
w_branch="ver""ein"
w_tag="tv""m-"
here="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

cd "$work"
git init -q . && git config user.name t && git config user.email t@example.invalid
git config commit.gpgsign false
mkdir -p .sanitize scripts
cp "$here"/.sanitize/*.txt .sanitize/
cp "$here"/scripts/sanitize.sh scripts/
echo "clean file" > a.txt
git add . && git commit -q -m "chore: init"

failed=0
expect() { # expect <0|1> <description> [output-regex]
  local want="$1" desc="$2" rx="${3:-}" out rc
  out="$(scripts/sanitize.sh 2>&1)"; rc=$?
  if [[ "$rc" -ne "$want" ]] || { [[ -n "$rx" ]] && ! grep -qE "$rx" <<<"$out"; }; then
    echo "not ok - $desc (rc=$rc)"; echo "$out" | sed 's/^/    /'; failed=1
  else
    echo "ok - $desc"
  fi
}

expect 0 "clean tree passes"

echo "see $w_generic here" > planted.txt
expect 1 "planted generic string in tree fails, naming the file" "planted.txt line 1"
rm planted.txt

echo "secret-term-xyz" > private.txt
echo "secret-term-xyz" > a.txt
SANITIZE_DENYLIST="$work/private.txt" expect 1 "private denylist hit fails" "a.txt line 1 matches private"
git checkout -q a.txt; rm private.txt

echo "bad" > planted.txt && git add planted.txt && echo "$w_hist" > planted.txt
git add planted.txt && git commit -q -m "chore: add" && git rm -q planted.txt && git commit -q -m "chore: drop"
expect 1 "string present only in history fails" "git history \(diffs\)"

git checkout -q --orphan fresh && git rm -rfq . >/dev/null 2>&1; git checkout -q master 2>/dev/null || git checkout -q main
git reset -q --hard HEAD~2
git branch -D fresh -q 2>/dev/null
git branch "feat/$w_branch-x" && expect 1 "branch name fails" "git refs"
git branch -q -D "feat/$w_branch-x"

git tag -a v1 -m "${w_tag}release" && expect 1 "tag annotation fails" "git refs"
git tag -d v1 >/dev/null

mkdir -p __tmp && echo x > __tmp/a.txt && git add -f __tmp/a.txt
expect 1 "forbidden path in tree fails" "forbidden path"

exit "$failed"
