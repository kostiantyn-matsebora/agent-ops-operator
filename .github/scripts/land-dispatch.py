#!/usr/bin/env python3
"""Land a dispatch's patch, then answer and close the threads it fixed.

THIS IS THE STEP THAT HOLDS `contents: write`, AND NO MODEL RUNS IN IT. The
fixing job reads the repository and emits two files -- a patch and a report --
and this program does the one fixed thing with them: apply, commit, push,
reply, resolve. Its behaviour cannot be rewritten by what it is handed.

WHAT IT TRUSTS, AND WHAT IT DOES NOT.

  - The WORK LIST came from `accepted-findings.py` (and, on a labelled pull
    request, `sonar-issues.py`), model-free programs that walked the threads
    and asked the analysis service. It is the set of things this run may
    touch. Every item carries an `id`: a thread id, or `sonar:<issue key>`.
  - The REPORT came from the model. It is a CLAIM about which items were
    addressed, and it is checked twice: an item is "fixed" only if it is on
    the work list AND the patch changes the file the item points at. A claim
    about an id the list does not carry is dropped and named.
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

TWO MODES, ONE PROGRAM.

  threads   a person accepted findings in their threads and sent a dispatch.
            Everything above, then a comment saying the landed commit has no
            CI and no review until somebody pushes again -- a push made with
            the workflow token starts no workflow.
  all       the pull request carries the approval label (`.github/
            review-triage.json`, `approve_label`). Every item is FIXED or
            DISPUTED, never dropped: a disputed thread gets a reply under the
            dispute marker and stays open, disputed analysis issues get ONE
            pull request comment under the same marker, and the person who
            placed the label is mentioned. The landed commit RE-TRIGGERS
            `ci.yml` and the review by `workflow_dispatch` -- the documented
            exception to the token rule -- so the next round starts by itself.
            Rounds are counted from this program's own round-marked comments
            since the label was placed (state on the pull request, derivable,
            no store), capped at `--max-rounds`, and every ending posts ONE
            summary.

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

DEFAULT_VOCABULARY = pathlib.Path(__file__).resolve().parents[1] / "review-triage.json"
SUMMARY_MARKER = "<!-- autofix:summary -->"


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


def plural(n: int, word: str) -> str:
    return f"{n} {word}{'' if n == 1 else 's'}"


def load_markers(path: pathlib.Path) -> dict:
    try:
        doc = json.loads(path.read_text())
    except (OSError, json.JSONDecodeError):
        doc = {}
    return {"dispute": doc.get("dispute_marker", "<!-- autofix:disputed -->"),
            "round": doc.get("round_marker", "<!-- autofix:round")}


def read_report(report: dict, work: dict) -> tuple[list[str], dict[str, str]]:
    """The model's claims, in either shape it may have written:
    {"items":[{"id","action","reason"}]} or the older {"fixed":[...],
    "unfixed":[{"threadId","reason"}]}. Returns (claimed fixed ids,
    {disputed id: reason}); ids the work list does not carry are dropped and
    named."""
    claimed_fixed: list[str] = []
    claimed_disputed: dict[str, str] = {}
    for entry in report.get("items", []) or []:
        if not isinstance(entry, dict):
            continue
        item_id = entry.get("id") or entry.get("threadId")
        if entry.get("action") == "fixed":
            claimed_fixed.append(item_id)
        else:
            claimed_disputed[item_id] = entry.get("reason") or "no reason given"
    claimed_fixed += [t for t in report.get("fixed", []) or [] if isinstance(t, str)]
    for u in report.get("unfixed", []) or []:
        if isinstance(u, dict):
            claimed_disputed[u.get("id") or u.get("threadId")] = u.get("reason", "no reason given")
    for stray in [t for t in claimed_fixed if t not in work] + [t for t in claimed_disputed if t not in work]:
        print(f"  DROPPED  {stray}: the report names an item the work list does not carry")
    return [t for t in claimed_fixed if t in work], {t: r for t, r in claimed_disputed.items() if t in work}


def where(item: dict) -> str:
    return f"{item.get('path')}:{item.get('line') or '?'}"


def describe(item: dict) -> str:
    if item.get("source") == "sonar":
        return f"`{where(item)}` — {item.get('rule')}: {first_line(item.get('message', ''))}"
    return f"`{where(item)}` — {first_line(item.get('finding', ''))}"


def parse_comments(raw: str) -> list[dict]:
    """`gh api --paginate` concatenates pages as adjacent JSON arrays."""
    try:
        return json.loads(raw or "[]")
    except json.JSONDecodeError:
        comments: list[dict] = []
        for chunk in raw.replace("][", "]\n[").splitlines():
            if chunk.strip():
                comments.extend(json.loads(chunk))
        return comments


class Round:
    """The labelled pull request's bookkeeping: which round this is, and the
    one summary an ending posts."""

    def __init__(self, args, markers: dict, work: dict):
        self.args = args
        self.markers = markers
        self.work = work
        self.number = self.count_rounds() + 1
        self.sonar = self.read_sonar()

    def read_sonar(self) -> dict:
        if self.args.sonar and self.args.sonar.is_file():
            try:
                return json.loads(self.args.sonar.read_text() or "{}")
            except json.JSONDecodeError:
                return {}
        return {}

    def count_rounds(self) -> int:
        """Landing comments carrying the round marker, since the label was
        placed. The label's timestamp is what makes re-labelling a fresh count."""
        try:
            raw = gh("api", f"repos/{self.args.repo}/issues/{self.args.pr}/comments", "--paginate")
        except subprocess.CalledProcessError:
            return 0
        n = 0
        for c in parse_comments(raw):
            if self.markers["round"] not in (c.get("body") or ""):
                continue
            if self.args.since and (c.get("created_at") or "") < self.args.since:
                continue
            n += 1
        return n

    def marker(self) -> str:
        return f"{self.markers['round']} {self.number} -->"

    def summary(self, ending: str, fixed: list[str], disputed: dict[str, str], sha: str | None = None,
                note: str = "") -> None:
        """ONE comment per ending: what was fixed, what was disputed, rounds
        used, what remains, and the approver mentioned."""
        a = self.args
        used = self.number if sha else self.number - 1
        lines = [SUMMARY_MARKER, f"**Autofix on #{a.pr}: {ending}** — @{a.approver}"]
        if note:
            lines.append(note)
        lines.append(f"Rounds used: {used} of {a.max_rounds}.")
        if fixed:
            lines.append(f"\nFixed{f' in {sha[:7]}' if sha else ''} ({len(fixed)}):")
            lines += [f"- {describe(self.work[t])}" for t in fixed]
        if disputed:
            lines.append(f"\nDisputed ({len(disputed)}) — each stays open until you answer it:")
            lines += [f"- {describe(self.work[t])}: {why}" for t, why in disputed.items()]
        remaining = [t for t in self.work if t not in fixed]
        if remaining:
            lines.append(f"\nStill open: {plural(len(remaining), 'item')} from this round.")
        elif fixed:
            lines.append("\nNothing from this round is left open.")
        if self.sonar:
            if not self.sonar.get("consulted"):
                stale = self.sonar.get("stale") or []
                lines.append("\nThe analysis service was NOT consulted this round: it had no analysis of the "
                             "head commit" + (f" (stale for {', '.join(stale)})" if stale else "")
                             + ". Its next analysis runs with CI.")
            else:
                lines.append(f"\nThe analysis service reported {plural(len(self.sonar.get('issues') or []), 'open issue')}"
                             " for the head commit.")
        if a.run_url:
            lines.append(f"\n[run]({a.run_url})")
        pr_comment(a.repo, a.pr, "\n".join(lines))
        print(f"summary posted: {ending}")

    def retrigger(self, sha: str) -> list[str]:
        """The next round: CI on the branch (its check runs attach to the head
        sha) and the review from the default branch's copy, both by
        `workflow_dispatch`, which a workflow-token push may start."""
        started = []
        for wf, ref, inp in ((self.args.ci_workflow, self.args.branch, f"pr={self.args.pr}"),
                             (self.args.review_workflow, self.args.default_branch, f"number={self.args.pr}")):
            r = sh("gh", "workflow", "run", wf, "--repo", self.args.repo, "--ref", ref, "-f", inp, check=False)
            if r.returncode == 0:
                started.append(wf)
                print(f"dispatched {wf} on {ref} for {sha[:7]}")
            else:
                print(f"could not dispatch {wf}: {r.stderr.strip()}", file=sys.stderr)
        return started


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--pr", type=int, required=True)
    ap.add_argument("--branch", required=True, help="the pull request's head branch")
    ap.add_argument("--work-list", type=pathlib.Path, required=True,
                    help="the collect job's output: accepted findings, plus analysis issues in --mode all")
    ap.add_argument("--patch", type=pathlib.Path, required=True)
    ap.add_argument("--report", type=pathlib.Path, required=True,
                    help='the fixing step\'s {"items":[{"id","action","reason"}]}')
    ap.add_argument("--resolve-list", type=pathlib.Path, default=pathlib.Path(".resolve-threads"))
    ap.add_argument("--resolver", type=pathlib.Path,
                    default=pathlib.Path(__file__).with_name("resolve-review-threads.py"))
    ap.add_argument("--dispatched-by", default="a maintainer")
    ap.add_argument("--run-url", default="")
    ap.add_argument("--mode", choices=["threads", "all"], default="threads")
    ap.add_argument("--approver", default="", help="--mode all: who placed the label; mentioned in every summary")
    ap.add_argument("--since", default="", help="--mode all: when the label was placed (ISO 8601); rounds are counted from then")
    ap.add_argument("--max-rounds", type=int, default=3)
    ap.add_argument("--sonar", type=pathlib.Path, help="--mode all: sonar-issues.py output, for the summary")
    ap.add_argument("--vocabulary", type=pathlib.Path, default=DEFAULT_VOCABULARY)
    ap.add_argument("--ci-workflow", default="ci.yml")
    ap.add_argument("--review-workflow", default="", help="--mode all: the review workflow to dispatch (named by the workflow, not here)")
    ap.add_argument("--default-branch", default="master")
    args = ap.parse_args()
    if args.mode == "all" and not args.approver:
        args.approver = args.dispatched_by

    # ABSENT AND EMPTY ARE DIFFERENT FACTS, for every one of the three inputs.
    # The fixing job writes all of them before uploading, so a missing file is
    # a broken transfer rather than a quiet run.
    for name, path in (("work list", args.work_list), ("patch", args.patch), ("report", args.report)):
        if not path.is_file():
            print(f"the {name} is ABSENT at {path}: the transfer from the fixing job broke, "
                  "so nothing was landed and nothing was resolved.", file=sys.stderr)
            return 1

    work = {item.get("id") or item["threadId"]: item for item in json.loads(args.work_list.read_text())}
    for item in work.values():
        item.setdefault("source", "review")
    report = json.loads(args.report.read_text() or "{}")
    run = f" ([run]({args.run_url}))" if args.run_url else ""
    markers = load_markers(args.vocabulary)
    rnd = Round(args, markers, work) if args.mode == "all" else None

    if not work:
        print("nothing accepted: the work list is empty, so there is nothing to land")
        if rnd:
            rnd.summary("clean", [], {},
                        note="The review has no open finding and the analysis reports no open issue.")
            return 0
        pr_comment(args.repo, args.pr,
                   f"Dispatch by @{args.dispatched_by}: no finding is accepted, so nothing was "
                   f"written. Reply `fix it` under a finding to accept it.{run}")
        return 0

    claimed_fixed, claimed_disputed = read_report(report, work)

    patch_empty = args.patch.stat().st_size == 0
    touched = set() if patch_empty else patch_paths(args.patch)

    fixed: list[str] = []
    disputed: dict[str, str] = {}
    for item_id, item in work.items():
        if item_id in claimed_fixed and item["path"] in touched:
            fixed.append(item_id)
        elif item_id in claimed_fixed:
            disputed[item_id] = f"reported fixed, but the patch does not touch `{item['path']}`"
        else:
            disputed[item_id] = claimed_disputed.get(item_id, "not addressed by the fixing step")

    for t in fixed:
        print(f"  fixed    {t} ({work[t]['path']})")
    for t, why in disputed.items():
        print(f"  {'disputed' if rnd else 'unfixed '} {t} ({work[t]['path']}): {why}")

    def post_disputes() -> None:
        """--mode all: a marked reply per disputed thread, one comment for
        every disputed analysis issue. The marker is what the next round and
        the archive guard read."""
        for t, why in disputed.items():
            item = work[t]
            if item["source"] == "review":
                thread_reply(args.repo, args.pr, item["commentId"],
                             f"{markers['dispute']}\nDisputed by the fixing step: {why}\n\n"
                             f"Left open for @{args.approver} — reply here to answer it, or resolve the thread to dismiss it.{run}")
        sonar = {t: why for t, why in disputed.items() if work[t]["source"] == "sonar"}
        if sonar:
            lines = "\n".join(f"- `{work[t]['key']}` ({work[t].get('rule')}, `{where(work[t])}`): {why}"
                              for t, why in sonar.items())
            pr_comment(args.repo, args.pr,
                       f"{markers['dispute']}\nThe fixing step disputes {plural(len(sonar), 'analysis issue')}, "
                       f"for @{args.approver}. Nothing was changed in the analysis service; mark them there "
                       f"if you agree, or answer here.\n\n{lines}{run}")

    if not fixed:
        print("no finding was fixed, so nothing is committed and nothing is resolved")
        if rnd:
            post_disputes()
            rnd.summary("disputes only", [], disputed,
                        note="Every item this round was disputed and none was fixed, so the loop ends here.")
            return 0
        lines = "\n".join(f"- `{work[t]['path']}`: {why}" for t, why in disputed.items())
        pr_comment(args.repo, args.pr,
                   f"Dispatch by @{args.dispatched_by}: nothing landed. Every accepted finding "
                   f"is still open:\n\n{lines}{run}")
        return 0

    # APPLY. `--check` first so a stale patch changes nothing and says so.
    check = sh("git", "apply", "--check", str(args.patch), check=False)
    if check.returncode != 0:
        print(f"the patch does not apply:\n{check.stderr}", file=sys.stderr)
        if rnd:
            rnd.summary("stale patch", [], {},
                        note=f"The patch no longer applies to `{args.branch}` — the branch moved since the "
                             f"review. Nothing was pushed and no thread was resolved; the loop ends here. "
                             f"Rebase, then remove and re-add the label to start it again.\n\n"
                             f"```\n{check.stderr.strip()}\n```")
            return 1
        pr_comment(args.repo, args.pr,
                   f"Dispatch by @{args.dispatched_by}: the patch no longer applies to "
                   f"`{args.branch}` — the branch moved since the review. Nothing was pushed and "
                   f"no thread was resolved. Rebase, then dispatch again.{run}\n\n"
                   f"```\n{check.stderr.strip()}\n```")
        return 1
    sh("git", "apply", "--index", str(args.patch))

    n_review = sum(1 for t in fixed if work[t]["source"] == "review")
    n_sonar = len(fixed) - n_review
    if rnd:
        parts = [plural(n_review, "review finding")] if n_review else []
        parts += [plural(n_sonar, "analysis issue")] if n_sonar else []
        subject = f"fix(review): address {' and '.join(parts)} (autofix round {rnd.number})"
    else:
        subject = f"fix(review): address {len(fixed)} accepted review finding{'s' if len(fixed) != 1 else ''}"
    body = "\n".join(f"- {work[t]['path']}:{work[t].get('line') or '?'} — "
                     f"{first_line(work[t].get('finding') or work[t].get('message', ''))}"
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
        if work[t]["source"] == "review":
            thread_reply(args.repo, args.pr, work[t]["commentId"],
                         f"Fixed in {sha[:7]}, dispatched by @{args.dispatched_by}.{run}")
    if rnd:
        post_disputes()
    else:
        for t, why in disputed.items():
            thread_reply(args.repo, args.pr, work[t]["commentId"],
                         f"Not addressed by the dispatch ({why}). Left open.{run}")

    # THEN resolve, through the program that already refuses anything the
    # review did not author. Analysis issues have no thread; the service
    # closes them on its next analysis.
    args.resolve_list.write_text("".join(f"{t}\n" for t in fixed if work[t]["source"] == "review"))
    resolver = subprocess.run([sys.executable, str(args.resolver), "--repo", args.repo,
                               "--pr", str(args.pr), "--file", str(args.resolve_list)],
                              capture_output=True, text=True)
    print(resolver.stdout, end="")
    if resolver.returncode != 0:
        print(resolver.stderr, file=sys.stderr)
        return resolver.returncode

    left = f", {len(disputed)} left open" if disputed else ""
    if rnd:
        # THE ROUND IS RECORDED ON THE PULL REQUEST, in the comment that
        # announces the landing. Then either the cap ends the loop, or the
        # next round is started by `workflow_dispatch`.
        pr_comment(args.repo, args.pr,
                   f"{rnd.marker()}\nAutofix round {rnd.number} of {args.max_rounds}: {sha[:7]} addresses "
                   f"{plural(len(fixed), 'item')}{left}.{run}")
        if rnd.number >= args.max_rounds:
            rnd.summary("round cap reached", fixed, disputed, sha=sha,
                        note=f"{args.max_rounds} rounds have run; no further round starts. "
                             "Remove and re-add the label to run more.")
        else:
            started = rnd.retrigger(sha)
            if len(started) < 2:
                rnd.summary("could not start the next round", fixed, disputed, sha=sha,
                            note="The landed commit could not be re-checked: dispatching "
                                 f"{'CI' if args.ci_workflow not in started else 'the review'} failed. "
                                 "Push again (an empty commit will do) to get CI and the review.")
                return 1
            print(f"round {rnd.number} landed; the next starts when the review completes")
    else:
        # A PUSH MADE WITH THE WORKFLOW TOKEN STARTS NO WORKFLOW. GitHub
        # withholds `synchronize` from it, so this commit has neither CI nor a
        # review until somebody pushes again. Merge stays blocked meanwhile,
        # which is the safe side; the comment is what stops it reading as CI
        # silently passing.
        pr_comment(args.repo, args.pr,
                   f"Dispatch by @{args.dispatched_by}: {sha[:7]} addresses {len(fixed)} accepted "
                   f"finding{'s' if len(fixed) != 1 else ''}{left}.{run}\n\n"
                   "A commit pushed by a workflow starts no workflow, so CI and the review have NOT "
                   "run on it. Push again (an empty commit will do) to get both.")
    print(f"\n{len(fixed)} fixed and pushed as {sha[:7]}, {len(disputed)} left open")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except subprocess.CalledProcessError as exc:
        print(f"{exc.cmd[0]} failed: {exc.stderr or exc}", file=sys.stderr)
        sys.exit(1)
