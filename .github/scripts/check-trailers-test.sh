#!/usr/bin/env bash
# Self-test for check-trailers.py: builds a scratch repo and runs the check against good and bad commits, titles
# and bodies. Each forbidden form must fail (exit 1, naming the commit); each allowed form must pass (exit 0).
#   .github/scripts/check-trailers-test.sh
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
check="$here/check-trailers.py"
repo="$(mktemp -d)"
trap 'rm -rf "$repo"' EXIT
cd "$repo"
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@example.invalid GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@example.invalid
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
git init -q -b main .
git commit -q --allow-empty -m "chore: base"
base="$(git rev-parse HEAD)"

fails=0
# expect <pass|fail> <name> <want-in-output>: runs the check on BASE..HEAD with PR_TITLE/PR_BODY from the environment.
expect() {
  local want="$1" name="$2" needle="${3:-}" out rc=0
  out="$(python3 "$check" "$base" "$(git rev-parse HEAD)" 2>&1)" || rc=$?
  if [ "$want" = pass ] && [ "$rc" -ne 0 ]; then echo "FAIL $name: expected pass, got $rc: $out"; fails=$((fails + 1)); return; fi
  if [ "$want" = fail ] && [ "$rc" -ne 1 ]; then echo "FAIL $name: expected exit 1, got $rc: $out"; fails=$((fails + 1)); return; fi
  if [ -n "$needle" ] && ! grep -qF -- "$needle" <<<"$out"; then echo "FAIL $name: output lacks '$needle': $out"; fails=$((fails + 1)); return; fi
  echo "ok   $name"
}
# commit_case <pass|fail> <name> <message>: one commit on top of base, checked, then dropped.
commit_case() {
  git reset -q --hard "$base"
  git commit -q --allow-empty -m "$3"
  expect "$1" "$2" "${4:-}"
}

unset PR_TITLE PR_BODY BASE_REF || true
export PR_TITLE="feat: a title" PR_BODY="A body.

Closes nothing."

# Allowed.
commit_case pass "plain human commit" "fix(atlas): close the socket"
commit_case pass "human commit, body, no trailers" $'feat: x\n\nWhy: because.\nSee plan/15.'
commit_case pass "the word claude alone" "docs: claude is a name in a sentence"
commit_case pass "env names" $'chore: env\n\nCLAUDE_MODEL and CLAUDE_CODE_OAUTH_TOKEN are read by the worker.'
commit_case pass "ukubi-claude-cloud outside a trailer" "docs: ukubi-claude-cloud is the cloud agent's name"
commit_case pass "a sentence with generated with" "chore: the file is generated with go generate"
commit_case pass "Signed-off-by is not forbidden" $'fix: y\n\nSigned-off-by: A Human <a@example.invalid>'

# Forbidden commit forms, one commit each.
commit_case fail "Co-Authored-By ukubi-claude" $'feat: a\n\nCo-Authored-By: ukubi-claude-cloud <noreply@bnei.dev>' "Co-Authored-By trailer"
commit_case fail "Co-Authored-By ukubi-agent" $'feat: a\n\nCo-Authored-By: ukubi-agent <noreply@bnei.dev>' "Co-Authored-By trailer"
commit_case fail "Co-Authored-By human, lower case" $'feat: a\n\nco-authored-by: A Human <a@example.invalid>' "Co-Authored-By trailer"
commit_case fail "Co-Authored-By Claude" $'feat: a\n\nCo-Authored-By: Claude <noreply@anthropic.com>' "Co-Authored-By trailer"
commit_case fail "Agent-Session" $'feat: a\n\nAgent-Session: abc123' "Agent-Session trailer"
commit_case fail "Agent-Definition" $'feat: a\n\nAgent-Definition: reviewer@abc123' "Agent-Definition trailer"
commit_case fail "Claude-Session" $'feat: a\n\nClaude-Session: abc123' "Claude-Session trailer"
commit_case fail "Generated with footer" $'feat: a\n\n🤖 Generated with [Claude Code](https://example.invalid)' "Generated with"
commit_case fail "claude.ai link" $'feat: a\n\nsee https://claude.ai/code/session_01abc' "claude.ai"
commit_case fail "claude.com link" $'feat: a\n\nsee https://www.claude.com/x' "claude.com"
commit_case fail "model id claude-opus" $'feat: a\n\nwritten by claude-opus-4-1' "model name"
commit_case fail "model name Claude Sonnet" $'feat: a\n\nwritten by Claude Sonnet' "model name"
commit_case fail "model name Opus 5" $'feat: a\n\nthanks Opus 5' "model name"
commit_case fail "model name Sonnet 4.6" $'feat: a\n\nthanks Sonnet 4.6' "model name"
commit_case fail "model name Haiku 4" $'feat: a\n\nthanks haiku-4' "model name"
commit_case fail "model id claude-3-5" $'feat: a\n\nwritten by claude-3-5-sonnet' "model name"
commit_case fail "model id claude-fable" $'feat: a\n\nwritten by claude-fable' "model name"
commit_case fail "model name Fable" $'feat: a\n\nthanks to Fable 1' "model name"
commit_case fail "subject line offence" "feat: ask claude-sonnet-4 to do it" "model name"

# The offending line and its commit are named; a clean commit around a bad one does not hide it.
git reset -q --hard "$base"
git commit -q --allow-empty -m "feat: one"
git commit -q --allow-empty -m $'feat: two\n\nAgent-Session: zzz'
bad="$(git rev-parse --short=12 HEAD)"
git commit -q --allow-empty -m "feat: three"
out="$(python3 "$check" "$base" HEAD 2>&1 || true)"
if grep -qF "commit $bad line 3: Agent-Session trailer: Agent-Session: zzz" <<<"$out"; then echo "ok   offender named with its sha and line"; else echo "FAIL offender named: $out"; fails=$((fails + 1)); fi
expect fail "bad commit in the middle of a range"

# Title and body, passed through the environment.
git reset -q --hard "$base"
git commit -q --allow-empty -m "feat: clean"
expect pass "clean commit, clean title and body"
# Prefix assignments, not subshells: a failure counted in a subshell would be lost.
PR_TITLE="Co-Authored-By: ukubi-claude-cloud <noreply@bnei.dev>" expect fail "bad title (trailer shape)" "PR title line 1"
PR_TITLE="feat: by Opus 5" expect fail "bad title (model)" "PR title line 1"
PR_BODY=$'Summary\n\n🤖 Generated with [Claude Code](https://example.invalid)' expect fail "bad body" "PR body line 3"
PR_BODY=$'Summary\n\nhttps://claude.ai/code/session_x' expect fail "bad body link" "PR body line 3"
marker="$repo/should-not-exist"
PR_BODY="\$(touch $marker); \`touch $marker\`" expect pass "body is data, never run"
if [ -e "$marker" ]; then echo "FAIL a PR body was executed"; fails=$((fails + 1)); fi

# History already on the base branch is not the PR's: BASE_REF bounds the range.
git reset -q --hard "$base"
git commit -q --allow-empty -m $'feat: old\n\nAgent-Session: old'
git update-ref refs/remotes/origin/main HEAD
tip_on_main="$(git rev-parse HEAD)"
git commit -q --allow-empty -m "feat: new and clean"
out="$(BASE_REF=main python3 "$check" "$base" HEAD 2>&1)" && echo "ok   BASE_REF skips commits already on the base branch" || { echo "FAIL BASE_REF: $out"; fails=$((fails + 1)); }
out="$(python3 "$check" "$base" HEAD 2>&1)" && { echo "FAIL without BASE_REF the old commit must count: $out"; fails=$((fails + 1)); } || echo "ok   without BASE_REF the same range fails ($tip_on_main counted)"

if [ "$fails" -ne 0 ]; then echo "$fails failure(s)"; exit 1; fi
echo "all trailer checks behave"
