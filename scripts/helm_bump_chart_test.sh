#!/usr/bin/env bash
# Tests helm-bump-chart.sh against a copy of the chart metadata in taskmedia/helm.
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0

check() {
  local name="$1" got="$2" want="$3"
  if [[ "$got" == "$want" ]]; then
    echo "ok   $name"
  else
    echo "FAIL $name"; echo "  want: $want"; echo "  got:  $got"
    failures=$((failures + 1))
  fi
}

write_chart() {
  cat > "$work/Chart.yaml" <<'EOF'
apiVersion: v2
name: outprint
type: application
version: 1.1.0
# outprint release tag without the leading v
appVersion: "1.0.0"

annotations:
  artifacthub.io/category: skip-prediction
  artifacthub.io/images: |
    - name: outprint
      image: ghcr.io/taskmedia/outprint:1.0.0
    - name: outprint-injector
      image: ghcr.io/taskmedia/outprint-injector:1.0.0
  artifacthub.io/changes: |
    - kind: added
      description: Optional Traefik IngressRoute
    - kind: fixed
      description: Something else
  artifacthub.io/links: |
    - name: support
      url: https://github.com/taskmedia/helm/issues
EOF
}

write_chart
"$here/helm-bump-chart.sh" "$work" 1.2.0 > /dev/null
chart="$(cat "$work/Chart.yaml")"

check "bumps the chart patch version" "$(grep '^version:' <<< "$chart")" "version: 1.1.1"
check "sets appVersion" "$(grep '^appVersion:' <<< "$chart")" 'appVersion: "1.2.0"'
check "sets the outprint image" "$(grep 'taskmedia/outprint:' <<< "$chart")" "      image: ghcr.io/taskmedia/outprint:1.2.0"
check "sets the injector image" "$(grep 'outprint-injector:' <<< "$chart")" "      image: ghcr.io/taskmedia/outprint-injector:1.2.0"
check "replaces old changes with one entry" \
  "$(sed -n '/artifacthub.io\/changes/,/artifacthub.io\/links/p' <<< "$chart")" \
  "  artifacthub.io/changes: |
    - kind: changed
      description: Bump Outprint to 1.2.0
  artifacthub.io/links: |"
check "keeps the annotation after the changes" "$(tail -2 <<< "$chart")" "    - name: support
      url: https://github.com/taskmedia/helm/issues"

before="$(cat "$work/Chart.yaml")"
"$here/helm-bump-chart.sh" "$work" 1.2.0 > /dev/null
check "is a no-op when already at the version" "$(cat "$work/Chart.yaml")" "$before"

write_chart
"$here/helm-bump-chart.sh" "$work" 1.2.0-rc.1 > /dev/null 2>&1
check "rejects a pre-release version" "$?" "1"

exit "$failures"
