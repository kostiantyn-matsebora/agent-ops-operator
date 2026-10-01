#!/usr/bin/env python3
"""Check workflows.yaml's own internal consistency.

workflows.yaml is now the ONLY source -- no station.yaml / loop.yaml to
cross-check against. Those existed only to verify a hand-typed YAML against
conveyor.py's hardcoded tables, a temporary need for the one-time migration
away from conveyor.py. Once an engine loads and runs workflows.yaml
directly, there is nothing left to compare it against: the declaration IS
the behaviour.

Checks:
  1. Every transition's `from` and `to` are in that workflow's own
     `states:` list -- a workflow cannot move through a state it never
     declared.
  2. Every workflow's `acts_on` names either a real subject-machine concept
     (`station`, `loop`) or another declared workflow.
  3. No duplicate (from, event) pairs claim two different `to` targets
     within one workflow -- that would make the machine non-deterministic
     for the same trigger.
  4. Every `invokes:` entry maps a state this workflow actually declares to
     a workflow that actually exists -- an invocation naming either wrong
     is exactly the kind of prose-pretending-to-be-structure this format
     exists to make impossible.

Also checks workflows.desired.yaml (target designs, not yet implemented)
against itself AND against workflows.yaml, since a desired workflow's
`invokes:` commonly names an already-verified real workflow (e.g.
`propose` invokes `loop`) -- that reference has to resolve too.
"""
from __future__ import annotations

import sys
from pathlib import Path

import yaml

HERE = Path(__file__).resolve().parent
KNOWN_SUBJECTS = {"station", "loop"}


def load(name: str) -> dict:
    return yaml.safe_load((HERE / name).read_text())


def check_workflow(name: str, w: dict, all_workflows: dict) -> list[str]:
    problems = []
    states = set(w.get("states", []))

    acts_on = w.get("acts_on")
    if acts_on not in KNOWN_SUBJECTS and acts_on not in all_workflows:
        problems.append(f"{name}: acts_on={acts_on!r} is not a known subject "
                         f"({sorted(KNOWN_SUBJECTS)}) or another declared workflow")

    seen: dict[tuple[str, str], str] = {}
    for t in w.get("transitions", []):
        if t["from"] not in states:
            problems.append(f"{name}: transition from={t['from']!r} not in its own states: {sorted(states)}")
        if t["to"] not in states:
            problems.append(f"{name}: transition to={t['to']!r} not in its own states: {sorted(states)}")
        key = (t["from"], t["event"])
        if key in seen and seen[key] != t["to"]:
            problems.append(f"{name}: ({t['from']!r}, {t['event']!r}) declared to both "
                             f"{seen[key]!r} and {t['to']!r} -- non-deterministic")
        seen[key] = t["to"]

    for state, dep_name in w.get("invokes", {}).items():
        if state not in states:
            problems.append(f"{name}: invokes.{state} -- {state!r} is not in this workflow's own states")
        if dep_name not in all_workflows:
            problems.append(f"{name}: invokes.{state}: {dep_name!r} -- no such workflow declared")

    return problems


def main() -> int:
    workflows = load("workflows.yaml")["workflows"]

    problems: list[str] = []
    for name, w in workflows.items():
        problems += check_workflow(name, w, workflows)

    desired_path = HERE / "workflows.desired.yaml"
    if desired_path.exists():
        desired = load("workflows.desired.yaml")["workflows"]
        # a desired workflow may invoke either another desired one or an
        # already-verified real one -- both pools are visible to it.
        combined = {**workflows, **desired}
        for name, w in desired.items():
            problems += check_workflow(f"[DESIRED] {name}", w, combined)

    if problems:
        print(f"{len(problems)} problem(s):")
        for p in problems:
            print(f"  - {p}")
        return 1
    print("workflows.yaml and workflows.desired.yaml are internally consistent.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
