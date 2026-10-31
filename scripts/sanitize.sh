#!/usr/bin/env bash
# Scans the working tree and the full git history for forbidden strings and
# paths. Exits non-zero and names every location on failure.
#
#   SANITIZE_DENYLIST  optional path to a private denylist kept outside the repo
#
# Run from anywhere inside the repository.
set -uo pipefail

root="$(git rev-parse --show-toplevel)" || exit 2
cd "$root"

generic=".sanitize/denylist.generic.txt"
forbidden=".sanitize/forbidden-paths.txt"
exclude=(":(exclude).sanitize")

failures=0
fail() { echo "FAIL: $*"; failures=$((failures + 1)); }

patterns() { grep -vE '^\s*(#|$)' "$1"; }

# Writes the effective pattern files to a temp dir; the private list is
# deliberately never echoed.
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
patterns "$generic" > "$tmp/generic"
patterns "$forbidden" > "$tmp/paths"
: > "$tmp/private"
if [[ -n "${SANITIZE_DENYLIST:-}" ]]; then
  if [[ ! -r "$SANITIZE_DENYLIST" ]]; then
    echo "FAIL: SANITIZE_DENYLIST is set but not readable"
    exit 2
  fi
  patterns "$SANITIZE_DENYLIST" > "$tmp/private"
else
  echo "note: SANITIZE_DENYLIST not set; running generic checks only"
fi

# scan_text <label> <file>: report matching lines of a text stream.
scan_text() {
  local label="$1" file="$2" kind list
  for kind in generic private; do
    [[ -s "$tmp/$kind" ]] || continue
    while IFS=: read -r line _; do
      [[ -n "$line" ]] || continue
      if [[ "$kind" == generic ]]; then
        fail "$label line $line matches generic denylist"
      else
        fail "$label line $line matches private denylist"
      fi
    done < <(grep -nEi -f "$tmp/$kind" "$file" | cut -d: -f1 | sed 's/$/:/')
  done
}

# 1. Working tree: tracked files plus untracked files that are not ignored.
while IFS= read -r -d '' path; do
  [[ -f "$path" ]] || continue
  [[ "$path" == .sanitize/* ]] && continue
  if grep -qI . "$path" 2>/dev/null; then
    scan_text "$path" "$path"
  fi
  while IFS= read -r p; do
    [[ -n "$p" ]] && grep -qEi -e "$p" <<<"$path" && fail "$path is a forbidden path"
  done < "$tmp/paths"
done < <(git ls-files -co --exclude-standard -z)

# 2. History: commit messages, authors, tags, branches.
if git rev-parse --verify -q HEAD >/dev/null; then
  git log --all --format='commit %H%n%an <%ae>%n%cn <%ce>%n%B' > "$tmp/messages"
  scan_text "git history (commit messages/authors)" "$tmp/messages"

  # 3. History: every diff, excluding the denylist files themselves.
  git log --all -p --format='commit %H' -- . "${exclude[@]}" > "$tmp/diffs"
  scan_text "git history (diffs)" "$tmp/diffs"

  # 4. History: every path ever committed.
  git log --all --name-only --format= -- . "${exclude[@]}" | sort -u > "$tmp/histpaths"
  while IFS= read -r hp; do
    [[ -n "$hp" ]] || continue
    while IFS= read -r p; do
      [[ -n "$p" ]] && grep -qEi -e "$p" <<<"$hp" && fail "history contains forbidden path $hp"
    done < "$tmp/paths"
  done < "$tmp/histpaths"
fi

# Tag names and tag annotations, branch names.
{
  git tag -l
  git for-each-ref --format='%(refname)%0a%(contents)' refs/tags
  git for-each-ref --format='%(refname)' refs/heads refs/remotes
} > "$tmp/refs"
scan_text "git refs (tags/branches)" "$tmp/refs"

# 5. Secrets (optional locally, mandatory in CI).
if command -v gitleaks >/dev/null; then
  gitleaks detect --source . --no-banner --redact >/dev/null 2>&1 \
    || fail "gitleaks reported findings (run: gitleaks detect --source . --redact)"
else
  echo "note: gitleaks not installed; skipping secret scan"
fi

if (( failures > 0 )); then
  echo "sanitize: $failures finding(s)"
  exit 1
fi
echo "sanitize: clean"
