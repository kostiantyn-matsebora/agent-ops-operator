#!/usr/bin/env python3
"""PostToolUse hook: after a markdown file is written, check it against the
writing rules and hand the findings back to the model.

WHY A HOOK AND NOT A RULE. A rule in a context file is followed until the
evening somebody is tired, or until it is compacted away. The harness runs
this on every `Write` and `Edit`, so the check does not depend on the model
having the rule in context at the moment it matters.

EXIT 2 IS THE ANSWER. A PostToolUse hook cannot undo the write, and should
not: it reports on stderr, and the harness feeds that back to the model,
which then fixes the file. The decision is not made here —
`.claude/scripts/rules_compliance.py` is the one implementation, and a person
runs the same script over a tree by hand.

FAILS OPEN on anything it cannot read — a payload that is not JSON, a tool
that wrote no file, a path that is not markdown or no longer exists, an
import that fails. A hook that blocks work it does not understand gets
disabled, and then it enforces nothing at all.
"""
from __future__ import annotations

import json
import pathlib
import sys


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except (ValueError, OSError):
        return 0
    if not isinstance(payload, dict):
        return 0
    tool_input = payload.get("tool_input") or {}
    file_path = tool_input.get("file_path") if isinstance(tool_input, dict) else None
    if not isinstance(file_path, str) or not file_path.endswith(".md"):
        return 0
    path = pathlib.Path(file_path)
    if not path.is_file():
        return 0
    try:
        sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent.parent / "scripts"))
        import rules_compliance  # noqa: E402
        findings = rules_compliance.check(path)
    except Exception:  # noqa: BLE001 — fail open, by design
        return 0
    if not findings:
        return 0
    lines = [f"The file you just wrote breaks the writing rules (.claude/rules/writing.md; "
             "for a rules file also authoring.md). Fix these, then continue:"]
    lines += [f"  {f.path}:{f.line} {f.rule}" for f in findings]
    lines.append("semicolon: one thought per sentence — restructure, do not swap for a full stop. "
                 "long-paragraph: past about three lines it stops being read — a list or a table. "
                 "rules-heading / rules-second-h2: a rules file opens with one `## ` heading and holds one topic.")
    print("\n".join(lines), file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())
