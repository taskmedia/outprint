#!/usr/bin/env bash
# Tests the site scan: a clean build directory passes; a planted word in HTML or
# in the search index, a planted file name and a private denylist hit fail,
# naming the file and never the pattern.
set -uo pipefail

# Forbidden words are assembled at runtime so this file itself stays clean.
w_generic="foer""der""ver""ein"
here="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

site="$work/build"
mkdir -p "$site/docs" "$site/assets/images"
echo "<html><body>Clean page</body></html>" > "$site/index.html"
echo '[{"i":1,"t":"Clean title"}]' > "$site/search-index.json"
printf '\x89PNG\r\n' > "$site/assets/images/screen.png"

failed=0
expect() { # expect <rc> <description> <output-regex> [forbidden-output-regex]; env is passed through
  local want="$1" desc="$2" rx="${3:-}" bad="${4:-}" out rc
  out="$("$here/scripts/sanitize-site.sh" "$site" 2>&1)"; rc=$?
  if [[ "$rc" -ne "$want" ]] || { [[ -n "$rx" ]] && ! grep -qE "$rx" <<<"$out"; } \
     || { [[ -n "$bad" ]] && grep -qiE "$bad" <<<"$out"; }; then
    echo "not ok - $desc (rc=$rc)"; echo "$out" | sed 's/^/    /'; failed=1
  else
    echo "ok - $desc"
  fi
}

expect 0 "clean site passes" "sanitize-site: clean"

echo "<p>$w_generic</p>" > "$site/docs/page.html"
expect 1 "planted generic word in HTML fails, naming the file" "docs/page.html content matches generic" "$w_generic"
rm "$site/docs/page.html"

echo "[{\"t\":\"$w_generic\"}]" > "$site/search-index.json"
expect 1 "planted word in the search index fails" "search-index.json content matches generic" "$w_generic"
echo '[{"i":1,"t":"Clean title"}]' > "$site/search-index.json"

: > "$site/assets/images/$w_generic.png"
expect 1 "planted word in an image file name fails" "assets/images/.* name matches generic"
rm "$site/assets/images/$w_generic.png"

echo "secret-term-xyz" > "$work/private.txt"
echo "<p>about secret-term-xyz</p>" > "$site/docs/other.html"
SANITIZE_DENYLIST="$work/private.txt" expect 1 "private denylist hit fails, without the pattern" "docs/other.html content matches private" "secret-term-xyz"
expect 0 "the same page passes without the private list" "note: SANITIZE_DENYLIST not set"
rm "$site/docs/other.html"

SANITIZE_DENYLIST="$work/missing.txt" expect 2 "unreadable private list fails" "not readable"

rm -rf "$site"; mkdir "$site"
expect 2 "empty build directory fails instead of passing" "holds no files"

exit "$failed"
