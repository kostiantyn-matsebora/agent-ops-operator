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
#   images  — every Dockerfile. The component is derived from its PATH:
#             <group>/<leaf>, where a PLURAL group names a kind of component and
#             contributes its singular as a prefix, and a singular group is a
#             namespace and contributes nothing.
#
#               signals/cron       -> signal-cron        -> agentops-signal-cron
#               channels/telegram  -> channel-telegram   -> agentops-channel-telegram
#               runtimes/claude    -> runtime-claude     -> agentops-runtime-claude
#               gateways/telegram  -> gateway-telegram   -> agentops-gateway-telegram
#               platform/console   -> console            -> agentops-console
#
#             The name is therefore never repeated: the directory says the kind
#             once, and the image name says it once, and neither is written
#             down twice. Renaming a directory renames a published image, so
#             the derivation is asserted unique below.
#   modules — every go.mod. The envtest suite belongs to platform/manager, and
#             the `operator` job in ci.yml names that module so it is tested
#             once, with assets.
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

# component_for derives a component name from a directory PATH.
#
# A group whose name is PLURAL names a kind of component and lends its singular
# to every member; a group whose name is singular is a namespace and lends
# nothing. That is the whole rule, and it reproduces every name this repository
# has ever published except the one this change renames on purpose.
component_for() {
  local dir="${1#./}"
  local group="${dir%%/*}" leaf="${dir#*/}"
  case "$group" in
    *s) echo "${group%s}-${leaf}" ;;
    *)  echo "$leaf" ;;
  esac
}

images() {
  local out=()
  while IFS= read -r dockerfile; do
    local dir component context
    dir="$(dirname "$dockerfile")"
    component="$(component_for "$dir")"
    context="$dir"
    out+=("$(jq -nc --arg c "$component" --arg ctx "$context" --arg df "$dockerfile" \
      --arg p "$(platforms_for "$component")" \
      '{component:$c, context:$ctx, dockerfile:$df, platforms:$p}')")
  done < <(find . -name Dockerfile -not -path '*/node_modules/*' | sed 's|^\./|./|' | sort)
  # UNIQUE, asserted rather than assumed. A flat directory name was unique by
  # construction; a derived one is not — two groups could produce one component
  # name, and the release workflow matches a tag against exactly this list.
  local names dupes
  names="$(printf '%s\n' "${out[@]}" | jq -r .component | sort)"
  dupes="$(printf '%s\n' "$names" | uniq -d)"
  if [ -n "$dupes" ]; then
    echo "components.sh: two components derive one name: $dupes" >&2
    exit 1
  fi
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
