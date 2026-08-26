#!/usr/bin/env bash
# REFUSE TO PUSH A CHART RELEASE TAG THE DOCUMENTATION DOES NOT PRINT.
#
# `require-docs-task.sh` guards a CHANGE: it refuses `openspec archive` while
# the change's documentation task is open. A RELEASE is a different event — a
# `chart-v<semver>` tag, pushed outside any change — and nothing guarded it.
# Chart 13.1.0 shipped on 2026-08-26 while `docs/installation.md` still told
# adopters to install 13.0.1, and the person running the release had to be told
# three times before anyone looked.
#
# This hook fires on `git push` of a `chart-v<semver>` tag and refuses it unless
# THREE things agree on the number: `chart/Chart.yaml`, a `## [<semver>]` entry
# in `docs/CHANGELOG.md`, and every `--version` / first-party image tag a page
# prints — the last through `docs-generate.py --check`, the same check CI runs,
# so the local gate and CI cannot give two answers to one question.
#
# FAILS OPEN on anything it cannot read: no jq, no python, no chart in the tree
# the command runs in. A hook that blocks work it does not understand gets
# disabled, and then it enforces nothing at all.
set -u

command -v jq >/dev/null 2>&1 || exit 0
payload=$(cat) || exit 0
cmd=$(printf '%s' "$payload" | jq -r '.tool_input.command // empty' 2>/dev/null) || exit 0
[ -n "$cmd" ] || exit 0

case "$cmd" in *git*push*chart-v*) ;; *) exit 0 ;; esac
version=$(printf '%s' "$cmd" | grep -o 'chart-v[0-9]\+\.[0-9]\+\.[0-9]\+' | head -1 | sed 's/^chart-v//')
[ -n "$version" ] || exit 0

cd_prefix=$(printf '%s' "$cmd" |
  sed -n 's/^[[:space:]]*cd[[:space:]]\{1,\}\([^&;|]*\).*/\1/p' |
  sed 's/[[:space:]]*$//' | tr -d "\"'")
payload_cwd=$(printf '%s' "$payload" | jq -r '.cwd // empty' 2>/dev/null)
root=""
for candidate in "$cd_prefix" "$payload_cwd" "${CLAUDE_PROJECT_DIR:-}"; do
  [ -n "$candidate" ] || continue
  top=$(git -C "$candidate" rev-parse --show-toplevel 2>/dev/null) || continue
  root=$top; break
done
[ -n "$root" ] && [ -r "$root/chart/Chart.yaml" ] || exit 0

problems=""
chart_version=$(sed -n 's/^version:[[:space:]]*//p' "$root/chart/Chart.yaml" | head -1)
[ "$chart_version" = "$version" ] || problems="$problems
- chart/Chart.yaml says version $chart_version, the tag says $version"
grep -q "^## \[$version\]" "$root/docs/CHANGELOG.md" 2>/dev/null || problems="$problems
- docs/CHANGELOG.md has no '## [$version]' entry"
if command -v python3 >/dev/null 2>&1 && [ -r "$root/.github/scripts/docs-generate.py" ]; then
  if ! out=$(cd "$root" && python3 .github/scripts/docs-generate.py --check 2>&1); then
    problems="$problems
- docs-generate.py --check fails:
$(printf '%s' "$out" | sed 's/^/    /')"
  fi
fi
[ -n "$problems" ] || exit 0

cat >&2 <<EOM
BLOCKED: chart-v$version cannot be pushed yet — the documentation does not print this release.
$problems

A release is done when the install command, the worked examples and the
changelog all say its number. Fix these on a branch, merge, then tag.
See .claude/rules/documentation.md.
EOM
exit 2
