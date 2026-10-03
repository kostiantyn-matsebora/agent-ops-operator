"""Matches a real event against a workflow's own declared transitions.

Matching is a direct lookup: the workflow, the subject's current state, and
the event's own name -- never inferred, and never a function of any other
workflow's declaration (see conveyor-engine spec, "A trigger is a real
event, matched to a workflow's own vocabulary").
"""
from __future__ import annotations

from .loader import Transition, Workflow


def match_transitions(
    workflow: Workflow, current_state: str, event_name: str
) -> list[Transition]:
    """Every transition declared on `current_state` whose `event` equals
    `event_name` -- zero, one, or (when two transitions on one state share
    one event, gated by different guards) more than one.

    This function only surfaces the candidates. Guard evaluation decides
    which one, if any, actually fires.
    """
    state = workflow.states.get(current_state)
    if state is None:
        return []
    return [t for t in state.transitions if t.event == event_name]
