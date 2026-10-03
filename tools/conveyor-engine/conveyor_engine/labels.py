"""Loads `labels.yaml` -- the real label string, invocation mechanism and
propagation rule behind every workflow's abstract states and triggers.

`workflows.desired.yaml` never names a real label string beyond what a
state's or trigger's own id already is; this file is the one place that
says which prefix a label belongs to and how that prefix propagates (see
conveyor-engine spec, "A label's real name and propagation come from a
declared mapping, never from code").
"""
from __future__ import annotations

import dataclasses
from pathlib import Path
from typing import Optional

import yaml

from .paths import default_repo_root

LABELS_RELATIVE_PATH = Path(".github/conveyor-model/labels.yaml")


@dataclasses.dataclass(frozen=True)
class Prefix:
    name: str
    kind: str
    native_subject: str
    propagation: str
    set_manually_on: tuple[str, ...] = ()


@dataclasses.dataclass(frozen=True)
class Trigger:
    name: str
    label: Optional[str]
    invocation: str
    prefix: Optional[str]
    workflows: tuple[str, ...] = ()


@dataclasses.dataclass(frozen=True)
class StateMapping:
    name: str
    workflows: tuple[str, ...]
    prefix: str


@dataclasses.dataclass(frozen=True)
class LabelMapping:
    prefixes: dict[str, Prefix]
    triggers: dict[str, Trigger]
    states: dict[str, StateMapping]


def labels_path(repo_root: Optional[Path] = None) -> Path:
    root = repo_root or default_repo_root()
    return root / LABELS_RELATIVE_PATH


def load_label_mapping(repo_root: Optional[Path] = None) -> LabelMapping:
    path = labels_path(repo_root)
    with path.open() as f:
        raw = yaml.safe_load(f)
    return parse_label_mapping(raw)


def parse_label_mapping(raw: dict) -> LabelMapping:
    """The in-memory shape, built from an already-parsed YAML mapping --
    split out from `load_label_mapping` so a test can build one from a
    literal dict with no file on disk."""
    prefixes = {
        name: Prefix(
            name=name,
            kind=p["kind"],
            native_subject=p["native_subject"],
            propagation=p["propagation"],
            set_manually_on=tuple(p.get("set_manually_on") or []),
        )
        for name, p in (raw.get("prefixes") or {}).items()
    }
    triggers = {
        name: Trigger(
            name=name,
            label=t.get("label"),
            invocation=t["invocation"],
            prefix=t.get("prefix"),
            workflows=tuple(t.get("workflows") or []),
        )
        for name, t in (raw.get("triggers") or {}).items()
    }
    states = {
        name: StateMapping(
            name=name,
            workflows=tuple(s.get("workflows") or []),
            prefix=s["prefix"],
        )
        for name, s in (raw.get("states") or {}).items()
    }
    return LabelMapping(prefixes=prefixes, triggers=triggers, states=states)
