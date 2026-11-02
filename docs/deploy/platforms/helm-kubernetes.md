---
sidebar_label: Helm and Kubernetes
sidebar_position: 2
---

# Helm and Kubernetes

The Helm chart runs the Gateway, the Worker and, if you want the
[Export menu](../ways/export-menu.md), the injector. The Worker keeps the
baseline of [ADR 0001](../../adr/0001-split-renderer-into-gateway-and-worker.md)
as it is set in the [Compose file](docker-compose.md).

The chart is not part of this repository. It is released from
[taskmedia/helm](https://github.com/taskmedia/helm/tree/main/charts/outprint) and
published at [helm.task.media](https://helm.task.media) and as an OCI artifact
at `oci://ghcr.io/taskmedia/outprint`. The chart README and `values.yaml` list
every option.

## Prerequisites

- **A self-hosted Outline.** Hosted Outline (the cloud service of the Outline
  company) cannot be used: you cannot put anything in front of it. See the
  [common base](../common-base.md).
- **The injector sits between your edge proxy and Outline.** The edge proxy (your
  ingress controller, a load balancer) sends all traffic for the Outline host
  name to the injector Service, and the injector forwards to Outline. That keeps
  the browser on one address, so Outline's cookie reaches the Gateway. If you
  only want [PDF by link](../ways/pdf-by-link.md), set `injector.enabled=false` and route
  `/pdf` on Outline's address to the Gateway Service instead.
- **Helm 3** and a cluster.
- **The images** `ghcr.io/taskmedia/outprint` and
  `ghcr.io/taskmedia/outprint-injector`, which the release workflow builds. The
  chart uses its `appVersion` as the image tag by default; set `image.tag` and
  `injector.image.tag` to use another release.
- **A CNI that enforces NetworkPolicy**, if you want the Worker cut off from
  the network. Without it the policy is created but has no effect.

## Install

```sh
helm repo add taskmedia https://helm.task.media
helm repo update
helm upgrade --install outprint taskmedia/outprint \
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
it. Do not expose the Gateway or the Worker ([Security notes](../operate/security-for-operators.md)).

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

## Scaling and limits

Throughput scales by Worker replicas: `worker.replicas`. The chart sets the
Gateway's admission slots to the same number, as the
[admission control](../components/limits-and-admission.md) expects. The queue depth, the
per-user limit and the maximum wait are in `serviceConfig.admission`. Other
settings, such as `OUTPRINT_TIMEOUT` or the `OUTPRINT_BUTTON_*` options of
[The script tag](../components/script-tag.md#options-come-from-the-gateway), go into `worker.env`
and `gateway.env` (`injector.env` for the injector's own settings). If `outline.url` is an internal address, also set
`outline.publicUrl` ([Image fetching](../components/image-fetching.md)).

## Check that it works

```sh
kubectl -n outprint get pods                    # all Ready
kubectl -n outprint logs deploy/outprint-worker # "worker security posture", no "weaker than" warning
```

Then follow [Check that it works](../ways/export-menu.md#check-that-it-works) of the Export
menu way. The chart's CI installs it into a kind cluster. That does not sign in
to Outline, so the Gateway's path through Outline is covered by the
[browser tests](../../internals/development.md) of the Compose stack, not by the
chart test. It also does not check that a cluster's CNI enforces the NetworkPolicy.
