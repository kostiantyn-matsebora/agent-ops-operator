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
# when `ci` COMPLETED on the pull request -- with SUCCESS or FAILURE, since
# #221 made a red ci start a carried round too -- and hands the conclusion in
# as CI_CONCLUSION. Only a SUCCESS is the moment to ask whether the head is
# mergeable: ci green AND no review-authored thread open
# (`review-not-clean.py`, read live, since a check cannot carry that question
# -- see its docstring). Then the pull request's loop label says `mergeable`
# and the issue's station `merge`. Measured on #220: the first version read
# every `open` run as green and marked a red head mergeable. A person merges;
# nothing here does.
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
# ONLY A GREEN ci IS A MERGEABLE HEAD. With no review thread open the pull
# request is mergeable and the issue is at the merge station -- whether or not
# a grant was carried, since state grants nothing. A red ci, or an open
# thread, leaves the state to the loop's own ending (`stalled`) and to the
# merge box.
case "${CI_CONCLUSION:-}" in
  success)
    if python3 .github/scripts/review-not-clean.py --repo "$GITHUB_REPOSITORY" --pr "$pr"; then
      state --target "$pr" --loop mergeable
      state --target "$issue" --station merge
    else
      echo "pull request #$pr has a review thread open; not mergeable yet, state unchanged"
    fi ;;
  *)
    echo "ci concluded '${CI_CONCLUSION:-unknown}' on #$pr; not mergeable, state unchanged" ;;
esac
