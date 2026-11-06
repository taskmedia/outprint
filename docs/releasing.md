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

5. For a **new minor release** (`X.Y.0`) the `docs` job calls
   `.github/workflows/docs.yml`, which deploys the site with the new version
   (see "Docs versions" below). Patch releases and pre-releases do not add one.

6. For a final release (no `-` suffix) the `helm` job runs after `publish` and
   opens a pull request in `taskmedia/helm` that points `charts/outprint` at the
   new release: `appVersion`, both image tags, a chart patch bump and a single
   `artifacthub.io/changes` entry (`scripts/helm-bump-chart.sh`). Merging that
   pull request makes the helm repository's own workflow publish the chart.
   The job needs the `PAT` secret with access to `taskmedia/helm` (Contents and
   Pull requests: read and write) and fails without it. Chart changes a release
   needs beyond the version (values, templates) stay a manual pull request there.

   **Auto-merge** is prepared but off. Set the repository variable
   `HELM_AUTO_MERGE` to `true` (Settings > Secrets and variables > Actions >
   Variables) and the job enables auto-merge on that pull request. Before you do,
   make the chart test (`Lint and Test Charts`) a required check on `main` of
   `taskmedia/helm`: GitHub waits only for required checks and merges at once
   when there are none.

Publishing lives in the same workflow on purpose: a tag or release created
with the default `GITHUB_TOKEN` does not trigger other workflows, so a separate
`on: push: tags` workflow would never run.

A release PR created with the default `GITHUB_TOKEN` does not run the `test`
workflow, so required checks on `main` could never pass. The workflow therefore
uses the repository secret `PAT` when it exists and falls back
to `GITHUB_TOKEN` otherwise. Create a fine-grained personal access token (of a
user who may push to the repository) with access to this repository only and
the permissions **Contents: read and write**, **Pull requests: read and write**
and **Issues: read and write**, give it an expiry, and store it as the secret
`PAT` (Settings > Secrets and variables > Actions). Put the
expiry date in your calendar: when the token expires the release PR silently
stops getting its checks.

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

## Docs versions

The site shows one docs version per **minor** release, plus `dev` (the docs on
`main`). Older versions are not stored in the repository: the site build exports
them from Git (`website/scripts/docs-versions.mjs`), so there is no copy to
create, review or keep in sync. Background: `website/README.md`, "Versions".

### What happens at a minor release

Nothing you have to do. The release workflow calls `docs.yml` (job `docs`) when
release-please created a release whose patch number is 0 and whose version has no
`-` suffix (so `0.2.0` yes; `0.2.1` and `0.2.0-rc.1` no; the final `0.2.0` after
an `-rc` does). It does not depend on the image jobs: a failing image build
neither stops it nor is stopped by it. The called workflow builds the site from
`main` with all tags, so the new tag `vX.Y.0` becomes the version `X.Y`, and
deploys it. The push that merges the release pull request touches no docs file,
which is why `docs.yml` does not start on its own.

On the site the new minor takes the root and the previous one moves to its own
`/X.Y/` folder. The version dropdown lists every minor with a tag `vX.Y.0`. The
redirects follow by themselves: links the product prints (`/<minor>/...`,
`/dev/...`) keep working.

### A version needs a docs correction

Old docs may stay as released. To correct one anyway, create the branch
`docs-release/X.Y` from the tag and push the fix there:

```sh
git switch -c docs-release/0.1 v0.1.0
# edit docs/..., commit
git push -u origin docs-release/0.1
```

From then on the site takes the version `X.Y` from that branch instead of the
tag, and every push to it deploys the site (built from `main`). Fix `docs/` on
`main` as well when it has the same mistake. A pull request into the branch gets
the usual checks. To go back to the tag, delete the branch.

### When it fails

The build fails (it does not skip the version) when a tag or branch has no
`docs/` folder, or when a version no longer passes the strict build, for example
a link check that was added after the release. Read the failed run, then
correct that version on `docs-release/X.Y` as above. A missing version in the
dropdown usually means the build had no tags: the checkout needs
`fetch-depth: 0`.

### Locally

```sh
cd website && npm ci
npm start          # or: npm run build
npm test           # the export of the older versions
```

`npm start` and `npm run build` export the versions of the tags that exist in
your clone first. Without tags only the current docs are built.

What cannot be tested before the first release: the call from `release-please.yml`
to `docs.yml` and the deploy. The first real run is the v0.1.0 release; until a
deploy from `main` has run, the live site shows the docs of `main` only.

## Audit gate

The release PR runs the audit (`.github/workflows/audit.yml`, check `audit`,
see [Sanitisation gate](sanitize.md)). With `audit` in the required checks of
`main`, a release cannot be merged while the audit has a finding. Fix findings
by hand (issue and pull request text on GitHub, files in the repository); the
audit only reads. The check needs the Actions secret `SANITIZE_DENYLIST`; without
it the check fails on purpose.

## Immutable releases

Turn on **Settings > General > Releases > Enable release immutability**. From
then on a published release cannot be changed, its assets cannot be added or
replaced, and its tag cannot be moved or deleted. Consequences:

- A version number is used up once published. A broken release is fixed by the
  next patch release, not by redoing the tag. The *publish* jobs may be re-run
  (see below), because they only push images.
- This workflow attaches no assets to the GitHub release, so it works as is.
  If assets are added later (an SBOM, a checksum file), the release must be
  created as a draft, assets attached, and then published; set
  `"draft": true` in the release-please config for that.
- Immutability covers the GitHub release and its git tag, **not the images in
  GHCR**. `X.Y` and `latest` move by design, and a registry tag such as `X.Y.Z`
  is not protected by this setting. Users who need an exact artefact should pin
  the image by digest.

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
task build VERSION=0.0.0-test
bin/outprint version
bin/outprint-injector version

task image VERSION=0.0.0-test
docker inspect outprint --format '{{json .Config.Labels}}'
docker run --rm outprint version

task image-injector VERSION=0.0.0-test
docker run --rm outprint-injector version
```

Without `VERSION`, `task` takes `git describe --tags --always --dirty`.

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
