#!/usr/bin/env bash
# Tests the matching logic of prepublish-audit.sh with a fake `gh` on PATH.
set -uo pipefail

# Forbidden words are assembled at runtime so this file itself stays clean.
w_generic="foer""der""ver""ein"
w_private="secret-term-""xyz"
here="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

cd "$work"
git init -q . && git config user.name t && git config user.email t@example.invalid
git config commit.gpgsign false
mkdir -p .sanitize scripts bin fake
cp "$here"/.sanitize/*.txt .sanitize/
cp "$here"/scripts/sanitize.sh "$here"/scripts/prepublish-audit.sh scripts/
printf 'MIT License\n' > LICENSE
printf 'Licensed under [MIT](LICENSE)\n' > README.md
echo "notices" > THIRD_PARTY_NOTICES
git add . && git commit -q -m "chore: init"

# The fake gh answers from files in $FAKE_GH_DIR.
cat > bin/gh <<'EOF'
#!/usr/bin/env bash
d="${FAKE_GH_DIR:?}"
case "$1 $2" in
  "issue list") cat "$d/issues.json" ;;
  "pr list")    jq '[.[] | {number}]' "$d/prs.json" ;;
  "pr view")    jq --argjson n "$3" '.[] | select(.number == $n)' "$d/prs.json" ;;
  "repo view")
    if [[ "$*" == *defaultBranchRef* ]]; then echo main; else cat "$d/repo.json"; fi ;;
  "api --paginate")
    case "$3" in
      *pulls/comments) cat "$d/rc.json" ;;
      *releases)       cat "$d/rel.json" ;;
      *) exit 1 ;;
    esac ;;
  api*) exit 1 ;;
  *) echo "fake gh: unexpected: $*" >&2; exit 1 ;;
esac
EOF
chmod +x bin/gh
export PATH="$work/bin:$PATH" FAKE_GH_DIR="$work/fake" AUDIT_SKIP_SANITIZE=1

reset_fake() {
  echo '[{"number":1,"title":"A title","body":"plain","comments":[{"author":{"login":"u"},"body":"fine"}]}]' > fake/issues.json
  echo '[{"number":2,"title":"A PR","body":"plain","comments":[],"reviews":[{"body":"ok"}],"commits":[{"oid":"abcdef1234","messageHeadline":"feat: x","messageBody":"","authors":[{"name":"T","email":"t@example.invalid"}]}]}]' > fake/prs.json
  echo '[{"id":9,"pull_request_url":"https://api.example.invalid/pulls/2","body":"fine"}]' > fake/rc.json
  echo '[{"tag_name":"v1","name":"v1","body":"notes"}]' > fake/rel.json
  echo '{"description":"A tool","repositoryTopics":[{"name":"pdf"}]}' > fake/repo.json
}

failed=0
expect() { # expect <0|1> <description> [output-regex]
  local want="$1" desc="$2" rx="${3:-}" out rc
  out="$(scripts/prepublish-audit.sh 2>&1)"; rc=$?
  if [[ "$rc" -ne "$want" ]] || { [[ -n "$rx" ]] && ! grep -qE "$rx" <<<"$out"; }; then
    echo "not ok - $desc (rc=$rc)"; echo "$out" | sed 's/^/    /'; failed=1
  else
    echo "ok - $desc"
  fi
}

reset_fake
expect 0 "clean repository passes" "audit: clean"

reset_fake; sed -i.bak "s/A title/see $w_generic/" fake/issues.json
expect 1 "generic string in an issue title fails, naming the issue and field" "issue #1 title matches the generic"

reset_fake; sed -i.bak "s/\"fine\"/\"$w_generic\"/" fake/issues.json
expect 1 "generic string in an issue comment fails" "issue #1 comment by u matches"

reset_fake; sed -i.bak "s/\"body\":\"plain\",\"comments\":\[\],/\"body\":\"$w_generic\",\"comments\":[],/" fake/prs.json
expect 1 "generic string in a PR body fails" "PR #2 body matches"

reset_fake; sed -i.bak "s/\"ok\"/\"$w_generic\"/" fake/prs.json
expect 1 "generic string in a PR review fails" "PR #2 review matches"

reset_fake; sed -i.bak "s/feat: x/feat: $w_generic/" fake/prs.json
expect 1 "generic string in a commit message fails" "PR #2 commit abcdef1 matches"

reset_fake; sed -i.bak "s/\"body\":\"fine\"/\"body\":\"$w_generic\"/" fake/rc.json
expect 1 "generic string in an inline review comment fails" "review comment on PR 2"

reset_fake; sed -i.bak "s/notes/$w_generic/" fake/rel.json
expect 1 "generic string in release notes fails" "release v1 notes matches"

reset_fake; sed -i.bak "s/A tool/$w_generic/" fake/repo.json
expect 1 "generic string in the repository description fails" "repository description matches"

reset_fake; echo "$w_private" > private.txt; sed -i.bak "s/plain/$w_private/" fake/issues.json
out="$(SANITIZE_DENYLIST="$work/private.txt" scripts/prepublish-audit.sh 2>&1)"; rc=$?
if [[ $rc -eq 1 ]] && grep -q "issue #1 body matches the private denylist" <<<"$out" && ! grep -q "$w_private" <<<"$out"; then
  echo "ok - private denylist hit fails and the pattern is not echoed"
else
  echo "not ok - private denylist hit (rc=$rc)"; echo "$out" | sed 's/^/    /'; failed=1
fi
rm private.txt

reset_fake; mv LICENSE LICENSE.off
expect 1 "missing LICENSE fails" "LICENSE is missing"
mv LICENSE.off LICENSE

reset_fake; mv THIRD_PARTY_NOTICES T.off
expect 1 "missing THIRD_PARTY_NOTICES fails" "THIRD_PARTY_NOTICES is missing"
mv T.off THIRD_PARTY_NOTICES

reset_fake; head -c 2000 /dev/zero > big.bin; git add big.bin
AUDIT_MAX_FILE_BYTES=1000 expect 1 "tracked file over the limit fails" "big.bin is 2000 bytes"
git rm -qf big.bin

reset_fake; echo 'not json' > fake/issues.json
expect 1 "unreadable issue list is a finding, not a silent pass" "could not|parse|Invalid|issues"

exit "$failed"
