# Security

The renderer turns documents written by many people into PDFs with a TeX
toolchain, so it assumes a hostile Document. The reasoning and the split into
Gateway and Worker are in
[ADR 0001](adr/0001-split-renderer-into-gateway-and-worker.md). This page lists
the layers, what each one proves, the limits and how to run the tests.

The principle: **the Worker's container sandbox is the security boundary, the
filters are not.** The filters and the engine lockdown make a hostile Document
harmless in practice; the sandbox makes sure that even a pipeline that failed
at that cannot read secrets or other jobs, reach the network, or outlive its
limits.

## Layers

| Layer | What it proves | Tier | Where |
|-------|----------------|------|-------|
| 1 reader and filters | A corpus of hostile Documents (TeX, math, raw attributes, HTML, links, image paths, metadata values) is typeset as text, dropped, or rejected with a structured error. No file named by a payload is read, no Lua or shell runs. Engine lockdown (paranoid reads and writes, no shell escape). | 2, in the golden image | `internal/golden/security_test.go`, `internal/worker` |
| 2 sandbox | With the container flags of the shipped Compose file and an intentionally unsafe pipeline, a hostile job cannot read a random canary file, another container's env secret, or another job's directory; cannot reach a host or container listener, the internet or DNS; cannot write outside the tmpfs; cannot gain privileges, mount, `mknod`, `ptrace` or execute from a tmpfs; and a fork bomb stops at the pids limit. | 2, needs Docker | `internal/sandbox` |
| 3 resource bombs | A TeX infinite loop, a memory bomb, a page bomb, an oversized PDF, a full tmpfs, a fork bomb and hostile Documents (huge tables, deep nesting, giant cells, 100 000 lines) each end within the configured limit with a structured error, the Worker is restarted where it should be, and the next job renders clean. | 2, needs Docker | `internal/sandbox` |
| 4 Gateway | SSRF guard, cookie handling, error mapping without leaks, admission control. | 1 | `internal/gateway` |
| 5 packages | Template deny-list and typed-pattern injection. | 1 | `internal/packages` |

Layers 2 and 3 never skip: a missing Docker daemon or image fails the run, and
CI fails if any test of the security suite reports a skip.

## The shipped baseline

`deploy/compose/compose.yaml`, service `worker` (see
[deploy-compose.md](deploy-compose.md)):

- fixed non-root user `10001:10001`, `no-new-privileges`, all capabilities
  dropped, Docker's default seccomp profile, read-only root file system
- tmpfs for the job directory (`/work`, mode 0700, 256 MiB) and `/tmp` (64 MiB);
  Docker mounts them `noexec,nosuid,nodev`
- an `internal` network: no route out, no published port
- environment: only `RENDERER_LISTEN`, `RENDERER_WORK_DIR` and the optional
  `RENDERER_WORKER_TOKEN`
- read-only package mounts and nothing else
- `mem_limit: 1g`, `cpus: 1`, `pids_limit: 128`, `restart: always`

### Weakening it breaks the build

`internal/sandbox` parses the Compose file with the same YAML the runtime
reads and does two things with it:

1. **Baseline check** (tier 1, part of `make test`). Every row above is
   asserted, and each failure names the Compose key (`cap_drop: must contain
   ALL`). A key the harness does not know, such as `privileged`, `cap_add`,
   `network_mode` or `devices`, is a failure too, so a new key has to be
   translated and asserted before it can ship.
2. **Translation to `docker run`** (layers 2 and 3). The flags the tests run
   the Worker with are generated from the file: `--user`, `--security-opt`,
   `--cap-drop`, `--read-only`, `--tmpfs`, `-v ...:ro`, `-e`, `--network`
   (created with `--internal` when the file says `internal: true`),
   `--memory`, `--cpus`, `--pids-limit`, `--restart`. The tests read the limits
   back from the container's cgroup and compare them with the constants in
   `internal/limits`.

## The unsafe pipeline

Layers 2 and 3 use the `sandbox` stage of the Dockerfile: the release image
plus `poppler-utils` and one test binary, `cmd/sandbox-probe`. The probe is
installed twice:

- as `sandbox-probe`, which `docker run --entrypoint sandbox-probe` runs
  directly to execute the hostile checks and to start the helper containers;
- as `/usr/local/bin/lualatex`, ahead of the real one on `PATH`. A Document
  containing `probeaction-<name>` picks a hostile action; every other Document
  gets the real LuaLaTeX. So the rest of the pipeline (job directory, Pandoc,
  filters, process groups, timeouts, limits, error mapping) is the shipped
  code.

**The release image has no probe.** `TestReleaseImageHasNoProbe` runs a check
against the release image (no `sandbox-probe`, no `lualatex` in
`/usr/local/bin`, `lualatex` resolves to the distribution's) and the same check
against the sandbox image, where it must fail.

Canaries are random and generated at test time on the host; none is mounted.
The probe never prints file content, only sizes and digests, and the tests
assert that no canary value appears in any output. Each attack also has a
positive control that proves the attack works without the sandbox (the
canary is readable when mounted, a secret in the environment is detected, a
host listener is reachable from a normal container), so a passing test is not
an accident of the harness.

## Limits

One location in code, `internal/limits`; the values are the starting limits of
the specification and are measured before release.

| Limit | Value | Enforced by | Test knob |
|-------|-------|-------------|-----------|
| wall clock per job | 60 s | Worker pipeline: the process group is killed, the Worker exits | `RENDERER_TIMEOUT` |
| Markdown | 1 MiB | bundle validation (413 `too_large`) | |
| pages | 500 | Worker pipeline after LuaLaTeX (413 `pages_exceeded`) | `RENDERER_MAX_PAGES` |
| PDF size | 50 MiB | Worker pipeline (413 `pdf_too_large`) | `RENDERER_MAX_PDF_BYTES` (lower only) |
| LuaLaTeX runs | 2 | Worker pipeline | |
| memory | 1 GiB | container (`mem_limit`) | |
| CPU | 1 | container (`cpus`) | |
| processes and threads | 128 | container (`pids_limit`) | |
| job directory | 256 MiB | container (tmpfs size) | |

The application checks first, the container is authoritative: a tool killed
for memory is reported as `resource`; a full job directory is reported as
`resource`; a job that outruns the clock is `timeout`. After `timeout` and
`resource` the Worker answers, then exits non-zero; `restart: always` gives the
next job a new process and an empty tmpfs. The Gateway maps `pages_exceeded`,
`pdf_too_large` and `resource` to 413 with a sentence that tells the author
what to change, `timeout` to 504 (see [architecture.md](architecture.md)).

A page count is only known after LuaLaTeX has finished, so the wall clock is
what bounds a document that makes pages endlessly.

## Known limits of the guarantees

- **Pid exhaustion can take the Worker down with the job.** The 128 limit
  counts threads of the Worker process too. A fork bomb that fills it can
  prevent the Worker from answering; the client then sees a lost connection,
  not a structured error, and the restart policy recovers. The layer 3 fork
  bomb test accepts both outcomes and requires the recovery.
- **The shared token is not secret from a compromised job.** The optional
  `RENDERER_WORKER_TOKEN` is in the Worker process's environment, which a
  process of the same user can read through `/proc`. The tools get a rebuilt
  environment without it and TeX's paranoid reads keep it away from `/proc`,
  but the container sandbox itself does not protect it. It guards the hop from the Gateway on
  an internal network; do not reuse it anywhere else.
- The probe runs as the Worker's user in the Worker's container. It shows what
  that user can do with the shipped flags; it is not a kernel or runtime
  escape test.

## Running the tests

```sh
make test              # tier 1, includes the Compose baseline check
make test-security     # layers 1-3 (builds the images; needs Docker)
make test-sandbox      # layers 2 and 3 only
```

`make test-sandbox` builds the release image and the `sandbox` stage and runs
`go test -tags sandbox ./internal/sandbox/...` with `SANDBOX_IMAGE` and
`RELEASE_IMAGE` pointing at them. The skip guard, `scripts/security-guard.sh`,
reads the test output and fails on any skip or when nothing ran. CI uses
`-json` (`make test-security GOTEST_FLAGS=-json`); see
[testing.md](testing.md).
