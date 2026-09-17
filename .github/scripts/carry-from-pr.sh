#!/usr/bin/env bash
# THE HALF `open` AND `archive` SHARE, ONCE THE PULL REQUEST IS RESOLVED.
#
# The two jobs find the pull request differently -- `open` from the
# workflow_run's own head sha, `archive` from a merged-pull-request search on
# the pushed commit -- but from a resolved number on, the work is identical:
# refuse anything that is not a same-repo `change/*` branch (a merge queue can
# land a fork's pull request too), read which issue the body names, and carry
# the grant. Duplicating that here once, called from both jobs, rather than
# twice in the workflow YAML, is what keeps a fix to either half from landing
# in only one of them.
#
# TWO SHAPES OF ISSUE REFERENCE, AND WHICH STATION READS WHICH:
#   Refs #<n>    every pull request of a change but its last: the issue
#                follows the change through review and archiving
#   Closes #<n>  the ARCHIVE pull request (and a plain-lane fix that closes
#                an ordinary issue) -- pr-closes-guard.py enforces both
# `fix` carries onto EITHER, so the archive pull request is driven to
# mergeable by the same loop as the first one. `archive` carries only from a
# `Refs` pull request; a merged `Closes` pull request is the line's END, and
# the issue's station is set to done.
#
# STATE, NOT A GRANT, AT THE MOMENTS THIS PROGRAM KNOWS THEM. `open` runs
# only when `ci` COMPLETED WITH SUCCESS on the pull request -- and `ci` waits
# for the review and reads its threads live -- so a `fix` carry here means the
# head is mergeable: the pull request's loop label says so and the issue's
# station says `merge`. A person merges; nothing here does.
set -euo pipefail

[ $# -eq 2 ] || { echo "usage: carry-from-pr.sh <pr-number> <fix|archive>" >&2; exit 64; }
pr="$1"
station="$2"
case "$station" in
  fix|archive) ;;
  *) echo "usage: carry-from-pr.sh <pr-number> <fix|archive> -- got '$station'" >&2; exit 64 ;;
esac
state() { python3 .github/scripts/conveyor-state.py --repo "$GITHUB_REPOSITORY" "$@" || true; }

# isCrossRepository IS THE REAL CHECK -- review-dispatch.yml's own `who` step
# uses the same field for the same reason. A same-OWNER, different-REPO pull
# request (someone's own fork under this account, say) would still match a
# same-owner string comparison; isCrossRepository is what GitHub itself
# computes from the actual head repository, not an approximation of it.
read -r cross head_ref < <(gh pr view "$pr" --repo "$GITHUB_REPOSITORY" \
  --json isCrossRepository,headRefName --jq '"\(.isCrossRepository) \(.headRefName)"')
case "$cross/$head_ref" in
  false/change/*) ;;
  *)
    echo "pull request #$pr is not a same-repo change/* branch; nothing to carry"
    exit 0
    ;;
esac

body=$(gh pr view "$pr" --repo "$GITHUB_REPOSITORY" --json body --jq '.body')
refs=$(printf '%s' "$body" | grep -oE 'Refs #[0-9]+' | head -1 | grep -oE '[0-9]+' || true)
closes=$(printf '%s' "$body" | grep -oE 'Closes #[0-9]+' | head -1 | grep -oE '[0-9]+' || true)

if [ "$station" = "archive" ]; then
  if [ -n "$closes" ] && [ -z "$refs" ]; then
    # THE LINE'S END: the archive pull request (or a plain-lane fix closing an
    # ordinary issue) merged. GitHub closes the issue on the keyword; the
    # station label is the last thing to say.
    echo "pull request #$pr closes #$closes; the line ended at this merge, nothing to carry"
    state --target "$closes" --station done
    exit 0
  fi
  if [ -z "$refs" ]; then
    echo "pull request #$pr's body carries no 'Refs #<n>'; nothing to carry"
    exit 0
  fi
  out=$(python3 .github/scripts/carry-grant.py --repo "$GITHUB_REPOSITORY" \
    --issue "$refs" --station archive | tee /dev/stderr)
  case "$out" in
    *"is on the plain lane"*)
      # THE PLAIN LANE ENDS AT THE MERGE, and `Refs` is what its pull request
      # said. Nothing is archived, and the station says so.
      state --target "$refs" --station done ;;
  esac
  exit 0
fi

issue="${refs:-$closes}"
if [ -z "$issue" ]; then
  echo "pull request #$pr's body names no issue ('Refs #<n>' or 'Closes #<n>'); nothing to carry"
  exit 0
fi
python3 .github/scripts/carry-grant.py --repo "$GITHUB_REPOSITORY" \
  --issue "$issue" --station fix --pr "$pr"
# ci SUCCEEDED on this head (the caller's precondition), so the pull request
# is mergeable and the issue is at the merge station -- whether or not a grant
# was carried, since state grants nothing.
state --target "$pr" --loop mergeable
state --target "$issue" --station merge
