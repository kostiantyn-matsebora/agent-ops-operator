"""Loads `workflows.desired.yaml` into the engine's own in-memory shape.

A workflow's shape -- its states, and each state's own transitions -- comes
from this file alone. Nothing here, or anywhere downstream of it, encodes
one workflow's names or transitions as code (see conveyor-engine spec,
"A workflow's shape comes from its own declaration, never from code").
"""
from __future__ import annotations

import dataclasses
from pathlib import Path
from typing import Optional

import yaml

from .paths import default_repo_root

WORKFLOWS_RELATIVE_PATH = Path(".github/conveyor-model/workflows.desired.yaml")


@dataclasses.dataclass(frozen=True)
class Transition:
    event: str
    to: str
    guard: Optional[str] = None
    owned_by: Optional[str] = None


@dataclasses.dataclass(frozen=True)
class State:
    id: str
    initial: bool = False
    final: bool = False
    invokes: Optional[str] = None
    transitions: tuple[Transition, ...] = ()


@dataclasses.dataclass(frozen=True)
class Workflow:
    name: str
    subject: str
    acts_on: str
    label: Optional[str] = None
    states: dict[str, State] = dataclasses.field(default_factory=dict)

    def initial_state(self) -> str:
        for state_id, state in self.states.items():
            if state.initial:
                return state_id
        raise ValueError(f"workflow {self.name!r} declares no initial state")


def workflows_path(repo_root: Optional[Path] = None) -> Path:
    root = repo_root or default_repo_root()
    return root / WORKFLOWS_RELATIVE_PATH


def load_workflows(repo_root: Optional[Path] = None) -> dict[str, Workflow]:
    path = workflows_path(repo_root)
    with path.open() as f:
        raw = yaml.safe_load(f)
    return parse_workflows(raw)


def parse_workflows(raw: dict) -> dict[str, Workflow]:
    """The in-memory shape, built from an already-parsed YAML mapping --
    split out from `load_workflows` so a test can build a workflow from a
    literal dict with no file on disk."""
    workflows: dict[str, Workflow] = {}
    for name, wf_raw in (raw.get("workflows") or {}).items():
        states: dict[str, State] = {}
        for state_id, state_raw in (wf_raw.get("states") or {}).items():
            transitions = tuple(
                Transition(
                    event=t["event"],
                    to=t["to"],
                    guard=t.get("guard"),
                    owned_by=t.get("owned_by"),
                )
                for t in (state_raw.get("transitions") or [])
            )
            states[state_id] = State(
                id=state_id,
                initial=bool(state_raw.get("initial", False)),
                final=bool(state_raw.get("final", False)),
                invokes=state_raw.get("invokes"),
                transitions=transitions,
            )
        workflows[name] = Workflow(
            name=name,
            subject=wf_raw["subject"],
            acts_on=wf_raw["acts_on"],
            label=wf_raw.get("label"),
            states=states,
        )
    return workflows


def all_action_names(workflows: dict[str, Workflow]) -> set[str]:
    """Every `owned_by` action referenced anywhere, across every workflow --
    the set the stub registry (`actions.py`) is built from, so a new action
    in the YAML needs no second place edited."""
    return {
        transition.owned_by
        for workflow in workflows.values()
        for state in workflow.states.values()
        for transition in state.transitions
        if transition.owned_by
    }


def event_to_workflows(workflows: dict[str, Workflow]) -> dict[str, set[str]]:
    """Every event name declared anywhere, mapped to the workflow(s) that
    declare a transition on it -- used by the workflow/label-mapping
    consistency check."""
    result: dict[str, set[str]] = {}
    for name, workflow in workflows.items():
        for state in workflow.states.values():
            for transition in state.transitions:
                result.setdefault(transition.event, set()).add(name)
    return result


def state_to_workflows(workflows: dict[str, Workflow]) -> dict[str, set[str]]:
    """Every state id declared anywhere, mapped to the workflow(s) that
    declare it -- used by the workflow/label-mapping consistency check."""
    result: dict[str, set[str]] = {}
    for name, workflow in workflows.items():
        for state_id in workflow.states:
            result.setdefault(state_id, set()).add(name)
    return result
