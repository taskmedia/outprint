#!/usr/bin/env bash
# Creates the docs snapshot of one minor release from the docs at the current
# checkout: `docusaurus docs:version X.Y`, then prunes everything the site does
# not publish (logo kit, repo-only pages; the list comes from
# website/docusaurus.config.js). Prints the paths to commit.
#
#   scripts/docs-snapshot.sh X.Y
#
# Refuses an invalid version, a snapshot that already exists and a dirty tree.
# Needs `npm ci` in website/ first. See docs/releasing.md.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
version="${1:-}"

if [[ ! "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  echo "usage: scripts/docs-snapshot.sh X.Y   (a minor version such as 0.1, got '${version}')" >&2
  exit 2
fi

cd "$root"
if [[ -n "$(git status --porcelain)" ]]; then
  echo "FAIL: the working tree is not clean; commit or stash first" >&2
  exit 1
fi

site="website"
snapshot="$site/versioned_docs/version-$version"
sidebar="$site/versioned_sidebars/version-$version-sidebars.json"
versions="$site/versions.json"

if [[ -e "$snapshot" ]] || { [[ -f "$versions" ]] && grep -qF "\"$version\"" "$versions"; }; then
  echo "FAIL: a snapshot for $version already exists; old versions are frozen" >&2
  exit 1
fi

if [[ ! -d "$site/node_modules" ]]; then
  echo "FAIL: $site/node_modules is missing; run 'npm ci' in $site first" >&2
  exit 1
fi

# The tree was clean, so a failed run can be undone completely.
undo_partial_snapshot() {
  git clean -fdq -- "$snapshot" "$sidebar"
  git checkout -q -- "$versions" 2>/dev/null || rm -f "$versions"
}
on_exit() { local status=$?; [[ $status -eq 0 ]] || undo_partial_snapshot; }
trap on_exit EXIT

(cd "$site" && npx --no-install docusaurus docs:version "$version")
(cd "$site" && node scripts/prune-snapshot.mjs "versioned_docs/version-$version")

echo
echo "Snapshot $version created ($(du -sk "$snapshot" | cut -f1) KB). Commit:"
printf '  %s\n' "$snapshot" "$sidebar" "$versions"
