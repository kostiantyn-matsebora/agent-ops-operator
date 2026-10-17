#!/usr/bin/env python3
"""`.claude/rules/runtime-spec.md`'s `paths:` frontmatter, checked against the
REAL tracked tree -- this rule's scope is a fact about the real repository,
not a fixture, so (unlike struct-table.test.py) this reads the real rule file
and runs `git ls-files` for real, exactly as `authoring.md` says a scoped
rule must: "every way to get it wrong is preceded by reading a matching
file."

Two things pinned:

1. Every glob the rule declares matches at least one tracked file, and the
   five paths the task names are each matched by SOME glob -- a scope that
   matches nothing is a rule nobody will ever see loaded.
2. The matcher can actually FAIL: a glob typo'd to match nothing is proven
   to match nothing, on a throwaway in-memory copy of the frontmatter. A
   test that can only pass never proved it was testing anything.
"""
from __future__ import annotations

import fnmatch
import pathlib
import re
import subprocess
import unittest

REPO = pathlib.Path(__file__).resolve().parents[2]
RULE_PATH = REPO / ".claude" / "rules" / "runtime-spec.md"

FRONTMATTER_RE = re.compile(r"^---\n(?P<body>.*?)\n---\n", re.DOTALL)


def parse_paths(text: str) -> list[str]:
    """The flat `paths:` list in a rule's YAML frontmatter. No YAML library
    needed for this shape: a header line and one `  - "glob"` per line,
    the same flat shape every scoped rule in this repository uses."""
    m = FRONTMATTER_RE.match(text)
    if not m:
        raise AssertionError(f"no frontmatter block at the top of {text[:40]!r}...")
    body = m.group("body")
    fm_match = re.search(r"^paths:\n((?:\s+-\s*.+\n?)+)", body, re.MULTILINE)
    if not fm_match:
        raise AssertionError("frontmatter has no `paths:` list")
    globs = []
    for line in fm_match.group(1).splitlines():
        item = line.strip()
        if not item.startswith("-"):
            continue
        globs.append(item[1:].strip().strip('"').strip("'"))
    return globs


def tracked_files() -> list[str]:
    out = subprocess.run(
        ["git", "ls-files"], cwd=REPO, capture_output=True, text=True, check=True
    )
    return out.stdout.splitlines()


def matches(globs: list[str], files: list[str]) -> dict[str, list[str]]:
    """glob -> every tracked file it matches."""
    return {g: [f for f in files if fnmatch.fnmatch(f, g)] for g in globs}


class TheRuleFileParses(unittest.TestCase):
    def test_runtime_spec_declares_the_five_expected_globs(self):
        globs = parse_paths(RULE_PATH.read_text())
        self.assertEqual(
            globs,
            [
                "runtimes/**",
                "platform/manager/internal/dispatch/**",
                "platform/manager/internal/runtimepod/**",
                "platform/manager/internal/httpapi/**",
                "docs/contracts/runtime.md",
            ],
        )


class EveryGlobMatchesAtLeastOneTrackedFile(unittest.TestCase):
    def setUp(self):
        self.globs = parse_paths(RULE_PATH.read_text())
        self.files = tracked_files()
        self.hits = matches(self.globs, self.files)

    def test_no_glob_is_vacuous(self):
        for glob, hit in self.hits.items():
            self.assertTrue(hit, f"{glob!r} matches no tracked file")


class TheFiveNamedPathsAreEachReached(unittest.TestCase):
    """The task's own list: a real file under each named path, found by
    SOME glob in the rule -- not just that the glob exists in the abstract."""

    def setUp(self):
        self.globs = parse_paths(RULE_PATH.read_text())
        self.files = tracked_files()

    def assert_reached(self, path: str):
        self.assertIn(path, self.files, f"{path!r} is not a tracked file")
        self.assertTrue(
            any(fnmatch.fnmatch(path, g) for g in self.globs),
            f"{path!r} is matched by none of {self.globs!r}",
        )

    def test_a_runtimes_file(self):
        self.assert_reached("runtimes/claude/runtime.js")

    def test_a_dispatch_package_file(self):
        hits = [f for f in self.files if f.startswith("platform/manager/internal/dispatch/")]
        self.assertTrue(hits, "no tracked file under platform/manager/internal/dispatch/")
        self.assert_reached(hits[0])

    def test_a_runtimepod_package_file(self):
        hits = [f for f in self.files if f.startswith("platform/manager/internal/runtimepod/")]
        self.assertTrue(hits, "no tracked file under platform/manager/internal/runtimepod/")
        self.assert_reached(hits[0])

    def test_a_httpapi_package_file(self):
        hits = [f for f in self.files if f.startswith("platform/manager/internal/httpapi/")]
        self.assertTrue(hits, "no tracked file under platform/manager/internal/httpapi/")
        self.assert_reached(hits[0])

    def test_the_runtime_contract_page_itself(self):
        self.assert_reached("docs/contracts/runtime.md")


class ABrokenGlobIsCaught(unittest.TestCase):
    """Proves the matcher can fail -- never written back to the real rule
    file, only to an in-memory copy of its frontmatter."""

    def test_a_typo_d_directory_matches_nothing(self):
        broken = ["runtimes-typo/**"]
        files = tracked_files()
        hits = matches(broken, files)
        self.assertEqual(hits["runtimes-typo/**"], [])

    def test_the_real_rule_s_dispatch_glob_would_catch_the_same_typo(self):
        """Same assertion, run through the real rule's OWN globs with one
        swapped for a broken one, so the failure mode is the one this test
        class exists to prove: a corrupted copy of the real scope is
        detectably worse than the real scope."""
        real_globs = parse_paths(RULE_PATH.read_text())
        corrupted = [g if g != "runtimes/**" else "runtimes-typo/**" for g in real_globs]
        self.assertIn("runtimes-typo/**", corrupted)
        files = tracked_files()
        hits = matches(corrupted, files)
        self.assertEqual(hits["runtimes-typo/**"], [])
        # Every OTHER glob in the corrupted copy is untouched and still matches.
        for g in corrupted:
            if g != "runtimes-typo/**":
                self.assertTrue(hits[g], f"{g!r} unexpectedly matches nothing")


if __name__ == "__main__":
    unittest.main()
