# Deploying with Docker Compose

`deploy/compose/compose.yaml` runs the Gateway and the Worker from the
`renderer` image. The `worker` service is the shipped Worker baseline of
[ADR 0001](adr/0001-split-renderer-into-gateway-and-worker.md).

## Run it

```sh
export OUTLINE_URL=https://wiki.example.test      # your Outline
# optional: a token shared by Gateway and Worker
export RENDERER_WORKER_TOKEN="$(openssl rand -hex 32)"

docker compose -f deploy/compose/compose.yaml up --build
```

The Gateway listens on port 8080 (`RENDERER_PORT` changes the published port).
Download a Document as PDF while signed in to Outline:

```
GET http://localhost:8080/render/<documentId>
```

The browser must send the user's Outline session cookie, so in a real
deployment the Gateway is served from the same site as Outline (the injector
does this later). Check the file with `docker compose -f
deploy/compose/compose.yaml config`.

`./templates` and `./configurations` of the repository are mounted read-only
into both services at the paths of the shipped `service.yaml`. Replace the
sources to install your own packages; the service configuration inside the
image lists the allowed Bindings and the default Binding.

## The Worker baseline

| Setting | Why |
|---------|-----|
| `user: "10001:10001"` | fixed non-root user; the Worker also refuses to start as UID 0 |
| `security_opt: no-new-privileges:true` | no privilege gain through setuid binaries |
| no `seccomp` entry | Docker's default profile (`RuntimeDefault`) applies; never `unconfined` |
| `cap_drop: [ALL]` | the Worker needs no capability |
| `read_only: true` | the root file system cannot be written |
| `tmpfs: /work`, mode `0700`, `uid=10001`, 256 MiB | the job directories (`RENDERER_WORK_DIR`); gone when the container stops |
| `tmpfs: /tmp` | scratch space for tools |
| `networks: [worker-net]`, `internal: true` | no route out: no internet, no Outline, no host |
| no `ports` | the Worker is reachable from the Gateway only |
| no secrets | the only environment variable that looks like one is the optional `RENDERER_WORKER_TOKEN` |
| package mounts `read_only: true` | a package cannot be changed from inside |
| `mem_limit: 1g`, `cpus: 1`, `pids_limit: 128` | a resource bomb ends at the container |
| `restart: always` | the Worker exits after a timeout or kill; this gives the next job a clean process |

The baseline is enforced by the build: `internal/sandbox` reads this file,
fails if any row above is weakened or if the service uses a key it does not
know, and the security suite runs the Worker with exactly these settings
([security.md](security.md)).

At start the Worker logs its posture (`worker security posture`); anything
weaker than this table is logged as `worker posture is weaker than the shipped
baseline`.

The Gateway is also non-root with all capabilities dropped and a read-only
root. It is on `worker-net` (to reach the Worker) and on `frontend` (to publish
its port and reach Outline). It needs `OUTLINE_URL` and refuses to start
without it.

## Scaling

Throughput scales by Worker replicas, not by parallel jobs in one Worker. One
slow render delays others; a busy Worker answers `429` with `Retry-After`
through the Gateway. The Gateway's admission control (slots, a queue, a
per-user limit) sits in front of the Workers, see [limits.md](limits.md); raise
its `slots` together with the number of Worker replicas.
