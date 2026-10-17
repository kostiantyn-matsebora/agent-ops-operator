#!/usr/bin/env bash
# The `struct-table` marker in docs-generate.py: a rendered table, a drift
# failure, and a loud failure on a struct the parser cannot handle. The
# cases are Python (unittest, standard library only, loading the real
# docs-generate.py module by path); this wrapper is what run.sh discovers.
# See struct-table.test.py for what each case holds.
cd "$(dirname "$0")" || exit 1
out=$(python3 struct-table.test.py -v 2>&1); rc=$?
if [ "$rc" -ne 0 ]; then printf '%s\n' "$out"; else printf '%s\n' "$out" | tail -5; fi
exit "$rc"
