import unittest

from conveyor_engine.guards import (
    GuardSyntaxError,
    UnknownPredicateError,
    evaluate_guard,
)


class EvaluateGuardTest(unittest.TestCase):
    def test_no_guard_is_always_true(self):
        self.assertTrue(evaluate_guard(None, {}))

    def test_single_predicate_true(self):
        registry = {"has_access": lambda: True}
        self.assertTrue(evaluate_guard("has_access", registry))

    def test_single_predicate_false(self):
        registry = {"has_access": lambda: False}
        self.assertFalse(evaluate_guard("has_access", registry))

    def test_multi_clause_true(self):
        registry = {"has_access": lambda: True, "is_session_at_work": lambda: False}
        self.assertTrue(
            evaluate_guard("has_access AND NOT is_session_at_work", registry)
        )

    def test_multi_clause_false(self):
        registry = {"has_access": lambda: True, "is_session_at_work": lambda: True}
        self.assertFalse(
            evaluate_guard("has_access AND NOT is_session_at_work", registry)
        )

    def test_or(self):
        registry = {"a": lambda: False, "b": lambda: True}
        self.assertTrue(evaluate_guard("a OR b", registry))
        self.assertFalse(evaluate_guard("a OR NOT b", registry))

    def test_parentheses(self):
        registry = {"a": lambda: True, "b": lambda: False, "c": lambda: False}
        self.assertTrue(evaluate_guard("a AND (b OR NOT c)", registry))

    def test_unknown_predicate_raises(self):
        with self.assertRaises(UnknownPredicateError):
            evaluate_guard("not_registered_anywhere", {})

    def test_unknown_predicate_inside_expression_raises(self):
        registry = {"has_access": lambda: True}
        with self.assertRaises(UnknownPredicateError):
            evaluate_guard("has_access AND mystery_predicate", registry)

    def test_malformed_expression_raises_syntax_error(self):
        registry = {"a": lambda: True}
        with self.assertRaises(GuardSyntaxError):
            evaluate_guard("a AND", registry)

    def test_never_falls_back_to_eval(self):
        # A string that would be dangerous under a bare eval() must still
        # resolve through the registry lookup alone, and fail the same way
        # any other unknown name does.
        with self.assertRaises(UnknownPredicateError):
            evaluate_guard("__import__", {})


if __name__ == "__main__":
    unittest.main()
