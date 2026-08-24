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

count=0; issues=0; branches=0
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
  elif [ -n "$DRY" ]; then
    printf 'issue:would open\n'
    issues=$((issues + 1))
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
[ -n "$DRY" ] && exit 0

cat <<'EOF'

The sidecars just written are UNTRACKED. Commit them on each change's own
branch, not on the default branch — that is the flow this migration exists to
start, and the commit hook now enforces it.
EOF
