# Sanitisation gate

`task sanitize` keeps organisation-, place- and person-specific strings out of
the repository and its history. It checks:

- the working tree (tracked files and untracked files that are not ignored),
  fixtures included
- the full git history: diffs, commit messages and authors, tags and tag
  annotations, branch names
- forbidden paths (`.sanitize/forbidden-paths.txt`), in the tree and in history
- secrets, with [gitleaks](https://github.com/gitleaks/gitleaks) when installed

Two denylists are used, one extended regular expression per line, matched
case-insensitively:

| List | Location | Purpose |
|------|----------|---------|
| generic | `.sanitize/denylist.generic.txt` (committed) | vocabulary of the proof of concept |
| private | file named by `$SANITIZE_DENYLIST`, outside the repository | organisation-specific strings |

The `.sanitize/` directory is excluded from the scan. Findings name the file or
history section and the line; private matches never echo the pattern.

```sh
SANITIZE_DENYLIST=~/private/denylist.txt task sanitize
task test-sanitize   # tests the gate itself
```

## CI

The `sanitize` workflow runs on every push and pull request. The private
denylist comes from the `SANITIZE_DENYLIST` Actions secret; pull requests from
forks run only the generic checks.

The `docs` workflow also scans the built site (next section), with the same
secret and the same fork rule.

## The built site

`task docs:sanitize-site` (`scripts/sanitize-site.sh [DIR]`, default
`website/build`) runs the generic and the private denylist over the generated
site: the content of every text file (HTML, the search index, scripts) and the
name of every file and directory (copied images included), plus the forbidden
paths. The source pages are already covered by `task sanitize`; this catches
what only the build produces. Findings name the file and the list, never the
pattern; an empty or missing directory exits 2, so a broken build step never
looks clean.

```sh
task docs:build
SANITIZE_DENYLIST=~/private/denylist.txt task docs:sanitize-site
```

In CI it runs in the job **`docs build (strict)`** right after the build. Fork
pull requests have no secret and run the generic list only; the step prints a
note. Its test is `scripts/sanitize_site_test.sh` (`task docs:test-gates`).

## Pre-publish audit

`task audit` (`scripts/prepublish-audit.sh`) is the last check before the
maintainer makes the repository public. It runs `task sanitize` and then
reads, through the authenticated `gh` CLI, content that the git history does
not hold:

- every issue: title, body, comments
- every pull request: title, body, comments, reviews, inline review comments,
  commit messages and commit authors
- release names and notes, the repository description and topics

Each text is matched against the generic denylist and `$SANITIZE_DENYLIST`
with the same rule as above: findings name the issue or pull request and the
field (`PR #12 review`, `issue #3 comment by someone`), and a private pattern
is never printed. It also checks that `LICENSE` (MIT) and `THIRD_PARTY_NOTICES`
exist, that no tracked file is larger than `AUDIT_MAX_FILE_BYTES` (default
1 MiB), and prints a note about branch protection, which is set by hand. The
exit code is 1 when there is any finding, and also when `gh` fails or returns
something that is not JSON, so a broken scan never looks clean.

```sh
gh auth login            # once
SANITIZE_DENYLIST=~/private/denylist.txt task audit
task test-audit          # tests the matching logic with a fake gh
```

The audit only reads. It never edits issues or pull requests; fix findings by
hand. Limits: `AUDIT_LIMIT` (default 1000) caps the issue and pull request
lists and the script fails when the cap is reached. Discussions, wikis and
Actions logs are not scanned. See [Publishing checklist](publishing.md) for the whole
checklist.

### The audit in CI

`.github/workflows/audit.yml` runs the same audit as a check named `audit`:

- on the **release pull request** (branch `release-please--branches--main`):
  the full audit. Add `audit` to the required checks of `main` and the release
  cannot be merged while a finding is open.
- on **every other pull request**: the job reports success at once, so a
  required `audit` check never blocks unrelated work.
- **weekly** (Monday) and on demand (`workflow_dispatch`): the full audit, to
  catch text added to issues, comments or pull requests after the last release.

It needs the Actions secret `SANITIZE_DENYLIST` (the contents of the private
denylist) and **fails without it**, so an audit that cannot see the private list
never looks clean. It reads with the workflow's own read-only token
(`contents`, `issues`, `pull-requests`) and changes nothing. Pull requests from
forks get the skip path (no secrets). The audit looks at the text as it is at
the time of the run: a comment added after the check but before the merge is
caught by the next weekly run, not by the merge.

## Optional pre-commit hook

Not enforced. To enable it locally:

```sh
cp scripts/pre-commit.sample .git/hooks/pre-commit
```
