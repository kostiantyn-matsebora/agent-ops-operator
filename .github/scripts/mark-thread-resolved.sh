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
  # GraphQL node ids only, matched WHOLE. Anything else is a mistake worth
  # failing on rather than appending, because a malformed line reads as a
  # refusal later and looks like the guard working when it was the caller that
  # was wrong.
  #
  # A `case` GLOB WAS WRONG HERE and shipped that way: `[A-Za-z0-9_=-]*`
  # matches a first character followed by ANYTHING, so it accepted
  # `PRRT_x; rm -rf /` unchanged. Anchoring is the whole of the check, and a
  # pattern that only looks anchored is worse than no check — it reads as
  # validation in review.
  if [[ ! "$id" =~ ^[A-Za-z0-9_=-]+$ ]]; then
    echo "not a thread id: $id" >&2
    exit 65
  fi
  printf '%s\n' "$id" >> "$out"
done

echo "recorded $# thread(s) for resolution in $out"
