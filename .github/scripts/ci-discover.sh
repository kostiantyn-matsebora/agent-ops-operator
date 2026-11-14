#!/usr/bin/env bash
#
# WHAT A CHANGE TOUCHES, turned into what ci.yml must run.
#
# Reads the changed paths, one per line, from the file named by $1 and writes
# `key=value` lines (the shape GITHUB_OUTPUT takes) to stdout. With no file, or
# an empty one, the changed set could not be established and EVERYTHING runs: a
# filter that silently matches nothing is a CI that silently tests nothing.
#
# A change that touches only openspec/, docs-free metadata or .claude/ matches
# no component directory and no job flag, so no component is built or tested.
# The cheap gates that read the diff themselves (`openspec`, `docs-task`,
# `publication`...) still run; they are not components.
#
# Needs ENVTEST_MODULE (the module the plain matrix leaves out) and jq.
set -euo pipefail
cd "$(dirname "$0")/../.."

changed_file="${1:-}"
ALL_MODULES=$(./.github/components.sh modules \
  | jq -c --arg m "${ENVTEST_MODULE:?}" 'map(select(. != $m))')
ALL_IMAGES=$(./.github/components.sh images)

everything() {
  echo "modules=$ALL_MODULES"
  echo "images=$ALL_IMAGES"
  for j in operator chart docs site system scripts; do echo "$j=true"; done
}

# THE PATHS THAT REBUILD EVERYTHING. The shared Dockerfile is nine components'
# recipe, `components.sh` decides what exists at all, this workflow decides how
# any of it is tested, this script decides what runs, and the scan action runs
# against every image. None sits in a component directory.
if [ -z "$changed_file" ] || ! grep -q . "$changed_file" 2>/dev/null \
   || grep -qE '^\.github/(docker/|components\.sh|workflows/ci\.yml|scripts/ci-discover\.sh|actions/)' "$changed_file"; then
  everything
  exit 0
fi

# A component is built when a file inside its own directory moved.
echo "modules=$(echo "$ALL_MODULES" | jq -c --rawfile c "$changed_file" '
  ($c | split("\n") | map(select(length > 0))) as $files
  | map(select(. as $m | $files | any(startswith($m + "/"))))')"

# Each flag is `true` or empty; a job reads its own and nothing else. `operator`
# provisions envtest and its tests render the CHART, so it watches both trees.
m() { grep -qE "$1" "$changed_file" && echo true || true; }
echo "operator=$(m '^(platform/manager/|chart/)')"
echo "chart=$(m '^(chart/|platform/manager/api/)')"
echo "docs=$(m '^(docs/|chart/|platform/manager/api/|\.github/scripts/docs-generate)')"
echo "site=$(m '^docs/')"
# Anything a running install is built from, and the pack itself.
echo "system=$(m '^(platform/|runtimes/|signals/|channels/|gateways/|chart/|test/|\.github/workflows/system\.yml)')"
# The workflow's own scripts, their suite, and the hooks the suite exercises.
echo "scripts=$(m '^(\.github/scripts/|\.github/tests/|\.github/\.coveragerc|\.claude/hooks/)')"

echo "images=$(echo "$ALL_IMAGES" | jq -c --rawfile c "$changed_file" '
  ($c | split("\n") | map(select(length > 0))) as $files
  | map(select((.context | sub("^\\./"; "")) as $d | $files | any(startswith($d + "/"))))')"
