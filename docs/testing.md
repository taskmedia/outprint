# Testing

## Tiers

| Tier | Command | Needs | Covers |
|------|---------|-------|--------|
| 1 | `make test` | Go | Unit tests, the Gateway HTTP API with fakes, the Compose baseline check, the Gateway half of the SVG attack corpus |
| 2 | `make test-golden` | Docker | PDF goldens, the Worker tests and security layer 1 inside the pinned renderer image |
| 2 | `make test-sandbox` | Docker | Security layers 2 and 3: the Worker under the shipped container flags (see [security.md](security.md)) |
| 3 | not yet | Docker, browser | Browser end-to-end tests against the Compose stack |

`make test-security` runs layers 1 to 3. No test of tier 2 skips: a missing
tool, image or Docker daemon fails the run.

## CI

`.github/workflows/test.yml` runs tier 1 and the golden tests on every push and
pull request. `.github/workflows/security.yml` runs the security suite as its
own job, named **`security (layers 1-3)`**:

1. `make test-security GOTEST_FLAGS=-json` builds the golden, release and
   sandbox images and runs layer 1 (golden image) and layers 2 and 3
   (`go test -tags sandbox -json`). The output is kept in `.security-logs/`.
2. `scripts/security-guard.sh` fails the job if any logged test was skipped
   (`"Action":"skip"` in the JSON, `--- SKIP` in text) or if no test passed.
   It runs as the last step with `if: always()`, so a test that failed to
   report cannot hide.

### Required check

The maintainer must mark **`security (layers 1-3)`** as a required status check
in the branch protection rule of `main`. The repository settings are not
changed by the code or the workflow. The job name is the check name; keep it
stable.

### Cadence decision

The specification asks for layers 2 and 3 on every push while the whole
security suite stays under about three minutes, and otherwise for a path
filter plus `main` plus a weekly run.

Measured on a developer machine with a warm Docker layer cache (Docker Desktop,
10 CPUs, 8 GB):

| Step | Time |
|------|------|
| layers 2 and 3 (`go test -tags sandbox -parallel 4`) | about 50 s |
| whole `make test-security` (layer 1, release and sandbox images, layers 2 and 3) | about 1 min 32 s |

On the GitHub runner the job took about 6 to 7 minutes, almost all of it the
cold image builds (the `goldens` job takes as long for the same reason); the
tests themselves are the 50 to 90 seconds above. The job runs in parallel with
the others and is kept on every push and pull request for now; the maintainer
should decide whether the wall clock justifies the filter below. The cost that grows with the repository is the image build on a
cold runner: the job builds the same images as the `goldens` job and runs in
parallel with it. If the wall clock of the job passes three minutes in CI, move
the triggers of `security.yml` to this filter, which is already written there
as a comment: paths `internal/worker/**`, `internal/workerserver/**`,
`internal/limits/**`, `internal/sandbox/**`, `cmd/**`, `deploy/**`,
`Dockerfile`, `templates/**`, plus `push` to `main` and a weekly `schedule`.
Because a required check that never runs blocks a pull request, keep the job
required only if it runs for every pull request; use a path-aware "always
green when skipped" step if the filter is introduced.

Parallelism inside the suite is `SANDBOX_PARALLEL` (default 4). Each Worker
container is limited to one CPU and 1 GiB, so a runner with fewer than four
cores slows the suite down but does not change what it proves.

## Skip guard

```sh
make test-guard        # tests scripts/security-guard.sh
scripts/security-guard.sh .security-logs/*.log
```
