#!/usr/bin/env bash
# Tests the docs example gate (scripts/docs-validate-examples): good blocks and
# skips pass; a bad block, an unmarked block, an unknown mode and a stray
# marker fail with file:line.
set -uo pipefail

here="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

cd "$here"
go build -o "$work/outprint" ./cmd/outprint || { echo "not ok - build outprint"; exit 1; }
go build -o "$work/gate" ./scripts/docs-validate-examples || { echo "not ok - build gate"; exit 1; }

failed=0
expect() { # expect <0|1> <description> [output-regex]; runs the gate on $work/docs
  local want="$1" desc="$2" rx="${3:-}" out rc
  out="$("$work/gate" -docs "$work/docs" -root "$here" -bin "$work/outprint" 2>&1)"; rc=$?
  if [[ "$rc" -ne "$want" ]] || { [[ -n "$rx" ]] && ! grep -qE "$rx" <<<"$out"; }; then
    echo "not ok - $desc (rc=$rc)"; echo "$out" | sed 's/^/    /'; failed=1
  else
    echo "ok - $desc"
  fi
}

rm -rf "$work/docs"; mkdir -p "$work/docs"
cat > "$work/docs/good.md" <<'EOF'
# Good

<!-- validate: metadata template=neutral configuration=example -->
```yaml
type: letter
subject: Spring plant sale
```

<!-- validate: binding -->
````markdown
```yaml
binding:
  template: neutral
  configuration: example
```
````

<!-- validate: service-fragment -->
```yaml
admission:
  slots: 2
```

<!-- validate: skip illustrative fragment -->
```yaml
anything: [goes
```

```sh
echo "not yaml, needs no marker"
```
EOF
expect 0 "good blocks and a skip pass" "ok, 3 validated, 1 skipped"

cat > "$work/docs/bad.md" <<'EOF'
<!-- validate: metadata template=neutral configuration=example -->
```yaml
type: letter
closing: [a, b]
```
EOF
expect 1 "invalid Metadata Block fails with the tool's message" "must be text, not a list"

cat > "$work/docs/bad.md" <<'EOF'
<!-- validate: service-fragment -->
```yaml
admission:
  slots: 0
```
EOF
expect 1 "invalid service configuration fails with the output of validate" "bad.md:2: \[service-fragment\].*validate failed"

cat > "$work/docs/bad.md" <<'EOF'
<!-- validate: binding -->
```yaml
binding:
  template: neutral
  configuration: nonexistent
```
EOF
expect 1 "binding to an unknown Configuration fails" "bad.md:2: \[binding\]"

cat > "$work/docs/bad.md" <<'EOF'
Text.

```yaml
a: b
```
EOF
expect 1 "unmarked yaml block fails" "bad.md:3: yaml block has no"

cat > "$work/docs/bad.md" <<'EOF'
<!-- validate: frobnicate -->
```yaml
a: b
```
EOF
expect 1 "unknown mode fails" "unknown validate mode"

cat > "$work/docs/bad.md" <<'EOF'
<!-- validate: skip -->
```yaml
a: b
```
EOF
expect 1 "skip without a reason fails" "needs a reason"

cat > "$work/docs/bad.md" <<'EOF'
<!-- validate: skip because -->

Not a fence.
EOF
expect 1 "marker that is not before a fence fails" "bad.md:1: validate marker is not directly"

rm "$work/docs/bad.md"
expect 0 "clean again after removing the bad page"

# The real docs pass.
out="$("$work/gate" -docs "$here/docs" -root "$here" -bin "$work/outprint" 2>&1)"; rc=$?
if [[ "$rc" -eq 0 ]]; then echo "ok - repository docs pass"; else echo "not ok - repository docs"; echo "$out" | sed 's/^/    /'; failed=1; fi

exit "$failed"
