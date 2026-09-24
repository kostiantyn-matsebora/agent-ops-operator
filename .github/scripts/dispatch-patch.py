#!/usr/bin/env python3
"""Cut the fixing step's patch: what the model changed, minus what it left behind.

THE PATCH IS THE WHOLE OF WHAT CROSSES from the fixing job to the landing job,
and it used to be `git add -N . && git diff --binary` -- every file in the
working tree, tracked or not. The fixer writes its scratch INTO the checkout,
because the checkout is its working directory: on #243 six consecutive rounds
each landed a helper it had written to find its own inputs (`.tmp_getenv.py`,
`.tmp_read_worklist.sh`, `.worklist_reader.sh`, `.scratch_read.sh`, ...) at the
repository root, beside the real fix. The review then found the scratch, the
next round disputed that finding, and `docs-task` refused the head until a
person answered -- a red check caused by nothing anybody wrote on purpose.

A NEW FILE CROSSES ONLY WHERE THE REPORT DECLARES IT. The report's `created`
list is the model's own claim of what it added; an untracked file it does not
name is deleted before the patch is cut and recorded in `--dropped`, so the
landing summary can say what did not land. A tracked file's change needs no
declaration: nothing the fixer modifies is scratch, since it did not create it.

Ignored files (`.gitignore`) are neither landed nor dropped -- `git add` never
staged them before either, and a check reproduced in the checkout leaves its
`coverage.out` behind legitimately.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import subprocess
import sys


def sh(*args: str) -> subprocess.CompletedProcess:
    return subprocess.run(args, capture_output=True, text=True, check=True)


def declared(report: pathlib.Path) -> set[str]:
    """The `created` paths the report names, normalised as git names them.
    A missing or unreadable report declares nothing: a run that wrote no
    report gets no new file landed on its behalf."""
    if not report.is_file():
        return set()
    try:
        data = json.loads(report.read_text() or "{}")
    except json.JSONDecodeError:
        return set()
    paths = data.get("created") if isinstance(data, dict) else None
    if not isinstance(paths, list):
        return set()
    return {str(pathlib.PurePosixPath(p)).lstrip("/") for p in paths if isinstance(p, str) and p.strip()}


def untracked() -> list[str]:
    out = sh("git", "ls-files", "--others", "--exclude-standard", "-z").stdout
    return [p for p in out.split("\0") if p]


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--report", type=pathlib.Path, required=True,
                    help='the fixing step\'s report; its optional "created" list names the new files that may land')
    ap.add_argument("--out", type=pathlib.Path, required=True, help="where the patch is written")
    ap.add_argument("--dropped", type=pathlib.Path, required=True,
                    help='where {"dropped": [...]} is written -- the undeclared new files deleted before the cut')
    args = ap.parse_args()

    keep = declared(args.report)
    dropped: list[str] = []
    for path in untracked():
        if path in keep:
            sh("git", "add", "-N", "--", path)
            print(f"new file, declared: {path}")
        else:
            pathlib.Path(path).unlink()
            dropped.append(path)
            print(f"::warning::{path}: created by the fixing step but not declared in its report; "
                  "deleted before the patch was cut")
    for path in sorted(keep):
        if not pathlib.Path(path).exists():
            print(f"::warning::{path}: declared as created but not present in the checkout")

    patch = sh("git", "diff", "--binary").stdout
    args.out.write_text(patch)
    args.dropped.write_text(json.dumps({"dropped": dropped}) + "\n")
    print(f"patch: {len(patch.encode())} bytes, {len(dropped)} undeclared new file(s) dropped")
    return 0


if __name__ == "__main__":
    sys.exit(main())
