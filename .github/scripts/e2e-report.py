#!/usr/bin/env python3
"""A markdown report of one `go test -tags e2e -json` run, for the Actions
run summary.

Reads test2json events (one JSON object per line — `go test -json`'s output)
and renders one of two things, chosen by `--level`:

  summary   pass/fail/skip counts, total wall-clock elapsed, and the names of
            any test that did not pass. For the full tier: many lanes, a
            nightly cadence, and a report meant to be skimmed rather than
            read start to end.
  full      every test's name, status and elapsed time, plus the captured
            output of any failure. For the smoke tier: short, deterministic,
            and worth reading in full from the summary page alone.

Which tier gets which level is a decision the CALLER makes (see
openspec/changes/e2e-report-levels/design.md) — this script only renders the
level it is given.

    e2e-report.py --events events.jsonl --level summary >> "$GITHUB_STEP_SUMMARY"

A file with no parseable test events — most often a build failure, which
`go test` reports before test2json ever emits a `run` action — renders a
one-line report saying so. This script always exits 0: the "Run the pack"
step is what fails the job, and this one must never double-report that
failure under a second name.
"""
from __future__ import annotations

import argparse
import datetime as dt
import json
import pathlib
import sys

TERMINAL_ACTIONS = {"pass", "fail", "skip"}


def load_events(path: pathlib.Path) -> list[dict]:
    if not path.exists():
        return []
    out = []
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(event, dict):
            out.append(event)
    return out


def tests_from(events: list[dict]) -> dict[str, dict]:
    """One entry per `Test` name — subtests kept distinct from their parent,
    since a summary's failed-test list must say which subtest failed, not
    just which top-level test contains one. First-seen order."""
    tests: dict[str, dict] = {}
    for event in events:
        name = event.get("Test")
        if not name:
            continue  # package-level events (no Test) carry no per-test result
        test = tests.setdefault(name, {"status": None, "elapsed": None, "output": []})
        action = event.get("Action")
        if action in TERMINAL_ACTIONS:
            test["status"] = action
            test["elapsed"] = event.get("Elapsed")
        elif action == "output":
            output = event.get("Output")
            if output:
                test["output"].append(output)
    return tests


def parse_time(value: str | None) -> dt.datetime | None:
    if not value:
        return None
    try:
        return dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None


def wall_elapsed(events: list[dict]) -> str:
    times = [t for t in (parse_time(e.get("Time")) for e in events) if t is not None]
    if len(times) < 2:
        return "unknown"
    seconds = int((max(times) - min(times)).total_seconds())
    hours, seconds = divmod(seconds, 3600)
    minutes, seconds = divmod(seconds, 60)
    if hours:
        return f"{hours}h{minutes}m{seconds}s"
    if minutes:
        return f"{minutes}m{seconds}s"
    return f"{seconds}s"


def render_summary(tests: dict[str, dict], elapsed: str) -> str:
    counts = {"pass": 0, "fail": 0, "skip": 0, None: 0}
    failed, skipped = [], []
    for name, test in tests.items():
        counts[test["status"]] = counts.get(test["status"], 0) + 1
        if test["status"] == "fail":
            failed.append(name)
        elif test["status"] == "skip":
            skipped.append(name)

    lines = [
        "## E2E report (summary)",
        "",
        f"**{counts['pass']} passed, {counts['fail']} failed, {counts['skip']} skipped** "
        f"of {len(tests)} test(s), in {elapsed}",
    ]
    if failed:
        lines += ["", "**Failed:**"] + [f"- `{name}`" for name in failed]
    if skipped:
        lines += ["", "**Skipped:**"] + [f"- `{name}`" for name in skipped]
    return "\n".join(lines) + "\n"


def render_full(tests: dict[str, dict], elapsed: str) -> str:
    counts = {"pass": 0, "fail": 0, "skip": 0, None: 0}
    for test in tests.values():
        counts[test["status"]] = counts.get(test["status"], 0) + 1

    lines = [
        "## E2E report (full)",
        "",
        f"**{counts['pass']} passed, {counts['fail']} failed, {counts['skip']} skipped** "
        f"of {len(tests)} test(s), in {elapsed}",
        "",
        "| Test | Status | Elapsed |",
        "|---|---|---|",
    ]
    for name, test in tests.items():
        status = (test["status"] or "?").upper()
        elapsed_cell = f"{test['elapsed']}s" if test["elapsed"] is not None else "-"
        lines.append(f"| `{name}` | {status} | {elapsed_cell} |")

    failed = [name for name, test in tests.items() if test["status"] == "fail"]
    if failed:
        lines += ["", "### Failures"]
        for name in failed:
            output = "".join(tests[name]["output"]).rstrip("\n") or "(no output captured)"
            lines += [
                "",
                f"<details><summary><code>{name}</code></summary>",
                "",
                "```",
                output,
                "```",
                "",
                "</details>",
            ]
    return "\n".join(lines) + "\n"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--events", type=pathlib.Path, required=True, help="go test -json event stream, one JSON object per line")
    ap.add_argument("--level", choices=["summary", "full"], required=True)
    args = ap.parse_args()

    events = load_events(args.events)
    tests = tests_from(events)

    if not tests:
        print(
            "## E2E report\n\n"
            "no test results were parsed — the pack likely failed to build or "
            "start before any test ran; see the job log.\n"
        )
        return 0

    elapsed = wall_elapsed(events)
    renderer = render_summary if args.level == "summary" else render_full
    print(renderer(tests, elapsed))
    return 0


if __name__ == "__main__":
    sys.exit(main())
