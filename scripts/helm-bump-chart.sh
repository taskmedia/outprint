#!/usr/bin/env bash
# Points the outprint chart in taskmedia/helm at a new Outprint release.
# Usage: helm-bump-chart.sh <chart-dir> <outprint-version>   (version without "v")
#
# Sets appVersion and both artifacthub image tags, bumps the chart's patch
# version and replaces artifacthub.io/changes with a single entry: the
# annotation describes one chart version, so older entries must not carry over.
set -euo pipefail

chart_dir="${1:?usage: helm-bump-chart.sh <chart-dir> <outprint-version>}"
app_version="${2:?usage: helm-bump-chart.sh <chart-dir> <outprint-version>}"
chart_file="${chart_dir}/Chart.yaml"

if [[ ! "${app_version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "not a final X.Y.Z version: ${app_version}" >&2
  exit 1
fi

chart_version="$(sed -n 's/^version: \([0-9]*\.[0-9]*\.[0-9]*\)$/\1/p' "${chart_file}")"
if [[ -z "${chart_version}" ]]; then
  echo "no plain X.Y.Z 'version:' in ${chart_file}" >&2
  exit 1
fi

current_app_version="$(sed -n 's/^appVersion: "\(.*\)"$/\1/p' "${chart_file}")"
if [[ "${current_app_version}" == "${app_version}" ]]; then
  echo "chart already at appVersion ${app_version}"
  exit 0
fi

IFS=. read -r major minor patch <<< "${chart_version}"
next_chart_version="${major}.${minor}.$((patch + 1))"

awk -v chart="${next_chart_version}" -v app="${app_version}" '
  /^version: /                       { print "version: " chart; next }
  /^appVersion: /                    { print "appVersion: \"" app "\""; next }
  /image: ghcr\.io\/taskmedia\/outprint(-injector)?:/ {
    sub(/:[0-9][^[:space:]]*$/, ":" app)
  }
  /^  artifacthub\.io\/changes: \|/ {
    print
    print "    - kind: changed"
    print "      description: Bump Outprint to " app
    skipping = 1
    next
  }
  skipping && /^    / { next }
  { skipping = 0; print }
' "${chart_file}" > "${chart_file}.new"
mv "${chart_file}.new" "${chart_file}"

echo "${chart_version} -> ${next_chart_version}, appVersion ${current_app_version} -> ${app_version}"
