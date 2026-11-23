#!/usr/bin/env bash
# The testing station's programs: the diff coverage reader, the verdict and report, and the
# branch sync. The cases are Python (unittest, standard library only), as conveyor.test.sh's are.
# NO NETWORK: `gh` is replaced in-process, and the sync runs in throwaway git repositories.
cd "$(dirname "$0")" || exit 1
out=$(python3 testing-station.test.py 2>&1); rc=$?
if [ "$rc" -ne 0 ]; then printf '%s\n' "$out"; else printf '%s\n' "$out" | tail -3; fi
exit "$rc"
