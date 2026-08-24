#!/usr/bin/env bash
# THE TRACKING ISSUE FOR AN OPENSPEC CHANGE — opened, advanced, closed.
#
# ONE ISSUE PER CHANGE, AND IT IS A POINTER. The change directory under
# openspec/changes/<name>/ is the source of truth. This issue links it, names
# the phase, and restates NOTHING — a copied proposal is a second thing to keep
# true, and nothing ever tells a reader which of the two they are reading.
#
# THE BINDING IS A REF. The issue NUMBER is written to <change>/.github-issue
# and nothing else about the issue is stored. A number stays correct when the
# title, labels or body change; anything the issue SAYS can rot with no
# mechanism to notice. The sidecar travels into openspec/changes/archive/ with
# the change, which is what keeps an archived change traceable.
#
# PROMOTION IN PLACE. `open --promote <n>` adopts an issue somebody else filed
# rather than opening a project-authored one beside it. The reporter is waiting
# in that thread; closing it discards their words and every reply attached.
#
# Used by /opsx:propose, /opsx:apply, /opsx:archive and the migration script, so
# the four cannot drift into four spellings of the same thing.
set -euo pipefail

PREFIX="opsx"
usage() {
  cat >&2 <<EOF
usage:
  opsx-issue.sh open   <change> [--promote <issue>] [--dry-run]
  opsx-issue.sh phase  <change> <proposed|applying|review|archived> [--note <text>]
  opsx-issue.sh close  <change>
  opsx-issue.sh number <change>
EOF
  exit 64
}

repo_root() { git rev-parse --show-toplevel; }
change_dir() { echo "$(repo_root)/openspec/changes/$1"; }
sidecar()    { echo "$(change_dir "$1")/.github-issue"; }

number() {
  local f; f=$(sidecar "$1")
  [ -r "$f" ] || return 1
  tr -dc '0-9' < "$f"
}

# The one line the issue is allowed to say about the change: its proposal's
# title, which the proposal already had to write.
headline() {
  local p; p="$(change_dir "$1")/proposal.md"
  [ -r "$p" ] && sed -n 's/^# *//p' "$p" | head -1 || true
}

body() {
  local change="$1" head; head=$(headline "$change")
  cat <<EOF
${head:-$change}

| | |
|---|---|
| **Change** | \`openspec/changes/$change/\` |
| **Branch** | \`change/$change\` |
| **Phase** | see the \`$PREFIX:\` label |

The change directory is the source of truth — the why, the design, the specs and
the task list all live there, and this issue deliberately restates none of it.

\`\`\`sh
openspec show $change
\`\`\`
EOF
}

cmd_open() {
  local change="$1"; shift
  local promote="" dry=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --promote) promote="$2"; shift 2 ;;
      --dry-run) dry=1; shift ;;
      *) usage ;;
    esac
  done

  [ -d "$(change_dir "$change")" ] || { echo "no such change: $change" >&2; exit 1; }

  # Idempotent by the sidecar: re-running must never open a second issue.
  if n=$(number "$change" 2>/dev/null) && [ -n "$n" ]; then
    echo "$n"; return 0
  fi

  if [ -n "$dry" ]; then
    echo "would open an issue for '$change'${promote:+ by promoting #$promote}" >&2
    echo "0"; return 0
  fi

  local n
  if [ -n "$promote" ]; then
    # PROMOTED IN PLACE: title, body and every comment are left exactly as the
    # reporter wrote them. Only a pointer comment and the label are added.
    n="$promote"
    gh issue comment "$n" --body "$(body "$change")" >/dev/null
  else
    n=$(gh issue create \
          --title "$change: $(headline "$change")" \
          --body "$(body "$change")" \
          --label "$PREFIX:proposed" \
        | sed 's|.*/||')
  fi

  gh issue edit "$n" --add-label "$PREFIX:proposed" >/dev/null
  printf '%s\n' "$n" > "$(sidecar "$change")"
  echo "$n"
}

cmd_phase() {
  local change="$1" to="$2"; shift 2
  local note=""
  [ "${1:-}" = "--note" ] && { note="$2"; shift 2; }
  case "$to" in proposed|applying|review|archived) ;; *) usage ;; esac

  local n; n=$(number "$change") || { echo "no tracking issue for '$change'" >&2; exit 1; }

  local args=(--add-label "$PREFIX:$to")
  for other in proposed applying review archived; do
    [ "$other" = "$to" ] || args+=(--remove-label "$PREFIX:$other")
  done
  gh issue edit "$n" "${args[@]}" >/dev/null

  # ONE comment per transition, not per task. A stream of progress comments
  # makes the issue longer than the artifact it points at.
  gh issue comment "$n" --body "${note:-Phase: **$to**.}" >/dev/null
  echo "$n"
}

cmd_close() {
  local change="$1"
  local n; n=$(number "$change") || { echo "no tracking issue for '$change'" >&2; exit 1; }
  gh issue close "$n" >/dev/null
  echo "$n"
}

[ $# -ge 2 ] || usage
action="$1"; shift
case "$action" in
  open)   cmd_open   "$@" ;;
  phase)  cmd_phase  "$@" ;;
  close)  cmd_close  "$@" ;;
  number) number "$1" ;;
  *)      usage ;;
esac
