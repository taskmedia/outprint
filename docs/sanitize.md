# Sanitisation gate

`make sanitize` keeps organisation-, place- and person-specific strings out of
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
SANITIZE_DENYLIST=~/private/denylist.txt make sanitize
make test-sanitize   # tests the gate itself
```

## CI

The `sanitize` workflow runs on every push and pull request. The private
denylist comes from the `SANITIZE_DENYLIST` Actions secret; pull requests from
forks run only the generic checks.

## Optional pre-commit hook

Not enforced. To enable it locally:

```sh
cp scripts/pre-commit.sample .git/hooks/pre-commit
```
