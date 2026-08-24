#!/usr/bin/env bash
# THE ONLY THING THAT CROSSES THE PRIVILEGE BOUNDARY: a list of thread ids.
#
# The review job cannot resolve a thread -- that needs `contents: write`, which
# it deliberately does not have. It records which of its own threads it believes
# are addressed, and a second, model-free job acts on the list.
#
# A LIST OF IDS IS THE WHOLE INTERFACE, on purpose. Nothing the review writes
# here is executed, interpolated or trusted: the reconcile step re-reads every
# thread from GitHub, refuses any it did not author, and treats this file as a
# request rather than an instruction.
set -euo pipefail

out="${RESOLVE_LIST:-.resolve-threads}"

[ $# -ge 1 ] || { echo "usage: mark-thread-resolved.sh <threadId> [...]" >&2; exit 64; }

for id in "$@"; do
  # GraphQL node ids only. Anything else is a mistake worth failing on rather
  # than appending, because a malformed line reads as a refusal later and looks
  # like the guard working when it is the caller that was wrong.
  case "$id" in
    PRRT_*|MDIzOlB1*|[A-Za-z0-9_=-]*) printf '%s\n' "$id" >> "$out" ;;
    *) echo "not a thread id: $id" >&2; exit 65 ;;
  esac
done

echo "recorded $# thread(s) for resolution in $out"
