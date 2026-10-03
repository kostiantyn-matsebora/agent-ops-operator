"""The guard registry and its boolean-expression evaluator.

A guard string is a boolean expression over bare `snake_case` predicate
names, combined with `AND`, `OR`, `NOT` and optional parentheses -- e.g.
`"has_access AND NOT is_session_at_work"`. Each name is looked up, at
evaluation time, in a registry of zero-argument predicates
(`dict[str, Callable[[], bool]]`); a name with no entry RAISES rather than
falling back to anything, including Python's own `eval()` (see design.md,
Decision 2 -- an untrusted or mistyped name must refuse outright, not
silently accept arbitrary syntax).

The registry itself holds no subject or event context: each predicate
function is expected to close over whatever live state it needs. Building
that closure -- which subject, which facts -- is the responsibility of
whoever constructs the registry for one `evaluate()` call, not of this
module.
"""
from __future__ import annotations

import re
from typing import Callable, Optional

GuardRegistry = dict[str, Callable[[], bool]]

_TOKEN_RE = re.compile(r"\(|\)|AND|OR|NOT|[A-Za-z_][A-Za-z0-9_]*")


class GuardError(Exception):
    """Base class for a guard expression that cannot be evaluated."""


class UnknownPredicateError(GuardError):
    def __init__(self, name: str):
        super().__init__(f"no predicate named {name!r} is registered")
        self.name = name


class GuardSyntaxError(GuardError):
    pass


def evaluate_guard(guard: Optional[str], registry: GuardRegistry) -> bool:
    """A transition with no guard (`guard is None`) is always permitted,
    once its event is recognized."""
    if guard is None:
        return True
    tokens = _TOKEN_RE.findall(guard)
    if not tokens:
        raise GuardSyntaxError(f"empty guard expression: {guard!r}")
    parser = _GuardParser(tokens, registry)
    result = parser.parse_expr()
    if parser.pos != len(tokens):
        raise GuardSyntaxError(
            f"unexpected token {tokens[parser.pos]!r} in guard {guard!r}"
        )
    return result


class _GuardParser:
    """Recursive-descent over `NOT` > `AND` > `OR`, matching the ordinary
    boolean precedence the YAML's own guard strings are written against."""

    def __init__(self, tokens: list[str], registry: GuardRegistry):
        self.tokens = tokens
        self.registry = registry
        self.pos = 0

    def _peek(self) -> Optional[str]:
        return self.tokens[self.pos] if self.pos < len(self.tokens) else None

    def _advance(self) -> str:
        token = self.tokens[self.pos]
        self.pos += 1
        return token

    def parse_expr(self) -> bool:
        return self._parse_or()

    def _parse_or(self) -> bool:
        result = self._parse_and()
        while self._peek() == "OR":
            self._advance()
            result = self._parse_and() or result
        return result

    def _parse_and(self) -> bool:
        result = self._parse_not()
        while self._peek() == "AND":
            self._advance()
            result = self._parse_not() and result
        return result

    def _parse_not(self) -> bool:
        if self._peek() == "NOT":
            self._advance()
            return not self._parse_not()
        return self._parse_primary()

    def _parse_primary(self) -> bool:
        token = self._peek()
        if token is None:
            raise GuardSyntaxError("guard expression ended unexpectedly")
        if token == "(":
            self._advance()
            result = self.parse_expr()
            if self._peek() != ")":
                raise GuardSyntaxError("missing closing parenthesis in guard")
            self._advance()
            return result
        if token in ("AND", "OR", ")"):
            raise GuardSyntaxError(f"unexpected token {token!r} in guard")
        self._advance()
        if token not in self.registry:
            raise UnknownPredicateError(token)
        return bool(self.registry[token]())
