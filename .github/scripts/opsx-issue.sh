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

# A CHANGE IS LIVE OR ARCHIVED, AND `close` RUNS AFTER IT IS ARCHIVED.
#
# `openspec archive` moves the directory to
# openspec/changes/archive/<YYYY-MM-DD>-<name>/ and the sidecar travels with it —
# deliberately, since a reference is what keeps an archived change traceable. But
# resolving only the live path meant `phase ... archived` and `close` failed at
# EXACTLY the point /opsx:archive tells you to run them, reporting "no tracking
# issue" for a change whose issue was sitting right there. The issue then stays
# open on work that is finished, or somebody closes it by hand and the label is
# never advanced.
#
# The date prefix is matched rather than a bare suffix: `*-$1` would let a change
# named `auth` collect the sidecar of `2026-01-01-oauth`.
change_dir() {
  local root live hit
  root=$(repo_root)
  live="$root/openspec/changes/$1"
  [ -d "$live" ] && { echo "$live"; return 0; }
  # `|| true` BECAUSE THERE MAY BE NO ARCHIVE DIRECTORY AT ALL. `find` exits 1
  # on a missing path, `pipefail` makes that the assignment's status, and
  # `errexit` then aborts this function BEFORE the fallback below — the same
  # shape that made `ci.yml`'s openspec gate die on a pull request touching no
  # change. Every caller today embeds the substitution in a larger word, so the
  # abort merely yields an empty string that still reads as "not found"; a
  # future `x=$(change_dir "$name")` would get that empty string rather than the
  # live path this function promises.
  hit=$(find "$root/openspec/changes/archive" -maxdepth 1 -type d \
          -name '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]-'"$1" 2>/dev/null |
        sort | tail -1) || true
  # The live path is still the answer when there is no archive either: the
  # caller's own "no tracking issue" message is the right report for a change
  # that does not exist.
  echo "${hit:-$live}"
}
sidecar()    { echo "$(change_dir "$1")/.github-issue"; }

number() {
  local f; f=$(sidecar "$1")
  [ -r "$f" ] || return 1
  tr -dc '0-9' < "$f"
}

# The one line the issue is allowed to say about the change, and it is TAKEN
# rather than written: the first sentence of the proposal's own Why.
#
# IT USED TO READ THE FIRST `# ` HEADING, AND THERE ISN'T ONE. An openspec
# proposal opens at `## Why` — the change's name is the directory, so the file
# never repeats it — and `s/^# *//` on `## Why` strips one hash and yields
# `# Why`. Every issue this script has ever opened is titled
# `<change>: # Why`, which says nothing and looks like a bug in the tool that
# reads it. #38 is the surviving example.
#
# The first sentence is the one a proposal spends the most care on, and taking
# it is not a copy in the sense the pointer rule forbids: a title identifies,
# and an issue with no title cannot be scanned at all.
headline() {
  local p; p="$(change_dir "$1")/proposal.md"
  [ -r "$p" ] || { echo "$1"; return 0; }
  # From under `## Why`, the first non-empty prose line, to its first full stop.
  # Bold markers go: `**The review posts findings**` is emphasis for a reader
  # of the file, and noise in a list of issue titles.
  # THE PARAGRAPH IS JOINED BEFORE THE SENTENCE IS CUT, because the prose here
  # is hard-wrapped: `head -1` takes a LINE, and a first sentence routinely runs
  # onto the second one. Cutting by line produced `...serviceaccounts that
  # nothing`, which reads as a truncation bug rather than a title.
  local line
  line=$(sed -n '/^## Why/,/^## /p' "$p" |
         sed '1d;/^#/d' |
         awk 'NF{buf=buf" "$0;next} buf{print buf;exit} END{if(buf)print buf}' |
         sed 's/^ *//; s/\*\*//g; s/`//g')
  line=${line%%. *}
  line=${line%.}
  # A title is scanned in a list, so it is bounded. 72 leaves room for the
  # change name in front of it.
  [ ${#line} -gt 72 ] && line="${line:0:69}..."
  echo "${line:-$1}"
}

# WHERE THE CHANGE CAN BE READ, AS A LINK — a path is not a pointer to anybody
# who has not cloned this repository, and the issue's whole job is pointing.
# The ref follows the change: its own branch while it is in flight, the default
# branch once it is archived and the branch is gone.
slug() { gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null || true; }

read_links() {
  local change="$1" repo ref path
  repo=$(slug); [ -n "$repo" ] || return 0
  if [ -d "$(repo_root)/openspec/changes/$change" ]; then
    ref="change/$change"
    path="openspec/changes/$change"
  else
    # Archived: the directory moved and the branch is merged, so the default
    # branch is the only ref that still resolves.
    ref=$(git -C "$(repo_root)" symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null)
    ref=${ref#origin/}; ref=${ref:-master}
    path="openspec/changes/archive/$(basename "$(change_dir "$change")")"
  fi
  local base="https://github.com/$repo/blob/$ref/$path"
  echo "[proposal]($base/proposal.md) · [design]($base/design.md) · [tasks]($base/tasks.md)"
}

body() {
  local change="$1" head links; head=$(headline "$change"); links=$(read_links "$change")
  cat <<EOF
${head:-$change}

| | |
|---|---|
| **Change** | \`openspec/changes/$change/\` |
| **Read it** | ${links:-not published yet} |
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

  # THE BODY IS REGENERATED AT EVERY TRANSITION, and that is what makes the
  # links honest. At `propose` the change exists only in a working copy, so its
  # branch link resolves to nothing; by `applying` the branch is pushed, and at
  # `archived` the directory has moved and only the default branch still has it.
  # A body written once would be a 404 for the whole life of the issue.
  gh issue edit "$n" --body "$(body "$change")" >/dev/null

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
