#!/usr/bin/env bash
# THE HALF `open` AND `archive` SHARE, ONCE THE PULL REQUEST IS RESOLVED.
#
# The two jobs find the pull request differently -- `open` from the
# workflow_run's own head sha, `archive` from a merged-pull-request search on
# the pushed commit -- but from a resolved number on, the work is identical:
# refuse anything that is not a same-repo `change/*` branch (a merge queue can
# land a fork's pull request too), read `Refs #<n>` from its body, and carry
# the grant. Duplicating that here once, called from both jobs, rather than
# twice in the workflow YAML, is what keeps a fix to either half from landing
# in only one of them.
set -euo pipefail

[ $# -eq 2 ] || { echo "usage: carry-from-pr.sh <pr-number> <fix|archive>" >&2; exit 64; }
pr="$1"
station="$2"

head_repo=$(gh pr view "$pr" --repo "$GITHUB_REPOSITORY" --json headRepositoryOwner,headRefName \
  --jq '.headRepositoryOwner.login + "/" + .headRefName')
case "$head_repo" in
  "${GITHUB_REPOSITORY%%/*}"/change/*) ;;
  *)
    echo "pull request #$pr is not a same-repo change/* branch; nothing to carry"
    exit 0
    ;;
esac

body=$(gh pr view "$pr" --repo "$GITHUB_REPOSITORY" --json body --jq '.body')
issue=$(printf '%s' "$body" | grep -oE 'Refs #[0-9]+' | head -1 | grep -oE '[0-9]+' || true)
if [ -z "$issue" ]; then
  # AN ARCHIVE PULL REQUEST CARRIES Closes, NEVER Refs -- this is how the
  # archive job tells the last station of a lane from every other merge, and
  # does nothing on it. On `open` it means the pull request's own body never
  # named an issue at all.
  echo "pull request #$pr's body carries no 'Refs #<n>'; nothing to carry"
  exit 0
fi

if [ "$station" = "fix" ]; then
  python3 .github/scripts/carry-grant.py --repo "$GITHUB_REPOSITORY" \
    --issue "$issue" --station fix --pr "$pr"
else
  python3 .github/scripts/carry-grant.py --repo "$GITHUB_REPOSITORY" \
    --issue "$issue" --station archive
fi
