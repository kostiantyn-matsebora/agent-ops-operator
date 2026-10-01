#!/usr/bin/env python3
"""Check workflows.yaml's own internal consistency.

workflows.yaml is now the ONLY source -- no station.yaml / loop.yaml to
cross-check against. Those existed only to verify a hand-typed YAML against
conveyor.py's hardcoded tables, a temporary need for the one-time migration
away from conveyor.py. Once an engine loads and runs workflows.yaml
directly, there is nothing left to compare it against: the declaration IS
the behaviour.

STATES OWN THEIR TRANSITIONS. `states:` is a MAPPING keyed by state id, each
holding its own `initial`/`final` flag and its own `transitions:` list
(event, to, guard, owned_by) -- not a flat `transitions: [{from, ...}]` list
with a separate `states: [...]` name array. A state's own behavior lives on
the state.

Checks:
  1. Every transition's `to` names a state this workflow actually declares
     -- a workflow cannot move through a state it never declared.
  2. Every workflow's `acts_on` names either a real subject-machine concept
     (`station`, `loop`) or another declared workflow.
  3. No duplicate (state, event) pairs claim two different `to` targets --
     that would make the machine non-deterministic for the same trigger.
  4. At most one state per workflow is marked `initial: true` -- UML's
     initial pseudostate means exactly one unconditional entry point. A
     workflow with none is fine (several real states are independent entry
     points, each gated by its own transitions).
  5. Every `invokes:` value on a state names a workflow that actually
     exists -- an invocation naming the wrong one is exactly the kind of
     prose-pretending-to-be-structure this format exists to make
     impossible.

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
    states: dict = w.get("states", {})
    state_ids = set(states.keys())

    acts_on = w.get("acts_on")
    if acts_on not in KNOWN_SUBJECTS and acts_on not in all_workflows:
        problems.append(f"{name}: acts_on={acts_on!r} is not a known subject "
                         f"({sorted(KNOWN_SUBJECTS)}) or another declared workflow")

    initial_states = [sid for sid, s in states.items() if s.get("initial")]
    if len(initial_states) > 1:
        problems.append(f"{name}: more than one state marked initial: true -- {initial_states}")

    for sid, s in states.items():
        seen: dict[str, str] = {}
        for t in s.get("transitions", []):
            if t["to"] not in state_ids:
                problems.append(f"{name}.{sid}: transition to={t['to']!r} not in this workflow's own states: {sorted(state_ids)}")
            if t["event"] in seen and seen[t["event"]] != t["to"]:
                problems.append(f"{name}.{sid}: event {t['event']!r} declared to both "
                                 f"{seen[t['event']]!r} and {t['to']!r} -- non-deterministic")
            seen[t["event"]] = t["to"]

        invoked = s.get("invokes")
        if invoked and invoked not in all_workflows:
            problems.append(f"{name}.{sid}: invokes {invoked!r} -- no such workflow declared")

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
