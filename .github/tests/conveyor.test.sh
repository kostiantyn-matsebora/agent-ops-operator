#!/usr/bin/env bash
# The conveyor's state machine, over every combination of its facts. The cases
# are Python (unittest, standard library only); this wrapper is what run.sh
# discovers. See conveyor.test.py for what each block holds.
cd "$(dirname "$0")" || exit 1
out=$(python3 conveyor.test.py 2>&1); rc=$?
if [ "$rc" -ne 0 ]; then printf '%s\n' "$out"; else printf '%s\n' "$out" | tail -3; fi
exit "$rc"
