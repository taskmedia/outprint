#!/usr/bin/env bash
# Tests scripts/patch-outline-template.sh on a template file, without Docker.
set -uo pipefail

here="$(cd "$(dirname "$0")/.." && pwd)"
script="$here/scripts/patch-outline-template.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0

check() {
  local name=$1 ok=$2
  if [ "$ok" = 1 ]; then echo "ok   $name"; else echo "FAIL $name"; failures=$((failures + 1)); fi
}

printf '<html>\n  <body>\n    {script-tags}\n  </body>\n</html>\n' > "$work/index.html"

OUTLINE_TEMPLATE_SRC="$work/index.html" "$script" - "$work/out/patched.html" >/dev/null 2>&1
check "patching succeeds" "$([ $? -eq 0 ] && echo 1 || echo 0)"
check "the tag sits right before </body>" \
  "$(grep -qF '<script src="/pdf/button.js" defer></script></body>' "$work/out/patched.html" && echo 1 || echo 0)"
check "the placeholders are kept" \
  "$(grep -qF '{script-tags}' "$work/out/patched.html" && echo 1 || echo 0)"
check "the rest of the file is unchanged" \
  "$([ "$(diff "$work/index.html" "$work/out/patched.html" | grep -c '^[<>]')" -eq 2 ] && echo 1 || echo 0)"

OUTLINE_TEMPLATE_SRC="$work/out/patched.html" "$script" - "$work/twice.html" >/dev/null 2>&1
check "an already patched template is refused" "$([ $? -ne 0 ] && [ ! -e "$work/twice.html" ] && echo 1 || echo 0)"

printf '<html><body>one</body><body>two</body></html>\n' > "$work/two.html"
OUTLINE_TEMPLATE_SRC="$work/two.html" "$script" - "$work/two-out.html" >/dev/null 2>&1
check "a template with two </body> is refused" "$([ $? -ne 0 ] && echo 1 || echo 0)"

printf '<html></html>\n' > "$work/none.html"
OUTLINE_TEMPLATE_SRC="$work/none.html" "$script" - "$work/none-out.html" >/dev/null 2>&1
check "a template without </body> is refused" "$([ $? -ne 0 ] && echo 1 || echo 0)"

OUTLINE_TEMPLATE_SRC="$work/index.html" OUTPRINT_SCRIPT_TAG='<script src="/pdf/button.js?a=1&b=2" data-x="\1" defer></script>' \
  "$script" - "$work/custom.html" >/dev/null 2>&1
check "a custom tag is inserted literally (& and backslash)" \
  "$(grep -qF 'button.js?a=1&b=2" data-x="\1"' "$work/custom.html" && echo 1 || echo 0)"

OUTLINE_TEMPLATE_SRC="$work/index.html" OUTPRINT_SCRIPT_TAG='<script>{env}</script>' \
  "$script" - "$work/brace.html" >/dev/null 2>&1
check "a tag with braces is refused" "$([ $? -ne 0 ] && echo 1 || echo 0)"

[ "$failures" -eq 0 ] || { echo "$failures failed"; exit 1; }
