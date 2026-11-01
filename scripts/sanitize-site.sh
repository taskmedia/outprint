#!/usr/bin/env bash
# Scans a directory of generated files (the built docs site) with the same
# denylists as scripts/sanitize.sh: the generic list in .sanitize/ and the
# private list named by $SANITIZE_DENYLIST. It checks the content of every text
# file (HTML, the search index, scripts, sitemaps) and the path of every file
# (images included) against the denylists and .sanitize/forbidden-paths.txt.
#
#   scripts/sanitize-site.sh [DIR]     DIR defaults to website/build
#
# A finding names the file and the list, never the pattern. The exit code is 1
# for findings and 2 when the directory is missing or empty, so that a broken
# build step never looks clean.
set -uo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
dir="${1:-$root/website/build}"

if [[ ! -d "$dir" ]] || [[ -z "$(find "$dir" -type f -print -quit)" ]]; then
  echo "FAIL: $dir does not exist or holds no files (build the site first)"
  exit 2
fi

patterns() { grep -vE '^\s*(#|$)' "$1"; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
patterns "$root/.sanitize/denylist.generic.txt" > "$tmp/generic"
patterns "$root/.sanitize/forbidden-paths.txt" > "$tmp/paths"
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

cd "$dir" || exit 2
failures=0
fail() { echo "FAIL: $*"; failures=$((failures + 1)); }

for kind in generic private; do
  [[ -s "$tmp/$kind" ]] || continue
  # content of text files
  while IFS= read -r -d '' file; do
    fail "${file#./} content matches $kind denylist"
  done < <(grep -rlIEi --null -f "$tmp/$kind" . 2>/dev/null)
  # file and directory names
  while IFS= read -r path; do
    if grep -qEi -f "$tmp/$kind" <<<"${path##*/}"; then
      fail "${path#./} name matches $kind denylist"
    fi
  done < <(find . -mindepth 1)
done

while IFS= read -r path; do
  if [[ -s "$tmp/paths" ]] && grep -qEi -f "$tmp/paths" <<<"$path"; then
    fail "${path#./} is a forbidden path"
  fi
done < <(find . -mindepth 1)

if (( failures > 0 )); then
  echo "sanitize-site: $failures finding(s) in $dir"
  exit 1
fi
echo "sanitize-site: clean ($(find . -type f | wc -l | tr -d ' ') files in $dir)"
