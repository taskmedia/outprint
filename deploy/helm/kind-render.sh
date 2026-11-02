#!/usr/bin/env bash
# Install the chart into the current cluster (a kind cluster in CI) and render
# one PDF through the Worker. The Gateway needs a signed-in Outline user, so
# the test talks to the Worker's own endpoint; the Gateway and the injector are
# checked for readiness.
#
#   IMAGE=outprint:ci INJECTOR_IMAGE=outprint-injector:ci deploy/helm/kind-render.sh
#
# Both images must already be loaded into the cluster (kind load docker-image).
set -euo pipefail

chart="$(cd "$(dirname "$0")" && pwd)/outprint"
release=outprint-ci
namespace=outprint-ci
image="${IMAGE:?set IMAGE, for example outprint:ci}"
injector_image="${INJECTOR_IMAGE:?set INJECTOR_IMAGE, for example outprint-injector:ci}"
work="$(mktemp -d)"
pf_pid=""

cleanup() {
  [ -n "$pf_pid" ] && kill "$pf_pid" 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT

dump() {
  kubectl -n "$namespace" get pods -o wide || true
  kubectl -n "$namespace" describe pods || true
  kubectl -n "$namespace" logs -l app.kubernetes.io/instance="$release" --all-containers --prefix --tail=100 || true
}

helm upgrade --install "$release" "$chart" --namespace "$namespace" --create-namespace \
  --set outline.url=http://outline.invalid:3000 \
  --set "image.repository=${image%:*}" --set "image.tag=${image##*:}" --set image.pullPolicy=Never \
  --set "injector.image.repository=${injector_image%:*}" --set "injector.image.tag=${injector_image##*:}" \
  --set injector.image.pullPolicy=Never \
  --wait --timeout 5m || { dump; exit 1; }

worker="svc/${release}-worker"
token="$(kubectl -n "$namespace" get secret "${release}-worker-token" -o jsonpath='{.data.token}' | base64 -d)"

kubectl -n "$namespace" port-forward "$worker" 18081:8081 >"$work/pf.log" 2>&1 &
pf_pid=$!
for _ in $(seq 1 30); do
  curl -fsS http://127.0.0.1:18081/healthz >/dev/null 2>&1 && break
  sleep 1
done

cat >"$work/bundle.json" <<'EOF'
{"markdown":"# Helm install check\n\nThis PDF was typeset by the Worker of a chart install.\n",
 "values":{"title":"Helm install check"},
 "template":"neutral","documentType":"report","configuration":"example"}
EOF

curl -sS -o "$work/out.pdf" -w '%{http_code}\n' \
  -H "Authorization: Bearer ${token}" -H 'Content-Type: application/json' \
  --data-binary "@$work/bundle.json" http://127.0.0.1:18081/render | tee "$work/status"
if [ "$(cat "$work/status")" != 200 ] || [ "$(head -c 5 "$work/out.pdf")" != "%PDF-" ]; then
  echo "the Worker did not return a PDF:" >&2
  head -c 400 "$work/out.pdf" >&2 || true
  dump
  exit 1
fi

# A request without the token must be refused.
unauth="$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
  --data-binary "@$work/bundle.json" http://127.0.0.1:18081/render)"
[ "$unauth" = 401 ] || { echo "Worker accepted a request without the token (status $unauth)" >&2; exit 1; }

echo "rendered a PDF of $(wc -c <"$work/out.pdf" | tr -d ' ') bytes through the Worker"
kubectl -n "$namespace" logs deploy/"${release}-worker" | grep -E 'worker security posture|weaker than' || true
