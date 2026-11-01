#!/usr/bin/env bash
# Renders docs/assets/screens/template-<name>.png for every Template under
# templates/: the example document of the catalogue (the same one the page
# shows), typeset by the real pipeline in the golden image, first page turned
# into a PNG with pdftoppm. Needs a Docker daemon; run it through
# `task docs:catalogue-preview` and commit the result. The site build never
# renders: it only uses these committed files.
set -euo pipefail

cd "$(dirname "$0")/.."
image=${GOLDEN_IMAGE:-outprint-golden}
configuration=example
title="Spring plant sale"
date=2026-03-02
resolution=80
out=docs/assets/screens

mkdir -p "$out"
for dir in templates/*/; do
  name=$(basename "$dir")
  work=$(mktemp -d)
  trap 'rm -rf "$work"' EXIT
  chmod 0777 "$work"
  go run ./scripts/docs-catalogue -print-example "$name" >"$work/example.md"
  docker run --rm --entrypoint sh -v "$work:/job" \
    -e SOURCE_DATE_EPOCH=1700000000 -e FORCE_SOURCE_DATE=1 "$image" -c "
      outprint render --template '$name' --config '$configuration' --title '$title' --date '$date' \
        -o /job/example.pdf /job/example.md &&
      pdftoppm -png -r $resolution -f 1 -l 1 -singlefile /job/example.pdf /job/page"
  cp "$work/page.png" "$out/template-$name.png"
  echo "wrote $out/template-$name.png"
done
