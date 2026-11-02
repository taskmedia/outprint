---
sidebar_label: Helm and Kubernetes
sidebar_position: 13
---

# Helm and Kubernetes

The chart in `deploy/helm/outprint` runs the Gateway, the Worker and, if you
want the [Export menu](way-menu.md), the injector. The Worker keeps the
baseline of [ADR 0001](../adr/0001-split-renderer-into-gateway-and-worker.md)
as it is set in the [Compose file](docker-compose.md).

## Prerequisites

- **A self-hosted Outline.** Hosted Outline (the cloud service of the Outline
  company) cannot be used: you cannot put anything in front of it. See the
  [common base](common-base.md).
- **The injector sits between your edge proxy and Outline.** The edge proxy (your
  ingress controller, a load balancer) sends all traffic for the Outline host
  name to the injector Service, and the injector forwards to Outline. That keeps
  the browser on one address, so Outline's cookie reaches the Gateway. If you
  only want [PDF by link](way-link.md), set `injector.enabled=false` and route
  `/pdf` on Outline's address to the Gateway Service instead.
- **Helm 3** and a cluster. The chart is not published to a registry; install it
  from a checkout of the repository.
- **The images** `ghcr.io/taskmedia/outprint` and
  `ghcr.io/taskmedia/outprint-injector`, which the release workflow builds. The
  chart uses its `appVersion` as the image tag by default; check that the tag
  exists before you depend on it, or set `image.tag` and `injector.image.tag`.
- **A CNI that enforces NetworkPolicy**, if you want the Worker cut off from
  the network. Without it the policy is created but has no effect.

## Install

```sh
helm upgrade --install outprint ./deploy/helm/outprint \
  --namespace outprint --create-namespace \
  --set outline.url=http://outline.outline.svc:3000
```

`outline.url` is the address at which the Gateway and the injector reach
Outline; the chart refuses to render without it. Then point your edge proxy at
the Service `outprint-injector` (port 80) instead of at Outline. If you use an
ingress controller, the chart can create the Ingress:

<!-- validate: skip Helm chart values, not an outprint configuration file -->
```yaml
# values.yaml
outline:
  url: http://outline.outline.svc:3000
injector:
  ingress:
    enabled: true
    className: nginx
    hosts: [wiki.example.test]
    tls:
      - secretName: wiki-tls
        hosts: [wiki.example.test]
```

Move the host name off Outline's own Ingress first; two Ingresses must not claim
it. Do not expose the Gateway or the Worker ([Security notes](security-for-operators.md)).

## What the chart creates

| Object | Notes |
|--------|-------|
| Worker Deployment, Service, NetworkPolicy | the locked-down part, see below |
| Gateway Deployment, Service | reaches the Worker by its Service name and Outline by `outline.url` |
| Injector Deployment, Service, optional Ingress | `injector.enabled=false` leaves it out |
| ConfigMap `<release>-service` | the service configuration (`serviceConfig` in the values): Bindings, default Binding, the Template and Configuration roots, admission control |
| Secret `<release>-worker-token` | a random bearer token shared by Gateway and Worker; kept across upgrades. Use `workerToken.existingSecret` (key `token`) to bring your own, or `workerToken.enabled=false` for none |

The Templates and Configurations come with the image. To ship your own, build an
image on top of the release image and set `image.repository` and `image.tag`;
allow the pairings in `serviceConfig.bindings`.

## The Worker baseline

| Setting | In the chart |
|---------|--------------|
| non-root, fixed user | `runAsNonRoot`, UID and GID 10001 |
| no privilege gain, no capabilities | `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]` |
| seccomp | `RuntimeDefault` |
| read-only root file system | `readOnlyRootFilesystem: true` |
| job directory and `/tmp` | `emptyDir` in memory with a size limit (`worker.workDirSize`, `worker.tmpSize`), gone with the pod |
| no network | NetworkPolicy with no egress rule; ingress only from the Gateway pods on port 8081 |
| no secrets | no Kubernetes API token, no service links; the only variable from a Secret is the optional `OUTPRINT_WORKER_TOKEN` |
| limits | CPU and memory in `worker.resources` |
| restart after a timeout or kill | the Worker exits and Kubernetes restarts the container |

`worker.networkPolicy.enabled=false` removes the policy and the install notes
warn about it; do not do that on a shared cluster. Kubernetes has no per-pod
process limit: `pids_limit` of the Compose file has no counterpart in the chart,
the kubelet setting `podPidsLimit` applies instead.

`go test -tags helm ./deploy/helm/...` (`task test-helm`) renders the chart and
fails if the Worker part is weakened.

## Scaling and limits

Throughput scales by Worker replicas: `worker.replicas`. The chart sets the
Gateway's admission slots to the same number, as the
[admission control](limits-and-admission.md) expects. The queue depth, the
per-user limit and the maximum wait are in `serviceConfig.admission`. Other
settings, such as `OUTPRINT_TIMEOUT` or the `OUTPRINT_BUTTON_*` options of the
[injector page](injector.md#configuration), go into `worker.env`, `gateway.env`
and `injector.env`. If `outline.url` is an internal address, also set
`outline.publicUrl` ([Image fetching](image-fetching.md)).

## Check that it works

```sh
kubectl -n outprint get pods                    # all Ready
kubectl -n outprint logs deploy/outprint-worker # "worker security posture", no "weaker than" warning
```

Then follow [Check that it works](way-menu.md#check-that-it-works) of the Export
menu way. The CI workflow `helm` installs the chart into a kind cluster and has
the Worker render a PDF. It does not sign in to Outline, so the Gateway's path
through Outline is covered by the [browser tests](../internals/development.md)
of the Compose stack, not by the chart test. It also does not check that a
cluster's CNI enforces the NetworkPolicy.
