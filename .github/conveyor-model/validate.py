#!/usr/bin/env python3
"""Cross-check workflows.yaml against station.yaml / loop.yaml.

station IS THE ISSUE'S STATE. loop IS THE PULL REQUEST'S STATE. Neither is a
process -- both are GENERATED (extract_machines.py) plain data, trusted as
ground truth for what conveyor.py's real tables actually say.

workflows.yaml is the only PROCESS declaration: each conveyor:* is its own
complete state machine, written directly in station's or loop's real
vocabulary (never a separate invented one), hand-authored and checked here.

Checks:
  1. Totality: station.yaml / loop.yaml define a transition for every
     (state, event) pair -- the same property conveyor.test.py enforces on
     the Python tables, re-checked here on the generated YAML as a guard
     against a stale hand-run extraction.
  2. Every transition in EVERY conveyor workflow is a REAL (from, event, to)
     tuple in the machine it `acts_on` (station.yaml or loop.yaml) -- every
     one looked up directly, none inferred or assumed.
  3. Every workflow's `states:` list is a subset of its `acts_on` machine's
     real states -- a conveyor cannot claim to occupy a state that machine
     does not have.

Exit code 0 and silence on success; a nonzero exit and one line per problem
otherwise.
"""
from __future__ import annotations

import sys
from pathlib import Path

import yaml

HERE = Path(__file__).resolve().parent


def load(name: str) -> dict:
    return yaml.safe_load((HERE / name).read_text())


def check_totality(machine: dict) -> list[str]:
    problems = []
    states = machine["states"]
    events = sorted({t["event"] for t in machine["transitions"]})
    seen = {(t["from"], t["event"]) for t in machine["transitions"]}
    for s in states:
        for e in events:
            if (s, e) not in seen:
                problems.append(f"{machine['machine']}.yaml: no transition declared for ({s!r}, {e!r}) "
                                 f"-- the real table fills every cell, including SKIP; this file is "
                                 f"missing one, meaning it was hand-edited or extracted stale")
    return problems


def check_workflows(workflows: dict, machines: dict) -> list[str]:
    problems = []
    for name, w in workflows.items():
        machine = machines.get(w["acts_on"])
        if machine is None:
            problems.append(f"workflows.yaml: {name}.acts_on={w['acts_on']!r} is not 'station' or 'loop'")
            continue

        real = {(t["from"], t["event"]): t["to"] for t in machine["transitions"]}
        real_states = set(machine["states"])

        unknown_states = set(w.get("states", [])) - real_states
        if unknown_states:
            problems.append(f"workflows.yaml: {name} claims states {sorted(unknown_states)}, "
                             f"not in {w['acts_on']}.yaml's real states {sorted(real_states)}")

        for t in w.get("transitions", []):
            key = (t["from"], t["event"])
            if key not in real:
                problems.append(f"workflows.yaml: {name} transition ({t['from']!r}, {t['event']!r}) "
                                 f"-- no such transition in {w['acts_on']}.yaml")
                continue
            if real[key] != t["to"]:
                problems.append(f"workflows.yaml: {name} transition ({t['from']!r}, {t['event']!r}) "
                                 f"declares to={t['to']!r}, the real table says {real[key]!r}")
            if t["from"] not in w.get("states", []):
                problems.append(f"workflows.yaml: {name} transition from={t['from']!r} "
                                 f"is not listed in its own states: {w.get('states')}")
            if t["to"] not in w.get("states", []):
                problems.append(f"workflows.yaml: {name} transition to={t['to']!r} "
                                 f"is not listed in its own states: {w.get('states')}")
    return problems


def main() -> int:
    station = load("station.yaml")
    loop = load("loop.yaml")
    workflows = load("workflows.yaml")["workflows"]

    problems: list[str] = []
    problems += check_totality(station)
    problems += check_totality(loop)
    problems += check_workflows(workflows, {"station": station, "loop": loop})

    if problems:
        print(f"{len(problems)} problem(s):")
        for p in problems:
            print(f"  - {p}")
        return 1
    print("all files are consistent.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
