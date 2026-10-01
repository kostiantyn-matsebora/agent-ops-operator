#!/usr/bin/env python3
"""Cross-check the four declarative files for consistency.

station.yaml / loop.yaml are GENERATED (extract_machines.py) and therefore
trusted as ground truth. workflows.yaml and registry.yaml are HAND-AUTHORED,
so this is where drift between "what the docs say an action fires" and
"what the real machine actually accepts" would surface -- the exact failure
mode gotchas.md documents for every hand-copied rule in this repository.

Checks:
  1. Every `fires_station_event` / `fires_loop_event` named in registry.yaml
     is a real event that appears in station.yaml / loop.yaml.
  2. Every `guard:` / `action:` name used in workflows.yaml transitions has
     an entry in registry.yaml (guards: / actions:), OR is the literal
     `none`.
  3. Every workflow named in triggers.yaml exists in workflows.yaml.
  4. Totality: station.yaml / loop.yaml define a transition for every
     (state, event) pair -- the same property conveyor.test.py enforces on
     the Python tables, re-checked here on the generated YAML as a guard
     against a hand-run extraction going stale.

Exit code 0 and silence on success; a nonzero exit and one line per problem
otherwise. Never prints which production data, if any, triggered a problem
-- this operates on the schema files alone.
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
                                 f"-- station_table()/loop_table() in conveyor.py fills every cell, including SKIP; "
                                 f"this file is missing one, which means it was hand-edited or extracted stale")
    return problems


def main() -> int:
    station = load("station.yaml")
    loop = load("loop.yaml")
    workflows = load("workflows.yaml")["workflows"]
    registry = load("registry.yaml")
    triggers = load("triggers.yaml")["triggers"]

    problems: list[str] = []

    problems += check_totality(station)
    problems += check_totality(loop)

    station_events = {t["event"] for t in station["transitions"]}
    loop_events = {t["event"] for t in loop["transitions"]}

    # check 1: registry's claimed emitted events are real
    for section, events_key in (("actions", "fires_station_event"), ("actions", "fires_loop_event")):
        for name, spec in registry.get(section, {}).items():
            ev = spec.get(events_key)
            if ev is None:
                continue
            pool = station_events if events_key == "fires_station_event" else loop_events
            if ev not in pool:
                problems.append(f"registry.yaml: {section}.{name}.{events_key} names {ev!r}, "
                                 f"which does not exist in {'station' if 'station' in events_key else 'loop'}.yaml")

    for fn_name, spec in registry.get("decision_functions", {}).items():
        for ev in spec.get("emits_station_events", []):
            if ev not in station_events:
                problems.append(f"registry.yaml: decision_functions.{fn_name} claims station event {ev!r}, "
                                 f"not found in station.yaml")
        for ev in spec.get("emits_loop_events", []):
            if ev not in loop_events:
                problems.append(f"registry.yaml: decision_functions.{fn_name} claims loop event {ev!r}, "
                                 f"not found in loop.yaml")

    # check 2: every guard/action named in workflows.yaml resolves in registry.yaml
    known_guards = set(registry.get("guards", {}).keys())
    known_actions = set(registry.get("actions", {}).keys()) | {"none"}
    for wf_name, wf in workflows.items():
        for t in wf.get("transitions", []):
            g = t.get("guard")
            if g and g not in known_guards:
                problems.append(f"workflows.yaml: {wf_name} references guard {g!r}, not declared in registry.yaml")
            a = t.get("action")
            if a and a not in known_actions:
                problems.append(f"workflows.yaml: {wf_name} references action {a!r}, not declared in registry.yaml")
        for inv in wf.get("invokes", []):
            target = inv["workflow"]
            if target not in workflows:
                problems.append(f"workflows.yaml: {wf_name} invokes {target!r}, which is not a declared workflow")

    # check 3: every trigger names a real workflow (where it names one at all)
    for trig in triggers:
        wf_name = trig.get("workflow")
        if wf_name and wf_name not in workflows:
            problems.append(f"triggers.yaml: trigger for event {trig['event']!r} names workflow {wf_name!r}, "
                             f"which is not declared in workflows.yaml")

    if problems:
        print(f"{len(problems)} problem(s):")
        for p in problems:
            print(f"  - {p}")
        return 1
    print("all four files are consistent.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
