# Publishing checklist

Making the repository public is a **manual maintainer step**. Nothing in the
repository, its scripts or its workflows changes visibility or repository
settings. Work through this list in order.

## 1. Before the flip

1. Make sure `main` is green (`test`, `security`, `sanitize`, `e2e`).
2. Run the audit with your private denylist, from a clean checkout of `main`:

   ```sh
   gh auth status
   SANITIZE_DENYLIST=~/private/denylist.txt make audit
   ```

   It runs `make sanitize` (tree, history, refs, secrets) and scans every
   issue, pull request, comment, review comment, commit message, release note
   and the repository description and topics. See [Sanitisation gate](sanitize.md).
   Findings name the issue or pull request and the field, never the pattern.
   Fix each one:
   - issue or pull request text: edit it on GitHub (the audit does not edit);
   - tree or history: see [Sanitisation gate](sanitize.md); history rewrites need a
     coordinated force-push, so prefer fixing forward if the history is clean;
   - repeat until `audit: clean`.
3. Read `docs/research/` and the other docs once more by eye. The denylist
   finds known strings; it cannot find a name nobody listed.
4. Check `THIRD_PARTY_NOTICES` against the pinned images (see below), and that
   `LICENSE` is the licence you intend (MIT).
5. Nothing needs to be rotated: the repository holds no secrets (gitleaks is
   part of `make sanitize`), and the development secrets are generated locally
   by `make dev-secrets` and never committed. If a scan ever reports a real
   secret, rotate it first, then clean the history.

### Refreshing the notices

When the base image tag, the apt packages or a Go module changes:

```sh
docker build -t outprint .
docker run --rm --entrypoint dpkg-query outprint -W -f '${Package}\t${Version}\t${source:Package}\n'
go list -m all
```

Compare with `THIRD_PARTY_NOTICES`: the Pandoc and TeX Live versions in the
source offer, the librsvg version, the font package version, the Go module
versions and the dev stack pins in `deploy/dev/versions.env`. Re-read the
licence of a component that changed major version (Outline and Redis have
changed licence before).

## 2. Repository settings (by hand)

These are settings you change on GitHub; the project does not do it for you.

- **Branch protection on `main`**: require a pull request, and mark these
  checks as required (the job names are the check names; keep them stable):
  `unit tests (tier 1)`, `injector image`, `goldens (tier 2) and security layer 1`,
  `security (layers 1-3)`, `sanitize`.
  Do not require `browser e2e (tier 3)`: it only runs for pull requests that
  touch the paths in its workflow, and a required check that never reports
  blocks the other pull requests. Do not require
  `e2e with the latest Outline release` (weekly, non-blocking).
  Block force-pushes and deletions.
- **Private vulnerability reporting**: Settings, Code security, enable it, so
  reporters have a private channel. Consider adding a `SECURITY.md` that says
  to use it.
- **Secret scanning and push protection**, **Dependabot alerts**: enable.
- **Actions**: require approval for workflows from outside collaborators on
  public repositories; keep the `SANITIZE_DENYLIST` secret (fork pull
  requests only run the generic checks, see [Sanitisation gate](sanitize.md)).
- **Features**: turn off Wikis and Projects if unused; set the repository
  description and topics (the audit checks them).
- **Merge settings**: as you prefer; the commit style is Conventional Commits.

## 3. The flip

Settings, General, Danger Zone, Change visibility. GitHub will ask you to
confirm. After it:

- run `make audit` once more from a fresh clone (the audit uses `gh`, which
  sees the same content as the public);
- check that Actions run on a test pull request from a fork account;
- if you publish container images, include `THIRD_PARTY_NOTICES` in the
  release, and keep the written source offer in it valid (three years, and as
  long as the image is available).

## 4. What the audit does not do

- It does not change visibility, settings, issues, pull requests or history.
- It cannot see GitHub Discussions, wiki pages, Actions logs and artifacts,
  uploaded issue attachments, or deleted content that GitHub still caches.
  Check those by hand if they exist.
- It only matches known patterns. A new organisation-specific string needs a
  new entry in your private denylist.
