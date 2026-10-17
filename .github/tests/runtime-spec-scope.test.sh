#!/usr/bin/env bash
# `.claude/rules/runtime-spec.md`'s `paths:` frontmatter, checked against
# the real tracked tree (its own scope is a fact about the REAL
# repository, so this one -- unlike struct-table.test.py -- reads the real
# rule file and runs `git ls-files` for real, never a fixture). The cases
# are Python (unittest, standard library only); this wrapper is what
# run.sh discovers. See runtime-spec-scope.test.py for what each case holds.
cd "$(dirname "$0")" || exit 1
out=$(python3 runtime-spec-scope.test.py -v 2>&1); rc=$?
if [ "$rc" -ne 0 ]; then printf '%s\n' "$out"; else printf '%s\n' "$out" | tail -5; fi
exit "$rc"
