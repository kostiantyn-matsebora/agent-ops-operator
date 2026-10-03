"""Writes a workflow's state as a label, including whatever propagation
the label's own prefix declares (design.md Decisions 4, 4a, 4b).

A write never raises past its caller: a failure here is logged, and the
subject's line waits for the next real trigger to re-derive the state from
whatever label is actually there -- not a crashed job (Decision 5).
"""
from __future__ import annotations

import sys
from typing import Iterable

from .github_client import GitHubClient, Subject
from .labels import LabelMapping


def _log_error(message: str) -> None:
    print(f"::error::conveyor-engine {message}", file=sys.stderr)


class StateWriter:
    def __init__(self, label_mapping: LabelMapping, client: GitHubClient):
        self.label_mapping = label_mapping
        self.client = client

    def write_state(
        self, subject: Subject, state_vocabulary: Iterable[str], new_state: str
    ) -> bool:
        """Add `new_state` to `subject` and remove every other label in
        `state_vocabulary` (the calling workflow's own declared states),
        in one logical write, then propagate per `new_state`'s own prefix
        rule.

        Returns whether the LOCAL write succeeded. A propagation failure is
        logged but does not flip this to False -- the caller's own subject
        was written either way, and propagation re-derives from the live
        label on its own next trigger. Never raises.
        """
        state_vocabulary = list(state_vocabulary)  # read twice below
        try:
            self._write_local(subject, state_vocabulary, new_state)
        except Exception as exc:  # noqa: BLE001 -- the whole point: never raise past here
            _log_error(f"failed to write {new_state!r} on {subject}: {exc}")
            return False

        try:
            self.propagate_label(subject, new_state, state_vocabulary)
        except Exception as exc:  # noqa: BLE001
            _log_error(f"failed to propagate {new_state!r} from {subject}: {exc}")

        return True

    def _write_local(
        self, subject: Subject, state_vocabulary: Iterable[str], new_state: str
    ) -> None:
        vocabulary = set(state_vocabulary)
        current = self.client.get_labels(subject)
        desired = (current - vocabulary) | {new_state}
        self.client.set_labels(subject, desired)

    def propagate_label(
        self,
        subject: Subject,
        label: str,
        state_vocabulary: Iterable[str] = (),
    ) -> None:
        """The reusable half of Decision 4b: write `label` onto every
        subject related to `subject`, crossing the issue/pull-request
        boundary, when and only when the label's own prefix is declared
        `bidirectional`.

        The prefix is read directly off the label string (every label here
        is `<prefix>:<rest>`, per `labels.yaml`'s own convention), so this
        works identically for a state label and for a trigger label --
        `write_state` is one caller of it, not the only legitimate one.
        """
        prefix_name = label.split(":", 1)[0]
        prefix = self.label_mapping.prefixes.get(prefix_name)
        if prefix is None or prefix.propagation != "bidirectional":
            return

        # Same-prefix labels in the caller's vocabulary are stale on the target.
        stale = {
            v for v in state_vocabulary if v.split(":", 1)[0] == prefix_name
        } - {label}

        if subject.kind == "issue":
            targets = list(self.client.related_pull_requests(subject))
        else:
            issue = self.client.related_issue(subject)
            targets = [issue] if issue is not None else []

        # One target failing must not abort the rest.
        for target in targets:
            try:
                self._set_on(target, label, stale)
            except Exception as exc:  # noqa: BLE001
                _log_error(f"failed to propagate {label!r} to {target}: {exc}")

    def _set_on(self, target: Subject, label: str, stale: set) -> None:
        current = self.client.get_labels(target)
        desired = (current - stale) | {label}
        if desired != current:
            self.client.set_labels(target, desired)
