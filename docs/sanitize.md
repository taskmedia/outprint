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

## Optional pre-commit hook

Not enforced. To enable it locally:

```sh
cp scripts/pre-commit.sample .git/hooks/pre-commit
```
