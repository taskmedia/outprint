#!/usr/bin/env bash
# Writes a copy of Outline's page template with the Outprint script tag added,
# for mounting over the original (docs/deploy/components/mounted-template.md).
#
#   patch-outline-template.sh IMAGE OUTPUT_FILE
#   OUTLINE_TEMPLATE_SRC=index.html patch-outline-template.sh - OUTPUT_FILE
#
# IMAGE is the Outline image the template is taken from, for example
# outlinewiki/outline:1.12.0. Take it from the image you will mount it into:
# the template changes between Outline releases. With IMAGE "-" the template
# is read from OUTLINE_TEMPLATE_SRC instead (used by the tests, no Docker).
#
# OUTPRINT_SCRIPT_TAG overrides the tag, for example to set data- options.
set -euo pipefail

# Where the production server reads its template (build/server/routes/app.js:
# ../../app/index.html relative to build/server/routes), not
# server/static/index.html, which only development and test mode read.
readonly TEMPLATE_IN_IMAGE=/opt/outline/build/app/index.html
readonly DEFAULT_TAG='<script src="/pdf/button.js" defer></script>'

die() { echo "patch-outline-template: $*" >&2; exit 1; }

[ $# -eq 2 ] || die "usage: $0 IMAGE OUTPUT_FILE"
image=$1
output=$2
tag=${OUTPRINT_SCRIPT_TAG:-$DEFAULT_TAG}

work=$(mktemp -d)
container=
cleanup() {
  [ -z "$container" ] || docker rm -f "$container" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

source_file=$work/index.html
if [ "$image" = "-" ]; then
  [ -n "${OUTLINE_TEMPLATE_SRC:-}" ] || die "IMAGE is -, set OUTLINE_TEMPLATE_SRC"
  cp "$OUTLINE_TEMPLATE_SRC" "$source_file"
else
  container=$(docker create "$image") || die "cannot create a container from $image"
  docker cp "$container:$TEMPLATE_IN_IMAGE" "$source_file" \
    || die "$TEMPLATE_IN_IMAGE not found in $image; Outline moved its template, see the docs"
fi

closing_tags=$(grep -o '</body>' "$source_file" | wc -l | tr -d ' ')
[ "$closing_tags" -eq 1 ] || die "expected exactly one </body> in the template, found $closing_tags"
if grep -qF -- "$tag" "$source_file"; then
  die "the template already contains the tag; start from the original image"
fi

# Outline fills {placeholders} with a global replace after reading the file; a
# tag containing one would be rewritten.
case $tag in *'{'*) die "the tag must not contain braces" ;; esac

# index/substr, not sub(): the tag may contain & or backslashes, which sub()
# and sed would treat as special in the replacement.
TAG=$tag awk '{
  i = index($0, "</body>")
  if (i > 0) $0 = substr($0, 1, i - 1) ENVIRON["TAG"] substr($0, i)
  print
}' "$source_file" > "$work/patched.html"
grep -qF -- "$tag" "$work/patched.html" || die "inserting the tag failed"

mkdir -p "$(dirname "$output")"
mv "$work/patched.html" "$output"
chmod 0644 "$output"
echo "wrote $output (tag before </body>, from ${image})"
