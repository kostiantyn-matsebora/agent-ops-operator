#!/usr/bin/env bash
# THE APT LAYER IS REBUILT ONCE A DAY, OR `apt-get upgrade` PATCHES NOTHING.
#
# The build cache keys a RUN on its text and the build arguments it reads. A
# layer cached before a Debian security fix existed keeps the vulnerable
# package until the instruction changes — libssh2 (CVE-2026-7598) failed the
# scan on a layer that already carried the upgrade line, because a cached
# layer runs no command. So every Dockerfile that runs apt reads APT_REFRESH,
# and both image-building workflows pass the date. Either half missing
# silently reopens the trap, and the release path shares the cache scope.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)

apt_dockerfiles=$(cd "$ROOT" && git ls-files | grep -i 'dockerfile$' | xargs grep -l 'apt-get' | sort)

it "at least the four known apt Dockerfiles are under test"
assert_contains "$apt_dockerfiles" "runtimes/claude/Dockerfile"
assert_contains "$apt_dockerfiles" "runtimes/copilot/Dockerfile"
assert_contains "$apt_dockerfiles" "runtimes/ollama/Dockerfile"
assert_contains "$apt_dockerfiles" "platform/egress-proxy/Dockerfile"

for f in $apt_dockerfiles; do
  content=$(cat "$ROOT/$f")
  it "$f declares APT_REFRESH before its apt layer"
  assert_contains "$content" 'ARG APT_REFRESH='
  it "$f reads APT_REFRESH in the same RUN as apt-get, so the key changes with it"
  # The RUN that calls apt-get, joined across its continuations.
  run=$(awk '/^RUN /{r=$0; next} r && /\\$/{r=r $0; next} r{r=r $0; print r; r=""}' "$ROOT/$f" | grep 'apt-get' | head -1)
  assert_contains "$run" '${APT_REFRESH}'
  it "$f upgrades the base's packages in that layer, which is what the rebuild buys"
  assert_contains "$run" 'apt-get upgrade -y'
done

for wf in ci.yml build-image.yml; do
  content=$(cat "$ROOT/.github/workflows/$wf")
  it "$wf passes the date as APT_REFRESH to the image build"
  assert_contains "$content" 'build-args: APT_REFRESH=${{ steps.date.outputs.date }}'
  assert_contains "$content" 'echo "date=$(date -u +%F)" >> "$GITHUB_OUTPUT"'
done

summary
