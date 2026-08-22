#!/usr/bin/env bash
#
# WHAT THIS REPOSITORY SHIPS, derived from the repository.
#
# Both workflows read their matrices from here, and neither carries a list of
# modules or images. That is the point: this inventory went from five images to
# thirteen while the release procedure stood still, and the one document that
# listed them was already missing one (egress-proxy). A matrix you have to
# remember to edit is a matrix that silently stops covering things.
#
# Discovery rules:
#   images  — every Dockerfile. Root is `manager`; anything else is its directory.
#   modules — every go.mod EXCEPT the root's, which the `operator` job owns
#             because only it needs envtest.
#
# The one thing that cannot be derived is PLATFORMS, so it is declared, once,
# below — and the release asserts what it actually pushed against that
# declaration, as EQUALITY. Today every component builds both architectures and
# the exception map is empty.
set -euo pipefail

cd "$(dirname "$0")/.."

# Components that genuinely cannot be built for every architecture. EMPTY, and
# earning it: `runtime-claude` sat here on the strength of a note in CLAUDE.md
# saying its upstream was amd64-only. Building it settled that —
#
#   docker buildx build --platform linux/arm64 ./runtime-claude/   # succeeds
#   docker run --platform linux/arm64 … claude --version
#   arch: aarch64 / v22.23.2 / 2.1.239 (Claude Code)
#
# — so the constraint was the hand-run `--platform linux/amd64` in the build
# command, not the vendor. Nothing goes in this map on the strength of how an
# image has been built before: the declaration says what the image CAN do, and
# that is established by building it.
declare -A SINGLE_ARCH=()

DEFAULT_PLATFORMS="linux/amd64,linux/arm64"

platforms_for() {
  local component="$1"
  echo "${SINGLE_ARCH[$component]:-$DEFAULT_PLATFORMS}"
}

images() {
  local out=()
  while IFS= read -r dockerfile; do
    local dir component context
    dir="$(dirname "$dockerfile")"
    if [ "$dir" = "." ]; then
      component="manager"
      context="."
    else
      component="$(basename "$dir")"
      context="$dir"
    fi
    out+=("$(jq -nc --arg c "$component" --arg ctx "$context" --arg df "$dockerfile" \
      --arg p "$(platforms_for "$component")" \
      '{component:$c, context:$ctx, dockerfile:$df, platforms:$p}')")
  done < <(find . -name Dockerfile -not -path '*/node_modules/*' | sed 's|^\./|./|' | sort)
  printf '%s\n' "${out[@]}" | jq -sc .
}

modules() {
  find . -name go.mod -not -path '*/node_modules/*' -mindepth 2 \
    | sed 's|/go.mod$||; s|^\./||' | sort | jq -Rsc 'split("\n") | map(select(length > 0))'
}

case "${1:-}" in
  images)    images ;;
  modules)   modules ;;
  platforms) platforms_for "${2:?usage: components.sh platforms <component>}" ;;
  *) echo "usage: $0 {images|modules|platforms <component>}" >&2; exit 2 ;;
esac
