#!/usr/bin/env bash
# MOVE THE CHANGES ALREADY IN FLIGHT ONTO THE NEW FLOW.
#
# Every active openspec change gets the two things the flow assumes: a tracking
# issue and a branch named for it. Not a worktree -- that belongs to whoever
# picks the change up, and eleven idle working copies would be eleven trees to
# keep current for no one.
#
# A SCRIPT AND NOT A SEQUENCE TYPED ELEVEN TIMES. Eleven repetitions is where
# one gets done differently and nobody notices which; a month later the odd one
# out looks like a decision rather than a slip.
#
# IDEMPOTENT ON BOTH HALVES. `opsx-issue.sh` returns the existing number when the
# sidecar is present, and an existing branch is left exactly where it is -- a
# branch that has moved on is somebody's work in progress, and resetting it to
# master is the one thing this must never do.
#
# AND THE SIDECAR IS A CACHE, NOT THE SOURCE OF TRUTH. It is committed on the
# change's OWN branch, by this script's own instruction, so from the default
# branch it does not exist -- and a fresh clone, a cleaned tree or a CONCURRENT
# SESSION holding the change in its worktree all present the same blank. On
# 2026-08-25 the first dry-run of this script read `issue:would open` for a
# change whose issue had been opened minutes earlier by another session; only
# the dry-run stopped a duplicate.
#
# So GitHub is asked before an issue is opened: an `opsx:`-labelled issue whose
# title names this change, or whose body points at its directory, is ADOPTED and
# its number written back to the sidecar.
#
# THE ONE CASE THIS CANNOT SEE is a PROMOTED issue. Promotion keeps the
# reporter's own title and adds the pointer as a COMMENT rather than a body, so
# neither test matches -- and that is the case where a duplicate costs the most,
# because the reporter is waiting in that thread. A change with no sidecar and
# no match is therefore WARNED about rather than quietly given a second issue.
#
# RUN IT AFTER THE FLOW IS MERGED, not before. Until then the commit hook is not
# active on the default branch, and eleven branches that nothing yet expects are
# just litter. `--dry-run` is safe at any time.
#
# WHAT CHANGES THE DAY THIS RUNS: the hook starts refusing commits to the default
# branch that touch a change which now owns a branch. That is the point, and it
# will surprise anybody mid-edit -- tell them before, not after.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

DRY=""
[ "${1:-}" = "--dry-run" ] && DRY=1

BASE="${BASE:-origin/master}"
git fetch --quiet origin || true

changes=$(openspec list --json | python3 -c '
import json,sys
for c in json.load(sys.stdin)["changes"]:
    print(c["name"])' | sort)

[ -n "$changes" ] || { echo "no active changes"; exit 0; }

count=0; issues=0; branches=0; promoted_warning=""
ISSUES_JSON=""; ISSUES_FETCHED=""

# FETCHED ONCE, LAZILY. Once because the list is repository-wide and identical
# for every change -- refetching it per change was one full fetch per change on
# exactly the clean-checkout case this script exists for. Lazily because a run
# where every sidecar is present needs no API call at all.
#
# RETURNS NON-ZERO WHEN THE CALL FAILED, and the caller ABORTS on that. An
# expired token, a rate limit or a dropped connection must never read as "no
# issue tracks this change": that is the duplicate this whole change exists to
# prevent, arriving through a different door and with no dry-run in between to
# catch it. `opsx-issue.sh` leaves its own `gh` calls unguarded for the same
# reason -- a real failure should stop the run loudly.
ensure_issue_list() {
  [ -n "$ISSUES_FETCHED" ] && return 0
  ISSUES_JSON=$(gh issue list --state all --limit 200 --json number,title,labels,body) || return 1
  # A successful call with no output is not a documented `gh --json` response —
  # it returns `[]` — but normalising costs nothing and the STATUS is what this
  # function judges a failure by, deliberately.
  [ -n "$ISSUES_JSON" ] || ISSUES_JSON='[]'
  ISSUES_FETCHED=1
}

# An `opsx:`-labelled issue that already tracks this change, by title or by the
# path its generated body points at. Empty output means NO MATCH; a non-zero
# STATUS means the list could not be read, and the two are never conflated.
# Empty is also what a PROMOTED issue looks like, which is why the caller warns
# rather than trusting it.
match_issue() {  # match_issue <change>
  printf '%s' "$ISSUES_JSON" | python3 -c '
import json, sys
name = sys.argv[1]
for i in json.load(sys.stdin):
    if not any(l["name"].startswith("opsx:") for l in i.get("labels") or []):
        continue
    # The colon is load-bearing: `auth:` must not match `auth-tokens: ...`.
    if i["title"].startswith(name + ":") or f"openspec/changes/{name}/" in (i.get("body") or ""):
        print(i["number"])
        break
' "$1"
}
while IFS= read -r name; do
  [ -n "$name" ] || continue
  count=$((count + 1))
  printf '%-34s' "$name"

  # --- the branch -------------------------------------------------------
  if git show-ref --verify --quiet "refs/heads/change/$name"; then
    printf ' branch:kept  '
  elif [ -n "$DRY" ]; then
    printf ' branch:would '
    branches=$((branches + 1))
  else
    git branch "change/$name" "$BASE" >/dev/null
    printf ' branch:new   '
    branches=$((branches + 1))
  fi

  # --- the issue --------------------------------------------------------
  if [ -r "openspec/changes/$name/.github-issue" ]; then
    printf 'issue:#%s (kept)\n' "$(tr -dc '0-9' < "openspec/changes/$name/.github-issue")"
  elif ! ensure_issue_list; then
    printf 'issue:LOOKUP FAILED\n'
    cat >&2 <<EOF

FATAL: could not read this repository's issue list, so '$name' cannot be checked
for an existing tracking issue. Opening one now would risk the duplicate this
lookup exists to prevent -- an expired token or a rate limit must not read as
"nothing tracks this change". Nothing further was done.
EOF
    exit 1
  elif ! found=$(match_issue "$name"); then
    printf 'issue:LOOKUP UNREADABLE\n'
    echo >&2 "FATAL: the issue list came back unparseable while checking '$name'."
    exit 1
  elif [ -n "$found" ]; then
    # ADOPTED, and the sidecar is written back so the next run costs no API call
    # and every later command can resolve the binding locally.
    if [ -z "$DRY" ]; then
      printf '%s\n' "$found" > "openspec/changes/$name/.github-issue"
    fi
    printf 'issue:#%s (adopted)\n' "$found"
  elif [ -n "$DRY" ]; then
    printf 'issue:would open\n'
    issues=$((issues + 1))
    promoted_warning=1
  else
    n=$(.github/scripts/opsx-issue.sh open "$name")
    printf 'issue:#%s (new)\n' "$n"
    issues=$((issues + 1))
  fi
done <<EOF
$changes
EOF

echo
echo "$count change(s): $branches branch(es) and $issues issue(s) ${DRY:+would be }created"

# THE PROMOTED CASE, NAMED RATHER THAN GUESSED AT. Nothing here can tell a change
# that has never had an issue from one whose issue is a reporter's own thread, so
# the operator is told which changes are about to get a new one while there is
# still a dry-run between them and a duplicate.
# An `if`, not `[ ... ] && ...`: under `set -e` the AND-list returns 1 when there
# is nothing to warn about, and the script would exit non-zero on a clean run.
if [ -n "$promoted_warning" ]; then
  cat >&2 <<'EOF'

Some changes above would get a NEW issue. If any of them was PROMOTED from an
issue somebody filed, that issue keeps the reporter's title and its pointer is a
comment, so neither test above can see it -- and opening a second one strands the
reporter in a thread nobody reads again. Check those before running without
--dry-run.
EOF
fi
[ -n "$DRY" ] && exit 0

cat <<'EOF'

The sidecars just written are UNTRACKED. Commit them on each change's own
branch, not on the default branch — that is the flow this migration exists to
start, and the commit hook now enforces it.
EOF
