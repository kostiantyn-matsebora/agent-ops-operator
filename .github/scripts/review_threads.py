"""GitHub's review-thread GraphQL, the two bits every reader of one needs.

Three programs read or write a pull request's review threads --
`accepted-findings.py` (what a person accepted), `review-not-clean.py`
(whether the review's own threads are still open) and
`resolve-review-threads.py` (resolving the review's own) -- and each carried
its own copy of the one GraphQL call and the one login normalisation. Only
the QUERY BODY ever differed between them (which fields a reader needs), so
that stays local to each caller; the call itself and the identity fix do not.
"""
from __future__ import annotations

import json
import subprocess


def gh_graphql(query: str, **variables) -> dict:
    """One GraphQL call, `gh`'s own way of making one: an int variable typed `-F`
    (GraphQL's `Int`), everything else `-f` (a `String`). Raises
    `subprocess.CalledProcessError` on a failed call and RuntimeError on a
    response carrying `errors`."""
    cmd = ["gh", "api", "graphql", "-f", f"query={query}"]
    for key, value in variables.items():
        if isinstance(value, bool):
            # `-F` turns the literal `true`/`false` into a GraphQL Boolean;
            # Python's `True` would be sent as the string "True".
            cmd += ["-F", f"{key}={str(value).lower()}"]
            continue
        flag = "-F" if isinstance(value, int) else "-f"
        cmd += [flag, f"{key}={value}"]
    out = subprocess.run(cmd, capture_output=True, text=True, check=True).stdout
    payload = json.loads(out)
    if "errors" in payload:
        raise RuntimeError(payload["errors"])
    return payload["data"]


def normalise_login(login: str | None) -> str:
    """One spelling for a bot, whichever API produced it.

    REST reports `claude[bot]`; GraphQL reports `claude` and marks the account
    `__typename: Bot`. The allowlist is written the REST way, because that is
    how a person reads a login on GitHub -- so both sides are normalised here
    rather than one of them being rewritten to match the other.

    THIS IS WHY A THREAD WAS NEVER RESOLVED, once: the comparison was `claude`
    (GraphQL) against `claude[bot]` (config), so the review refused its own
    threads on every run -- reported honestly by that program's own
    diagnostic, and invisible until a review actually had a finding to close.
    Every reader of a thread's author shares this one normalisation now, so
    that bug cannot recur in a fourth copy of it."""
    return (login or "").strip().lower().removesuffix("[bot]")
