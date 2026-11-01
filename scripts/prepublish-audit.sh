#!/usr/bin/env bash
# Pre-publish audit. Run by the maintainer before making the repository
# public. It does not change any repository setting or visibility.
#
#   1. task sanitize (working tree, history, refs, secrets)
#   2. every issue and pull request on GitHub: titles, bodies, comments,
#      review comments, commit messages; release notes; repository
#      description and topics - matched against the generic denylist and the
#      private one
#   3. repository hygiene: LICENSE, THIRD_PARTY_NOTICES, large tracked files
#   4. a reminder about branch protection (informational)
#
# Environment:
#   SANITIZE_DENYLIST       optional private denylist outside the repository.
#                           Its patterns are never printed.
#   .sanitize/audit-allow.txt
#                           reviewed comments that stay as they are: one
#                           "issuecomment-<id>  # reason" per line. An allowed
#                           item is reported as a note and not scanned.
#   AUDIT_SKIP_SANITIZE=1   skip step 1 (used by the script's own test)
#   AUDIT_SKIP_GITHUB=1     skip step 2 (no network)
#   AUDIT_MAX_FILE_BYTES    size limit for tracked files (default 1048576)
#   AUDIT_LIMIT             how many issues/PRs to fetch (default 1000)
#
# Needs: gh (authenticated), jq, git. Exits 1 when there is any finding.
set -uo pipefail

root="$(git rev-parse --show-toplevel)" || exit 2
cd "$root"

generic=".sanitize/denylist.generic.txt"
allow=".sanitize/audit-allow.txt"
max_bytes="${AUDIT_MAX_FILE_BYTES:-1048576}"
limit="${AUDIT_LIMIT:-1000}"

failures=0
fail() { echo "FINDING: $*"; failures=$((failures + 1)); }
note() { echo "note: $*"; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

patterns() { grep -vE '^\s*(#|$)' "$1"; }
patterns "$generic" > "$tmp/generic"
: > "$tmp/allow"
[[ -r "$allow" ]] && sed -E 's/[[:space:]]*#.*$//; /^[[:space:]]*$/d' "$allow" > "$tmp/allow"
: > "$tmp/private"
if [[ -n "${SANITIZE_DENYLIST:-}" ]]; then
  if [[ ! -r "$SANITIZE_DENYLIST" ]]; then
    echo "FAIL: SANITIZE_DENYLIST is set but not readable"
    exit 2
  fi
  patterns "$SANITIZE_DENYLIST" > "$tmp/private"
else
  note "SANITIZE_DENYLIST not set; matching against the generic denylist only"
fi

# check_text <where> <text>: reports which list matched, never the pattern or
# the matched text.
check_text() {
  local where="$1" text="$2"
  [[ -n "$text" ]] || return 0
  if [[ -s "$tmp/allow" ]] && grep -qF -f "$tmp/allow" <<<"$where"; then
    note "$where is allowed by $allow; not scanned"
    return 0
  fi
  if [[ -s "$tmp/generic" ]] && grep -qEi -f "$tmp/generic" <<<"$text"; then
    fail "$where matches the generic denylist"
  fi
  if [[ -s "$tmp/private" ]] && grep -qEi -f "$tmp/private" <<<"$text"; then
    fail "$where matches the private denylist"
  fi
}

# scan_records: reads "where<TAB>base64(text)" lines on stdin.
scan_records() {
  local where b64
  while IFS=$'\t' read -r where b64; do
    [[ -n "$where" ]] || continue
    check_text "$where" "$(printf '%s' "$b64" | base64 -d 2>/dev/null)"
  done
}

# gh_json <what> <outfile> <command...>: runs the command into outfile and
# fails the audit when it errors or prints something that is not JSON, so an
# unreadable list can never pass as "nothing found".
gh_json() {
  local what="$1" out="$2"; shift 2
  if ! "$@" > "$out" 2>"$tmp/gh.err"; then
    fail "could not read $what via gh ($(head -c 200 "$tmp/gh.err" | tr '\n' ' '))"
    return 1
  fi
  if ! jq -e . "$out" >/dev/null 2>&1; then
    fail "gh returned something that is not JSON for $what"
    return 1
  fi
}

# 1. sanitize
if [[ "${AUDIT_SKIP_SANITIZE:-0}" != 1 ]]; then
  echo "== task sanitize"
  if ! ./scripts/sanitize.sh; then
    fail "task sanitize reported findings (see above)"
  fi
fi

# 2. GitHub content
if [[ "${AUDIT_SKIP_GITHUB:-0}" != 1 ]]; then
  echo "== issues"
  if gh_json issues "$tmp/issues.json" gh issue list --state all --limit "$limit" \
        --json number,title,body,comments; then
    n="$(jq length "$tmp/issues.json")"
    echo "scanned $n issue(s)"
    (( n >= limit )) && fail "issue list reached the limit of $limit; raise AUDIT_LIMIT"
    scan_records < <(jq -r '.[] | . as $i |
      ( ["issue #\($i.number) title", $i.title],
        ["issue #\($i.number) body", $i.body],
        ( $i.comments[]? | ["issue #\($i.number) comment by \(.author.login // "unknown")\(if .url then " [" + (.url | split("#") | last) + "]" else "" end)", .body] ) )
      | [.[0], ((.[1] // "") | @base64)] | @tsv' "$tmp/issues.json")
  fi

  echo "== pull requests"
  # One call per pull request: a single list call with commits and their
  # authors exceeds GitHub's GraphQL node limit on a repository with many PRs.
  if gh_json "pull request numbers" "$tmp/prnums.json" gh pr list --state all --limit "$limit" \
        --json number; then
    : > "$tmp/prs.ndjson"
    n=0
    while IFS= read -r num; do
      [[ -n "$num" ]] || continue
      if gh_json "pull request #$num" "$tmp/pr.json" gh pr view "$num" \
            --json number,title,body,comments,reviews,commits; then
        jq -c . "$tmp/pr.json" >> "$tmp/prs.ndjson"
        n=$((n + 1))
      fi
    done < <(jq -r '.[].number' "$tmp/prnums.json")
    echo "scanned $n pull request(s)"
    (( n >= limit )) && fail "pull request list reached the limit of $limit; raise AUDIT_LIMIT"
    scan_records < <(jq -r '. as $p |
      ( ["PR #\($p.number) title", $p.title],
        ["PR #\($p.number) body", $p.body],
        ( $p.comments[]? | ["PR #\($p.number) comment\(if .url then " [" + (.url | split("#") | last) + "]" else "" end)", .body] ),
        ( $p.reviews[]? | ["PR #\($p.number) review", .body] ),
        ( $p.commits[]? | ["PR #\($p.number) commit \(.oid[0:7])", "\(.messageHeadline)\n\(.messageBody)"] ),
        ( $p.commits[]? | .authors[]? | ["PR #\($p.number) commit author", "\(.name) <\(.email)>"] ) )
      | [.[0], ((.[1] // "") | @base64)] | @tsv' "$tmp/prs.ndjson")
  fi

  echo "== review comments (inline)"
  if gh_json "review comments" "$tmp/rc.json" gh api --paginate "repos/{owner}/{repo}/pulls/comments"; then
    # --paginate prints one JSON array per page; -s merges them.
    scan_records < <(jq -rs 'add // [] | .[] |
      ["review comment on PR \(.pull_request_url | split("/") | last) (id \(.id))", (.body // "" | @base64)] | @tsv' \
      "$tmp/rc.json")
  fi

  echo "== releases"
  if gh_json releases "$tmp/rel.json" gh api --paginate "repos/{owner}/{repo}/releases"; then
    scan_records < <(jq -rs 'add // [] | .[] |
      ( ["release \(.tag_name) name", (.name // "" | @base64)],
        ["release \(.tag_name) notes", (.body // "" | @base64)] ) | @tsv' "$tmp/rel.json")
  fi

  echo "== repository description and topics"
  if gh_json "repository metadata" "$tmp/repo.json" gh repo view --json description,repositoryTopics; then
    scan_records < <(jq -r '( ["repository description", (.description // "" | @base64)],
             ( (.repositoryTopics // [])[]? | ["repository topic", ((.name // .topic.name // "") | @base64)] ) ) | @tsv' \
      "$tmp/repo.json")
  fi
fi

# 3. Hygiene
echo "== repository hygiene"
[[ -f LICENSE ]] || fail "LICENSE is missing"
if [[ -f LICENSE ]]; then
  grep -qi "MIT License" LICENSE || fail "LICENSE is not the MIT License (README and notices say MIT)"
  grep -q "(LICENSE)" README.md 2>/dev/null && grep -qi "MIT" README.md \
    || fail "README.md does not point to the MIT LICENSE"
fi
[[ -f THIRD_PARTY_NOTICES ]] || fail "THIRD_PARTY_NOTICES is missing"

while IFS= read -r -d '' f; do
  [[ -f "$f" ]] || continue
  size="$(wc -c < "$f" | tr -d ' ')"
  if (( size > max_bytes )); then
    fail "tracked file $f is $size bytes (limit $max_bytes)"
  fi
done < <(git ls-files -z)

# 4. Branch protection (informational: the settings are changed by hand)
echo "== branch protection"
if [[ "${AUDIT_SKIP_GITHUB:-0}" != 1 ]]; then
  branch="$(gh repo view --json defaultBranchRef --jq .defaultBranchRef.name 2>/dev/null || true)"
  if [[ -n "$branch" ]] && gh api "repos/{owner}/{repo}/branches/$branch/protection" >/dev/null 2>&1; then
    note "branch protection is enabled on $branch; confirm the required checks in docs/publishing.md"
  else
    note "no branch protection readable on ${branch:-the default branch}; set it up by hand (docs/publishing.md)"
  fi
fi

echo
if (( failures > 0 )); then
  echo "audit: $failures finding(s)"
  exit 1
fi
echo "audit: clean"
