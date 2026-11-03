---
sidebar_position: 8
---

# The Worker baseline

**Background.** The security settings the Worker runs with in the Compose file and in the Helm chart, and how the build keeps them.

To run Outprint, see [Run Outprint](../install/run-outprint.mdx). The Worker keeps the
baseline of [ADR 0001](../adr/0001-split-renderer-into-gateway-and-worker.md).
It is not part of the injector, and the sandbox tests do not read the injector.

## Docker Compose

`deploy/compose/compose.yaml`, service `worker`:

| Setting | Why |
|---------|-----|
| `user: "10001:10001"` | fixed non-root user; the Worker also refuses to start as UID 0 |
| `security_opt: no-new-privileges:true` | no privilege gain through setuid binaries |
| no `seccomp` entry | Docker's default profile (`RuntimeDefault`) applies; never `unconfined` |
| `cap_drop: [ALL]` | the Worker needs no capability |
| `read_only: true` | the root file system cannot be written |
| `tmpfs: /work`, mode `0700`, `uid=10001`, 256 MiB | the job directories (`TMPDIR`); gone when the container stops |
| `tmpfs: /tmp` | scratch space for tools |
| `networks: [worker-net]`, `internal: true` | no route out: no internet, no Outline, no host |
| no `ports` | the Worker is reachable from the Gateway only |
| no secrets | the only environment variable that looks like one is the optional `OUTPRINT_WORKER_TOKEN` |
| package mounts `read_only: true` | a package cannot be changed from inside |
| `mem_limit: 1g`, `cpus: 1`, `pids_limit: 128` | a resource bomb ends at the container |
| `restart: always` | the Worker exits after a timeout or kill; this gives the next job a clean process |

The baseline is enforced by the build: `internal/sandbox` reads this file,
fails if any row above is weakened or if the service uses a key it does not
know, and the security suite runs the Worker with exactly these settings
([Security](security.md)).

At start the Worker logs its posture (`worker security posture`); anything
weaker than this table is logged as `worker posture is weaker than the shipped
baseline`.

The Gateway is also non-root with all capabilities dropped and a read-only
root. It is on `worker-net` (to reach the Worker) and on `frontend` (to publish
its port and reach Outline).

## Helm

The chart is released from
[taskmedia/helm](https://github.com/taskmedia/helm/tree/main/charts/outprint).

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

The chart's CI installs it into a kind cluster. That does not sign in to
Outline, so the Gateway's path through Outline is covered by the
[browser tests](development.md) of the Compose stack. It also does not check
that a cluster's CNI enforces the NetworkPolicy.

## Scaling

Throughput scales by Worker replicas, not by parallel jobs in one Worker. One
slow render delays others; a busy Worker answers `429` with `Retry-After`
through the Gateway. Raise the Gateway's admission `slots` together with the
number of replicas ([Tune limits](../install/operate/limits-and-admission.md)).
