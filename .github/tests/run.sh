#!/usr/bin/env bash
# Every script this repository's workflow depends on, exercised.
#
# WHY THESE IN PARTICULAR. Between them they refuse commits, refuse to resolve
# somebody's review thread, and open GitHub issues — decisions, not
# transformations, and each was originally verified once by hand in a directory
# that no longer exists. That proves a script worked one afternoon and catches
# nothing afterwards.
#
# It paid for itself on the first run: `mark-thread-resolved.sh` validated its
# input with `[A-Za-z0-9_=-]*`, a `case` glob that matches a first character
# followed by ANYTHING. It accepted `PRRT_x; rm -rf /` and read as validation in
# review.
#
# NO NETWORK, NO REAL REPOSITORY. `gh` and `openspec` are stubbed and every test
# builds a throwaway git repository, so a suite run can neither open an issue nor
# touch the shared checkout — where, by this project's own rule, other sessions
# have uncommitted work.
set -uo pipefail
cd "$(dirname "$0")"

failed=0
for t in *.test.sh; do
  echo "  $t"
  bash "$t" || failed=1
  echo
done

# The documentation gate is asserted TWICE — a local hook and a CI check — so its
# test is about the two agreeing rather than about one being right.
echo "  docs-task-guard (the gate and the check agree)"
../scripts/docs-task-guard-test.sh || failed=1

echo
if [ "$failed" -ne 0 ]; then
  echo "script tests FAILED" >&2
  exit 1
fi
echo "every script test passed"
