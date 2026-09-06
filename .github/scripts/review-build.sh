#!/usr/bin/env bash
# Build one review group with the SAME recipe CI uses for it, before any
# reader starts. A component that does not compile is not worth a model's
# turn: every finding against it would be a finding against code the author
# is about to rewrite. On failure this writes the group's READING itself —
# `unbuilt` with the tail of the build's own output — so the group is
# reported as a fact about the pull request, never silently skipped.
#
# NO CREDENTIAL. This runs the pull request's own code (its package.json
# scripts, its go generate) under `contents: read` alone; the model's token
# is never in this step's environment.
#
#   review-build.sh <group> --out <reading.json> [--paths p1,p2,...] [--root <dir>]
#
# Prints the recipe it chose (or "no build") and always exits 0 — a failed
# build is a fact this script records, not an error in this script. The
# caller reads `built=true|false` from $GITHUB_OUTPUT when set.
set -uo pipefail

group=""
out=""
paths=""
root="$(cd "$(dirname "$0")/../.." && pwd)"
while [ $# -gt 0 ]; do
  case "$1" in
    --out) out="$2"; shift 2 ;;
    --paths) paths="$2"; shift 2 ;;
    --root) root="$2"; shift 2 ;;
    -*) echo "review-build.sh: unknown flag: $1" >&2; exit 2 ;;
    *) group="$1"; shift ;;
  esac
done
[ -n "$group" ] || { echo "usage: review-build.sh <group> --out <reading.json> [--paths p1,p2,...]" >&2; exit 2; }

log=$(mktemp)
trap 'rm -f "$log"' EXIT
run() { echo "+ $*" >>"$log"; "$@" >>"$log" 2>&1; }

recipe="no build"
recipe_ran=true
ok=true
case "$group" in
  chart)
    recipe="helm lint chart && helm template chart"
    run helm lint "$root/chart" && run helm template "$root/chart" >/dev/null || ok=false
    ;;
  runtimes/claude|runtimes/copilot)
    recipe="node --test"
    ( cd "$root/$group" && run node --test ) || ok=false
    ;;
  platform/console)
    # ITS GO BUILD EMBEDS `ui/dist`, so the UI is built first — the same
    # order CI's `modules` job depends on for this one component.
    recipe="npm ci && npm run build (ui/), then go build + go vet"
    ( cd "$root/$group/ui" && run npm ci && run npm run build ) &&
      ( cd "$root/$group" && run go build ./... && run go vet ./... ) || ok=false
    ;;
  *)
    if [ -f "$root/$group/go.mod" ]; then
      recipe="go build ./... && go vet ./..."
      ( cd "$root/$group" && run go build ./... && run go vet ./... ) || ok=false
    else
      recipe_ran=false
    fi
    ;;
esac

if [ "$recipe_ran" = false ]; then
  echo "$group: no build"
  [ -n "${GITHUB_OUTPUT:-}" ] && echo "built=true" >> "$GITHUB_OUTPUT"
  exit 0
fi

if [ "$ok" = true ]; then
  echo "$group: built ($recipe)"
  [ -n "${GITHUB_OUTPUT:-}" ] && echo "built=true" >> "$GITHUB_OUTPUT"
  exit 0
fi

tail_text=$(tail -n 40 "$log")
{
  echo "$group: build failed ($recipe)"
  echo "$tail_text"
} >&2
[ -n "${GITHUB_OUTPUT:-}" ] && echo "built=false" >> "$GITHUB_OUTPUT"
if [ -n "$out" ]; then
  IFS=',' read -r -a patharr <<< "$paths"
  tailfile=$(mktemp)
  printf '%s' "$tail_text" > "$tailfile"
  python3 - "$group" "$out" "$tailfile" "${patharr[@]}" <<'PY'
import json, sys
group, out, tailfile = sys.argv[1], sys.argv[2], sys.argv[3]
unread = [p for p in sys.argv[4:] if p]
tail = open(tailfile).read()
json.dump({"component": group, "unbuilt": tail, "findings": [], "changedNames": [],
           "files": [], "threads": [], "unread": unread}, open(out, "w"), indent=1)
print()
PY
  rm -f "$tailfile"
fi
exit 0
