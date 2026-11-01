#!/usr/bin/env bash
# Compares the error codes in the source with the entries of the error
# catalogue, docs/write/when-something-goes-wrong.md (one `### `code``
# heading, and so one anchor, per code).
#
#   scripts/docs-error-codes.sh list     print the codes found in the source
#   scripts/docs-error-codes.sh check    fail if source and page differ
#
# The source side is every token the product can put in the `code` field of
# an error list, or use as the kind of any other failure:
#   internal/metadata/errors.go     Code...        document problems
#   internal/images/kinds.go        Kind...        image problems
#   internal/binding/binding.go     Code...        Binding problems
#   internal/gateway/admit.go       code...        busy, user_limit
#   internal/gateway/errors.go      code... and the kind argument of fail(...)
#   internal/gateway/*.go           the kind argument of fail(...)
#   internal/workerapi/workerapi.go Kind...        kinds of the Worker server
#   internal/worker/error.go        Kind...        kinds of the render pipeline
# Kinds that only appear in log lines (invalid_metadata, invalid_binding,
# invalid_images) are not part of an answer and are not listed.
set -euo pipefail

cd "$(dirname "$0")/.."
page=docs/write/when-something-goes-wrong.md

consts() { # file, constant-name pattern
  grep -hoE "^[[:space:]]*$2[A-Za-z]*[[:space:]]+([A-Za-z]+[[:space:]]+)?=[[:space:]]*\"[a-z_]+\"" "$1" | grep -oE '"[a-z_]+"' | tr -d '"' || true
}

source_codes() {
  consts internal/metadata/errors.go Code
  consts internal/images/kinds.go Kind
  consts internal/binding/binding.go Code
  consts internal/gateway/admit.go code
  consts internal/gateway/errors.go code
  # unknownToken ("unknown") replaces a malformed Worker kind; it is also
  # the metadata code `unknown`, so one entry covers both.
  consts internal/workerapi/workerapi.go Kind
  consts internal/worker/error.go Kind
  # the kind of fail(status, phase, "kind", "message", cause)
  find internal/gateway -name '*.go' ! -name '*_test.go' -exec grep -hoE 'fail\([^,]+,[[:space:]]*[A-Za-z.]+,[[:space:]]*"[a-z_]+"' {} + | grep -oE '"[a-z_]+"$' | tr -d '"'
}

page_codes() {
  grep -oE '^### `[a-z_]+`' "$page" | grep -oE '[a-z_]+' | grep -v '^$' || true
}

case "${1:-check}" in
  list)
    source_codes | sort -u
    ;;
  check)
    src=$(mktemp) doc=$(mktemp)
    trap 'rm -f "$src" "$doc"' EXIT
    source_codes | sort -u >"$src"
    page_codes | sort -u >"$doc"
    missing=$(comm -23 "$src" "$doc")
    extra=$(comm -13 "$src" "$doc")
    echo "codes in the source: $(wc -l <"$src" | tr -d ' ')"
    echo "entries in $page: $(wc -l <"$doc" | tr -d ' ')"
    status=0
    if [ -n "$missing" ]; then
      echo "in the source but not documented:" && sed 's/^/  /' <<<"$missing"
      status=1
    fi
    if [ -n "$extra" ]; then
      echo "documented but not in the source:" && sed 's/^/  /' <<<"$extra"
      status=1
    fi
    [ "$status" -eq 0 ] && echo "ok: source and page list the same codes"
    exit "$status"
    ;;
  *)
    echo "usage: $0 list|check" >&2
    exit 2
    ;;
esac
