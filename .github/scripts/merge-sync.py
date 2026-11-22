#!/usr/bin/env python3
"""Bring a pull request's branch up to date with its base, or say why it cannot be.

Run in a checkout of the branch. It merges `origin/<base>` into HEAD and reports ONE of:

  uptodate     the base is already in the branch: nothing to do
  updated      the base merged with no conflict marker: a merge commit is made
  regenerated  only generated files conflicted: the base's copies were taken and the
               repository's own regenerators re-run (`testing.regenerators`)
  asked        a hand-written file conflicts, or a regenerator failed: NOTHING is
               committed, the merge is aborted, and the files are named

THE MODEL NEVER RESOLVES A CONFLICT. This runs no model. A trivial conflict is one a
repository script can answer, and everything else is two people's intent, which a
person reads. `asked` exits 3 so the workflow can set the loop `waiting` and comment.

The script commits locally and never pushes: the workflow's deploy-key step pushes, as
it does for the fixer's patch, and only after the tests have run on the result.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import subprocess
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import conveyor  # noqa: E402

VOCAB = pathlib.Path(__file__).resolve().parents[1] / "review-triage.json"
MARKERS = ("<<<<<<< ", "=======\n", ">>>>>>> ")


def git(*args, check=True):
    return subprocess.run(["git", *args], capture_output=True, text=True, check=check)


def conflicted_files() -> list:
    return [f for f in git("diff", "--name-only", "--diff-filter=U").stdout.splitlines() if f]


def has_marker(path: str) -> bool:
    p = pathlib.Path(path)
    if not p.is_file():
        return False
    text = p.read_text(errors="replace")
    return any(line.startswith("<<<<<<< ") or line.startswith(">>>>>>> ") for line in text.splitlines())


def regenerators_for(files, regenerators) -> list:
    """The distinct commands whose path globs cover any of `files`, in declared order."""
    out = []
    for r in regenerators:
        if any(conveyor.path_matches(f, r["paths"]) for f in files) and r["run"] not in out:
            out.append(r["run"])
    return out


def sync(base: str, vocab: dict, message: str) -> dict:
    t = vocab["testing"]
    ref = f"origin/{base}"
    if git("merge-base", "--is-ancestor", ref, "HEAD", check=False).returncode == 0:
        return {"action": "uptodate", "files": []}
    r = git("merge", "--no-ff", "--no-commit", ref, check=False)
    files = conflicted_files()
    if r.returncode == 0 and not files:
        git("commit", "--no-edit", "-m", message)
        return {"action": "updated", "files": []}
    if not files:
        git("merge", "--abort", check=False)
        return {"action": "asked", "files": [], "reason": (r.stderr or r.stdout).strip().splitlines()[-1:]}
    if conveyor.classify_conflict(files, t["generated_paths"]) != "trivial":
        git("merge", "--abort", check=False)
        return {"action": "asked", "files": sorted(files)}
    # Take the base's copy of each generated file, then let the repository's own scripts rebuild it.
    for f in files:
        git("checkout", "--theirs", "--", f, check=False)
        git("add", "--", f)
    for cmd in regenerators_for(files, t.get("regenerators", [])):
        if subprocess.run(cmd, shell=True, capture_output=True, text=True).returncode != 0:
            git("merge", "--abort", check=False)
            return {"action": "asked", "files": sorted(files), "reason": "a regenerator failed"}
    git("add", "-A", "--", *files)
    if any(has_marker(f) for f in files):
        git("merge", "--abort", check=False)
        return {"action": "asked", "files": sorted(files), "reason": "a conflict marker survived"}
    git("commit", "--no-edit", "-m", message)
    return {"action": "regenerated", "files": sorted(files)}


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--base", default="master")
    ap.add_argument("--vocabulary", type=pathlib.Path, default=VOCAB)
    ap.add_argument("--message", default="chore: merge the base into the branch")
    args = ap.parse_args(argv)
    result = sync(args.base, json.loads(args.vocabulary.read_text()), args.message)
    json.dump(result, sys.stdout, sort_keys=True)
    print()
    return 3 if result["action"] == "asked" else 0


if __name__ == "__main__":
    sys.exit(main())
