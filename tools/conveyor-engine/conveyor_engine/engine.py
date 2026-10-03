"""The one entry point wiring the four other layers together.

`Engine.evaluate` reads a workflow's current state from the subject's own
labels, matches a real event to that workflow's declared transitions
(`triggers.py`), evaluates each candidate's guard (`guards.py`), writes the
winning transition's next state with whatever propagation its label's
prefix declares (`state_writer.py`), and only then calls its `owned_by`
stub (`actions.py`) -- in that order, never the stub before the state
write (conveyor-engine spec, "An action is a registered stub").
"""
from __future__ import annotations

import dataclasses
import sys
from typing import Optional

from .actions import ActionRegistry
from .github_client import GitHubClient, Subject
from .guards import GuardRegistry, evaluate_guard
from .labels import LabelMapping
from .loader import Workflow
from .state_writer import StateWriter
from .triggers import match_transitions


def _log_error(message: str) -> None:
    print(f"::error::conveyor-engine {message}", file=sys.stderr)


@dataclasses.dataclass
class EvaluationResult:
    moved: bool
    from_state: Optional[str] = None
    to_state: Optional[str] = None
    action_called: Optional[str] = None
    error: Optional[str] = None


class Engine:
    """Holds the five wired pieces for one or more `evaluate()` calls.

    `guard_registry` is a plain `dict[str, Callable[[], bool]]` (see
    `guards.py`). This section treats it as given: whoever builds real
    guard predicates is responsible for closing each one over the live
    subject and facts a given call needs, and for rebuilding (or
    replacing) `engine.guard_registry` before a call that needs different
    context. The engine itself never inspects a guard's internals, and
    never passes `subject` or `facts` into a guard call -- a registered
    predicate takes none (conveyor-engine spec, "A guard is a named
    predicate, evaluated against live state").
    """

    def __init__(
        self,
        workflows: dict[str, Workflow],
        label_mapping: LabelMapping,
        guard_registry: GuardRegistry,
        action_registry: ActionRegistry,
        client: GitHubClient,
    ):
        self.workflows = workflows
        self.label_mapping = label_mapping
        self.guard_registry = guard_registry
        self.action_registry = action_registry
        self.client = client
        self.state_writer = StateWriter(label_mapping, client)

    def evaluate(
        self,
        workflow_name: str,
        subject: Subject,
        event_name: str,
        facts: Optional[dict] = None,
    ) -> EvaluationResult:
        facts = facts or {}

        workflow = self.workflows.get(workflow_name)
        if workflow is None:
            error = f"no such workflow {workflow_name!r}"
            _log_error(error)
            return EvaluationResult(moved=False, error=error)

        current_state, state_error = self._current_state(workflow, subject)
        if current_state is None:
            error = state_error or (
                f"{subject} carries no recognizable {workflow_name!r} state"
            )
            _log_error(error)
            return EvaluationResult(moved=False, error=error)

        candidates = match_transitions(workflow, current_state, event_name)
        if not candidates:
            return EvaluationResult(moved=False, from_state=current_state)

        satisfied = [
            t for t in candidates if evaluate_guard(t.guard, self.guard_registry)
        ]

        if len(satisfied) > 1:
            names = ", ".join(f"{current_state} -> {t.to}" for t in satisfied)
            error = (
                f"two or more transitions satisfied for {workflow_name!r} "
                f"on event {event_name!r} from {current_state!r}: {names}"
            )
            _log_error(error)
            return EvaluationResult(moved=False, from_state=current_state, error=error)

        if not satisfied:
            return EvaluationResult(moved=False, from_state=current_state)

        transition = satisfied[0]
        vocabulary = set(workflow.states.keys())
        wrote = self.state_writer.write_state(subject, vocabulary, transition.to)
        if not wrote:
            error = f"failed to write state {transition.to!r} on {subject}"
            _log_error(error)
            return EvaluationResult(moved=False, from_state=current_state, error=error)

        action_called = None
        if transition.owned_by:
            action = self.action_registry.get(transition.owned_by)
            if action is None:
                error = f"no stub registered for action {transition.owned_by!r}"
                _log_error(error)
                return EvaluationResult(
                    moved=True,
                    from_state=current_state,
                    to_state=transition.to,
                    error=error,
                )
            else:
                action(workflow_name, subject, facts)
                action_called = transition.owned_by

        return EvaluationResult(
            moved=True,
            from_state=current_state,
            to_state=transition.to,
            action_called=action_called,
        )

    def _current_state(
        self, workflow: Workflow, subject: Subject
    ) -> tuple[Optional[str], Optional[str]]:
        """The subject's state, or None plus the specific reason it has none
        (an unreadable label set, or several state labels at once)."""
        try:
            labels = self.client.get_labels(subject)
        except Exception as exc:  # noqa: BLE001
            return None, f"failed to read labels on {subject}: {exc}"

        matches = labels & set(workflow.states.keys())
        if len(matches) > 1:
            return None, (
                f"{subject} carries more than one {workflow.name!r} state "
                f"label: {sorted(matches)}"
            )
        return next(iter(matches), None), None
