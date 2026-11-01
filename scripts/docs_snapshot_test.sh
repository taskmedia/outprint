#!/usr/bin/env bash
# Tests scripts/docs-snapshot.sh in a throwaway copy of the repository (the
# working tree stays untouched): invalid input is refused, the snapshot is lean
# (no logo kit, no repo-only pages), keeps the screens, lists the version, and
# the strict site build and the site scan pass with the snapshot in place.
#
# Needs Node 22 and network for `npm ci` when website/node_modules is missing.
set -uo pipefail

here="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
version="9.9"
snap="website/versioned_docs/version-$version"

# Tracked and untracked-but-not-ignored files, so uncommitted work is tested.
(cd "$here" && git ls-files -z -co --exclude-standard | tar --null -T - -cf - 2>/dev/null) | tar -C "$work" -xf -
cd "$work" || exit 2
git init -q . && git config user.name t && git config user.email t@example.invalid
git config commit.gpgsign false

if [[ -d "$here/website/node_modules" ]]; then
  ln -s "$here/website/node_modules" website/node_modules
  echo "website/node_modules" >> .git/info/exclude
else
  (cd website && npm ci --no-audit --no-fund >/dev/null 2>&1) || { echo "FAIL: npm ci"; exit 2; }
fi
git add -A && git commit -q -m "chore: init"

failed=0
ok() { echo "ok - $1"; }
notok() { echo "not ok - $1"; failed=1; }
indent() { while IFS= read -r line; do echo "    $line"; done <<<"$1"; }
check() { # check <description> <command...>
  local desc="$1"; shift
  if "$@" >/dev/null 2>&1; then ok "$desc"; else notok "$desc"; fi
}
# shellcheck disable=SC2329  # called through check
absent() { [[ ! -e "$1" ]]; }
# shellcheck disable=SC2329  # called through check
present() { [[ -e "$1" ]]; }
refuses() { # refuses <description> <exit-regex-of-output> <args...>
  local desc="$1" rx="$2" out rc; shift 2
  out="$(scripts/docs-snapshot.sh "$@" 2>&1)"; rc=$?
  if [[ "$rc" -ne 0 ]] && grep -qE "$rx" <<<"$out"; then ok "$desc"; else notok "$desc (rc=$rc)"; indent "$out"; fi
}

refuses "no version is refused" "usage"
refuses "patch version is refused" "usage" 1.2.3
refuses "text is refused" "usage" latest

touch stray.txt
refuses "dirty tree is refused" "not clean" "$version"
rm stray.txt

out="$(scripts/docs-snapshot.sh "$version" 2>&1)"; rc=$?
if [[ "$rc" -eq 0 ]]; then ok "snapshot $version is created"; else notok "snapshot $version is created (rc=$rc)"; indent "$out"; fi
for path in "$snap" "website/versioned_sidebars/version-$version-sidebars.json" website/versions.json; do
  if grep -qF "$path" <<<"$out"; then ok "prints $path"; else notok "prints $path"; fi
done

check "versions.json lists $version" grep -qF "\"$version\"" website/versions.json
check "published page is present" present "$snap/write/when-something-goes-wrong.md"
check "screens are present" present "$snap/assets/screens/template-neutral.png"
check "logo kit is absent" absent "$snap/assets/logo"
for excluded in README.md sanitize.md publishing.md releasing.md testing.md research; do
  check "$excluded is absent" absent "$snap/$excluded"
done
check "no node_modules, lock files or build scripts" bash -c "[[ -z \"\$(find '$snap' \\( -name node_modules -o -name package-lock.json -o -name '*.py' -o -name Taskfile.yml \\) -print -quit)\" ]]"

# Every non-glob entry of the config's exclude list stays out of the snapshot.
excluded_plain="$(cd website && node -e 'for (const p of require("./docusaurus.config.js").presets[0][1].docs.exclude) if (!/[*{]/.test(p)) console.log(p)')"
while IFS= read -r entry; do
  [[ -n "$entry" ]] || continue
  check "config exclude '$entry' is absent" absent "$snap/$entry"
done <<<"$excluded_plain"

git add -A && git commit -q -m "docs: snapshot $version"
refuses "an existing snapshot is refused" "already exists" "$version"
check "tree and history scan (sanitize.sh) passes with the snapshot" scripts/sanitize.sh

if (cd website && npm run build >"$work/build.log" 2>&1); then
  ok "strict site build passes with the snapshot"
else
  notok "strict site build passes with the snapshot"; indent "$(tail -n 30 "$work/build.log")"
fi
check "newest snapshot is served at the root" present website/build/write/when-something-goes-wrong/index.html
check "unreleased docs are served at /dev/" present website/build/dev/write/when-something-goes-wrong/index.html
check "the product's /$version/ links redirect" present "website/build/$version/write/when-something-goes-wrong/index.html"
check "site scan passes" scripts/sanitize-site.sh website/build

echo "snapshot size: $(du -sk "$snap" | cut -f1) KB, of which screens $(du -sk "$snap/assets/screens" | cut -f1) KB"
exit "$failed"
