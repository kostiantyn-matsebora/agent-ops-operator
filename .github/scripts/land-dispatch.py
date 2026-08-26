#!/usr/bin/env python3
"""Land a dispatch's patch, then answer and close the threads it fixed.

THIS IS THE STEP THAT HOLDS `contents: write`, AND NO MODEL RUNS IN IT. The
fixing job reads the repository and emits two files -- a patch and a report --
and this program does the one fixed thing with them: apply, commit, push,
reply, resolve. Its behaviour cannot be rewritten by what it is handed.

WHAT IT TRUSTS, AND WHAT IT DOES NOT.

  - The WORK LIST came from `accepted-findings.py`, a model-free program that
    walked the threads. It is the set of things this run may touch.
  - The REPORT came from the model. It is a CLAIM about which findings were
    addressed, and it is checked twice: a thread is "fixed" only if it is on
    the work list AND the patch changes the file the finding points at. A
    claim about a thread the list does not carry is dropped and named.
  - The PATCH came from the model. It lands as an ordinary commit on the
    branch, where the review re-reads it and a person still merges. Nothing
    here bypasses review; it produces work for it.

THE ORDER IS THE CORRECTNESS ARGUMENT. Push first, reply second, resolve
third. A reply reading "fixed in <sha>" written before the push is a claim
rather than a report, and a thread resolved before its fix landed is a thread
resolved on the strength of nothing.

A PATCH THAT DOES NOT APPLY RESOLVES NOTHING. The branch moved between review
and dispatch; the recovery is a re-dispatch after a rebase, and this program
says so on the pull request rather than leaving a silent failed run.

Resolution itself is DELEGATED to `resolve-review-threads.py`, which re-reads
every thread and refuses any the review did not author. Two programs that
could close a thread would be two places for that refusal to drift.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import subprocess
import sys


def sh(*cmd: str, check: bool = True, **kw) -> subprocess.CompletedProcess:
    return subprocess.run(list(cmd), capture_output=True, text=True, check=check, **kw)


def gh(*args: str) -> str:
    return sh("gh", *args).stdout


def pr_comment(repo: str, pr: int, body: str) -> None:
    gh("pr", "comment", str(pr), "--repo", repo, "--body", body)


def thread_reply(repo: str, pr: int, comment_id: int, body: str) -> None:
    gh("api", f"repos/{repo}/pulls/{pr}/comments/{comment_id}/replies", "-f", f"body={body}")


def patch_paths(patch: pathlib.Path) -> set[str]:
    """The files a patch changes, as git reads them -- never by parsing the
    patch here. `--numstat` is what `git apply` itself would touch."""
    out = sh("git", "apply", "--numstat", str(patch)).stdout
    return {line.split("\t")[2] for line in out.splitlines() if line.count("\t") >= 2}


def first_line(text: str, limit: int = 72) -> str:
    line = (text or "").strip().splitlines()[0] if (text or "").strip() else ""
    return line if len(line) <= limit else line[: limit - 1] + "…"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--pr", type=int, required=True)
    ap.add_argument("--branch", required=True, help="the pull request's head branch")
    ap.add_argument("--work-list", type=pathlib.Path, required=True,
                    help="accepted-findings.py output")
    ap.add_argument("--patch", type=pathlib.Path, required=True)
    ap.add_argument("--report", type=pathlib.Path, required=True,
                    help='the fixing step\'s {"fixed":[...],"unfixed":[{"threadId","reason"}]}')
    ap.add_argument("--resolve-list", type=pathlib.Path, default=pathlib.Path(".resolve-threads"))
    ap.add_argument("--resolver", type=pathlib.Path,
                    default=pathlib.Path(__file__).with_name("resolve-review-threads.py"))
    ap.add_argument("--dispatched-by", default="a maintainer")
    ap.add_argument("--run-url", default="")
    args = ap.parse_args()

    # ABSENT AND EMPTY ARE DIFFERENT FACTS, for every one of the three inputs.
    # The fixing job writes all of them before uploading, so a missing file is
    # a broken transfer rather than a quiet run.
    for name, path in (("work list", args.work_list), ("patch", args.patch), ("report", args.report)):
        if not path.is_file():
            print(f"the {name} is ABSENT at {path}: the transfer from the fixing job broke, "
                  "so nothing was landed and nothing was resolved.", file=sys.stderr)
            return 1

    work = {item["threadId"]: item for item in json.loads(args.work_list.read_text())}
    report = json.loads(args.report.read_text() or "{}")
    run = f" ([run]({args.run_url}))" if args.run_url else ""

    if not work:
        print("nothing accepted: the work list is empty, so there is nothing to land")
        pr_comment(args.repo, args.pr,
                   f"Dispatch by @{args.dispatched_by}: no finding is accepted, so nothing was "
                   f"written. Reply `fix it` under a finding to accept it.{run}")
        return 0

    claimed_fixed = [t for t in report.get("fixed", []) if isinstance(t, str)]
    claimed_unfixed = {u.get("threadId"): u.get("reason", "no reason given")
                       for u in report.get("unfixed", []) if isinstance(u, dict)}
    for stray in [t for t in claimed_fixed if t not in work] + [t for t in claimed_unfixed if t not in work]:
        print(f"  DROPPED  {stray}: the report names a thread the work list does not carry")

    patch_empty = args.patch.stat().st_size == 0
    touched = set() if patch_empty else patch_paths(args.patch)

    fixed: list[str] = []
    unfixed: dict[str, str] = {}
    for thread_id, item in work.items():
        if thread_id in claimed_fixed and item["path"] in touched:
            fixed.append(thread_id)
        elif thread_id in claimed_fixed:
            unfixed[thread_id] = f"reported fixed, but the patch does not touch `{item['path']}`"
        else:
            unfixed[thread_id] = claimed_unfixed.get(thread_id, "not addressed by the fixing step")

    for t in fixed:
        print(f"  fixed    {t} ({work[t]['path']})")
    for t, why in unfixed.items():
        print(f"  unfixed  {t} ({work[t]['path']}): {why}")

    if not fixed:
        print("no finding was fixed, so nothing is committed and nothing is resolved")
        lines = "\n".join(f"- `{work[t]['path']}`: {why}" for t, why in unfixed.items())
        pr_comment(args.repo, args.pr,
                   f"Dispatch by @{args.dispatched_by}: nothing landed. Every accepted finding "
                   f"is still open:\n\n{lines}{run}")
        return 0

    # APPLY. `--check` first so a stale patch changes nothing and says so.
    check = sh("git", "apply", "--check", str(args.patch), check=False)
    if check.returncode != 0:
        print(f"the patch does not apply:\n{check.stderr}", file=sys.stderr)
        pr_comment(args.repo, args.pr,
                   f"Dispatch by @{args.dispatched_by}: the patch no longer applies to "
                   f"`{args.branch}` — the branch moved since the review. Nothing was pushed and "
                   f"no thread was resolved. Rebase, then dispatch again.{run}\n\n"
                   f"```\n{check.stderr.strip()}\n```")
        return 1
    sh("git", "apply", "--index", str(args.patch))

    subject = f"fix(review): address {len(fixed)} accepted review finding{'s' if len(fixed) != 1 else ''}"
    body = "\n".join(f"- {work[t]['path']}:{work[t].get('line') or '?'} — {first_line(work[t]['finding'])}"
                     for t in fixed)
    trailer = f"Dispatched-By: {args.dispatched_by}"
    sh("git", "commit", "-q", "-m", subject, "-m", body, "-m", trailer)
    sha = sh("git", "rev-parse", "HEAD").stdout.strip()

    # PUSH BEFORE ANY REPLY. What follows reports a fact.
    push = sh("git", "push", "origin", f"HEAD:refs/heads/{args.branch}", check=False)
    if push.returncode != 0:
        print(f"push failed:\n{push.stderr}", file=sys.stderr)
        pr_comment(args.repo, args.pr,
                   f"Dispatch by @{args.dispatched_by}: the fix was produced but could not be "
                   f"pushed to `{args.branch}`. Nothing landed and no thread was resolved.{run}\n\n"
                   f"```\n{push.stderr.strip()}\n```")
        return 1
    print(f"pushed {sha[:7]} to {args.branch}")

    for t in fixed:
        thread_reply(args.repo, args.pr, work[t]["commentId"],
                     f"Fixed in {sha[:7]}, dispatched by @{args.dispatched_by}.{run}")
    for t, why in unfixed.items():
        thread_reply(args.repo, args.pr, work[t]["commentId"],
                     f"Not addressed by the dispatch ({why}). Left open.{run}")

    # THEN resolve, through the program that already refuses anything the
    # review did not author.
    args.resolve_list.write_text("".join(f"{t}\n" for t in fixed))
    resolver = subprocess.run([sys.executable, str(args.resolver), "--repo", args.repo,
                               "--pr", str(args.pr), "--file", str(args.resolve_list)],
                              capture_output=True, text=True)
    print(resolver.stdout, end="")
    if resolver.returncode != 0:
        print(resolver.stderr, file=sys.stderr)
        return resolver.returncode

    # A PUSH MADE WITH THE WORKFLOW TOKEN STARTS NO WORKFLOW. GitHub withholds
    # `synchronize` from it, so this commit has neither CI nor a review until
    # somebody pushes again. Merge stays blocked meanwhile, which is the safe
    # side; the comment is what stops it reading as CI silently passing.
    left = f", {len(unfixed)} left open" if unfixed else ""
    pr_comment(args.repo, args.pr,
               f"Dispatch by @{args.dispatched_by}: {sha[:7]} addresses {len(fixed)} accepted "
               f"finding{'s' if len(fixed) != 1 else ''}{left}.{run}\n\n"
               "A commit pushed by a workflow starts no workflow, so CI and the review have NOT "
               "run on it. Push again (an empty commit will do) to get both.")
    print(f"\n{len(fixed)} fixed and pushed as {sha[:7]}, {len(unfixed)} left open")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except subprocess.CalledProcessError as exc:
        print(f"{exc.cmd[0]} failed: {exc.stderr or exc}", file=sys.stderr)
        sys.exit(1)
