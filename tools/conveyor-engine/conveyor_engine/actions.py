"""The `owned_by` action registry.

Every action a transition names SHALL exist in a registry of named
functions, and calling one records what it would have done without
performing any real side effect -- no GitHub write, no label placement
beyond the engine's own state write (conveyor-engine spec, "An action is a
registered stub"; design.md Decision 6).

`build_action_registry` discovers which action names exist by scanning the
loaded workflows -- nothing here hardcodes a name, so a new `owned_by` in
the YAML gets a stub with no file to edit.
"""
from __future__ import annotations

from typing import Callable, Optional

from .github_client import GitHubClient, Subject
from .loader import Workflow, all_action_names

ActionCall = Callable[[str, Subject, dict], None]
ActionRegistry = dict[str, ActionCall]


def _make_stub(action_name: str) -> ActionCall:
    def stub(workflow_name: str, subject: Subject, facts: dict) -> None:
        print(
            "::notice::conveyor-engine stub "
            f"action={action_name} workflow={workflow_name} "
            f"subject={subject} facts={facts!r}"
        )

    stub.__name__ = f"stub_{action_name}"
    return stub


def build_action_registry(
    workflows: dict[str, Workflow], client: Optional[GitHubClient] = None
) -> ActionRegistry:
    """One stub per distinct `owned_by` name referenced anywhere in
    `workflows`.

    `client` is accepted only for shape-compatibility with a future
    registry whose actions have real effects -- every stub here ignores
    it completely. This change's whole point is that none of them may
    touch it, whether or not one happens to be available at construction
    time.
    """
    del client  # deliberately unused -- see docstring
    return {name: _make_stub(name) for name in all_action_names(workflows)}
