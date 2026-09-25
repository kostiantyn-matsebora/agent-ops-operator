#!/usr/bin/env bash
# The conveyor's state machine, over every combination of its facts. The cases
# are Python (unittest, standard library only); this wrapper is what run.sh
# discovers. See conveyor.test.py for what each block holds.
cd "$(dirname "$0")" || exit 1
python3 conveyor.test.py 2>&1 | tail -3
exit "${PIPESTATUS[0]}"
