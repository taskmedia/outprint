# Releasing

For the maintainer. Repo-only: this page is not part of the published docs.

## How a release happens

1. Work lands on `main` through pull requests with Conventional Commit titles
   (`feat:`, `fix:`, `docs:`, `chore:` ...). Before 1.0, `feat` and a breaking
   change (`feat!:` or a `BREAKING CHANGE:` footer) bump the minor version,
   `fix` bumps the patch.
2. On every push to `main`, `.github/workflows/release-please.yml` runs
   release-please. It opens or updates one **release PR** ("chore: release
   X.Y.Z") with the version bump in `.release-please-manifest.json` and the new
   `CHANGELOG.md` section.
3. You review the release PR and merge it. release-please then creates the tag
   `vX.Y.Z` and the GitHub release.
4. In the same workflow run, the `build` jobs build both images from that tag
   on native amd64 and arm64 runners, and the `publish` jobs merge them into
   multi-arch manifests and push them to GHCR:
   - `ghcr.io/taskmedia/outprint` (Dockerfile target `outprint`)
   - `ghcr.io/taskmedia/outprint-injector` (Dockerfile target `injector`)

   Tags: `X.Y.Z`, `X.Y` and `latest`. A pre-release (a version with a `-`
   suffix) gets `X.Y.Z` only and never moves `X.Y` or `latest`.

Publishing lives in the same workflow on purpose: a tag or release created
with the default `GITHUB_TOKEN` does not trigger other workflows, so a separate
`on: push: tags` workflow would never run.

The release PR is also created with `GITHUB_TOKEN`, so the `test` workflow does
not run on it. If you require status checks on `main`, merge it as an
administrator, or give release-please a personal access token or GitHub App
token instead.

### The first release (v0.1.0)

The manifest starts at `0.0.0` and the config carries `"release-as": "0.1.0"`,
so the first release PR proposes exactly `0.1.0` whatever the commit types are.
It lists every `feat` and `fix` commit in the history (there is no earlier tag),
grouped under Features and Bug Fixes; `docs`, `chore` and similar are hidden.
Add `"bootstrap-sha": "<commit>"` to the package in `release-please-config.json`
before merging the setup, if the changelog should start from a later commit.

`release-as` is sticky: left in place, every later release PR would propose
`0.1.0` again. After the v0.1.0 release has been published, delete the
`"release-as": "0.1.0"` line from `release-please-config.json` in a
`chore:` pull request.

## One-time GitHub settings

- Settings > Actions > General > Workflow permissions: tick **Allow GitHub
  Actions to create and approve pull requests**. Without it release-please
  cannot open the release PR.
- After the first publish, the packages are private. Open each package under
  the organisation's Packages, then Package settings: set the visibility to
  **public** (this cannot be done from a workflow) and link the package to the
  `taskmedia/outprint` repository. The `org.opencontainers.image.source` label
  already points there, which GHCR uses to suggest the link.
- Arm64 builds use the `ubuntu-24.04-arm` runner, which is free for public
  repositories only. Make the repository public first.

## Dry run locally

The images build from the same `Dockerfile` the workflow uses:

```sh
make build VERSION=0.0.0-test
bin/outprint version
bin/outprint-injector version

make image VERSION=0.0.0-test
docker inspect outprint --format '{{json .Config.Labels}}'
docker run --rm outprint version

make image-injector VERSION=0.0.0-test
docker run --rm outprint-injector version
```

Without `VERSION`, `make` takes `git describe --tags --always --dirty`.

To see what release-please would propose, without writing anything (needs a
token with read access; do not paste it into shell history):

```sh
npx release-please release-pr --dry-run \
  --repo-url=taskmedia/outprint --token="$GITHUB_TOKEN" \
  --config-file=release-please-config.json \
  --manifest-file=.release-please-manifest.json
```

Lint the workflow with `actionlint`.

## When a publish job fails

The tag and the GitHub release already exist by then; only the images are
missing.

- **A single build or publish job failed, for example a flaky runner:** open
  the run in the Actions tab and use **Re-run failed jobs**. The jobs rebuild
  from the tag, so the result is the same. The layer cache is kept.
- **The Dockerfile or workflow is broken:** fix it on `main` through a pull
  request, then re-run the **release** workflow of the failed run. It checks
  out the tag, so the workflow file from the failed run is used; if that is
  not enough, delete the tag and the GitHub release, restore the manifest
  version in a pull request, and let release-please open the release PR again.
- **Login or push is denied (403):** check that the package is linked to the
  repository and that Actions have write access to it (Package settings >
  Manage Actions access).
- Never move a published `X.Y.Z` tag to different content. Cut a patch release.
