#!/usr/bin/env python3
"""coverage-diff.py, testing-report.py and merge-sync.py, with no network and no real repository."""
from __future__ import annotations

import importlib.util
import io
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest
from contextlib import redirect_stdout

SCRIPTS = pathlib.Path(__file__).resolve().parents[1] / "scripts"
sys.path.insert(0, str(SCRIPTS))
import conveyor  # noqa: E402

V = json.loads((SCRIPTS.parent / "review-triage.json").read_text())


def load(name):
    spec = importlib.util.spec_from_file_location(name.replace("-", "_"), SCRIPTS / f"{name}.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


cd = load("coverage-diff")
tr = load("testing-report")
ms = load("merge-sync")
ts = load("testing-station")

DIFF = """diff --git a/platform/manager/internal/x/x.go b/platform/manager/internal/x/x.go
--- a/platform/manager/internal/x/x.go
+++ b/platform/manager/internal/x/x.go
@@ -1,0 +2,4 @@
+a
+b
+c
+d
@@ -20 +30 @@
-old
+new
diff --git a/platform/console/ui/src/a.tsx b/platform/console/ui/src/a.tsx
--- a/platform/console/ui/src/a.tsx
+++ b/platform/console/ui/src/a.tsx
@@ -0,0 +1,2 @@
+x
+y
diff --git a/README.md b/README.md
--- a/README.md
+++ b/README.md
@@ -1 +1 @@
-a
+b
diff --git a/old.go b/old.go
--- a/old.go
+++ /dev/null
@@ -1,3 +0,0 @@
-a
"""

GOPROF = """mode: atomic
github.com/o/r/platform/manager/internal/x/x.go:2.1,3.5 2 1
github.com/o/r/platform/manager/internal/x/x.go:4.1,5.5 2 0
github.com/o/r/platform/manager/internal/x/x.go:30.1,30.9 1 1
"""


class TheDiff(unittest.TestCase):
    def test_added_lines_per_file_and_deletions_and_deleted_files_are_ignored(self):
        d = cd.parse_diff(DIFF)
        self.assertEqual({2, 3, 4, 5, 30}, d["platform/manager/internal/x/x.go"])
        self.assertEqual({1, 2}, d["platform/console/ui/src/a.tsx"])
        self.assertNotIn("old.go", d)

    def test_a_single_line_hunk_has_an_implied_count_of_one(self):
        self.assertEqual({30}, cd.parse_diff("+++ b/f.go\n@@ -20 +30 @@\n")["f.go"])


class TheParsers(unittest.TestCase):
    def test_go_blocks_cover_every_line_they_span_and_any_run_block_covers_a_line(self):
        g = cd.parse_go(GOPROF)["github.com/o/r/platform/manager/internal/x/x.go"]
        self.assertEqual({2: True, 3: True, 4: False, 5: False, 30: True}, g)
        g2 = cd.parse_go("mode: set\nf.go:1.1,2.2 1 0\nf.go:2.1,3.2 1 1\n")["f.go"]
        self.assertTrue(g2[2])

    def test_lcov(self):
        r = cd.parse_lcov("SF:ui/src/a.tsx\nDA:1,3\nDA:2,0\nend_of_record\n")
        self.assertEqual({1: True, 2: False}, r["ui/src/a.tsx"])

    def test_cobertura(self):
        xml = '<coverage><packages><package><classes><class filename="a/b.py"><lines>' \
              '<line number="1" hits="2"/><line number="2" hits="0"/></lines></class></classes></package></packages></coverage>'
        self.assertEqual({1: True, 2: False}, cd.parse_cobertura(xml)["a/b.py"])


class TheMeasure(unittest.TestCase):
    T = V["testing"]

    def run_tier(self, tier, records, maps=()):
        return cd.measure(cd.parse_diff(DIFF), records, self.T["domains"][tier], self.T["excluded_from_coverage"], list(maps))

    def test_only_executable_changed_lines_in_the_domain_count(self):
        r = self.run_tier("unit", [cd.parse_go(GOPROF)])
        self.assertEqual((3, 7), (r["covered"], r["total"]))   # 5 measured lines in x.go, 2 unmeasured in a.tsx
        self.assertEqual({"platform/manager/internal/x/x.go": [4, 5]}, {k: v for k, v in r["files"].items() if "manager" in k})

    def test_a_changed_file_no_input_mentions_counts_as_uncovered(self):
        r = self.run_tier("unit", [cd.parse_go(GOPROF)])
        self.assertIn("platform/console/ui/src/a.tsx", r["unmeasured"])
        self.assertEqual([1, 2], r["files"]["platform/console/ui/src/a.tsx"])

    def test_a_file_outside_the_tiers_domain_is_not_its_to_measure(self):
        r = self.run_tier("system", [cd.parse_go(GOPROF)])
        self.assertEqual(1, r["domain_files"])
        self.assertEqual((3, 5), (r["covered"], r["total"]))

    def test_code_a_tier_cannot_reach_is_excluded_from_that_tier_only(self):
        # The system pack runs the NATS backend, and the conformance harness is run by the unit tier.
        path = "platform/manager/internal/state/conformance/suite.go"
        for tier, applies in (("system", True), ("unit", False)):
            excluded = self.T["excluded_from_coverage"] + self.T.get("excluded_from_tier", {}).get(tier, [])
            r = cd.measure({path: {1, 2}}, [{path: {1: False, 2: False}}], self.T["domains"][tier], excluded, [])
            self.assertEqual(0 if applies else 2, r["total"], tier)

    def test_a_tier_from_a_partial_rerun_cannot_be_passed(self):
        import importlib.util
        spec = importlib.util.spec_from_file_location("tr", SCRIPTS / "testing-report.py")
        tr = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(tr)
        covs = {t: {"covered": 9, "total": 10, "collected": True, "domain_files": 1} for t in conveyor.TIERS}
        outcomes = {t: "passed" for t in conveyor.TIERS}
        full = tr.judge(V, covs, outcomes)
        self.assertEqual("pass", {v["tier"]: v["status"] for v in full["verdicts"]}["system"])
        partial = tr.judge(V, covs, outcomes, partial=("system",))
        self.assertEqual("not_run", {v["tier"]: v["status"] for v in partial["verdicts"]}["system"])
        self.assertTrue(any(w["tier"] == "system" for w in partial["work"]))
        with self.assertRaises(ValueError):
            tr.judge(V, covs, outcomes, partial=("nope",))

    def test_a_backend_only_change_has_no_e2e_domain(self):
        d = {"platform/manager/internal/x/x.go": {1, 2}}
        r = cd.measure(d, [], self.T["domains"]["e2e"], self.T["excluded_from_coverage"], [])
        self.assertEqual((0, 0), (r["domain_files"], r["total"]))

    def test_inputs_that_hold_no_record_are_not_collected(self):
        self.assertFalse(self.run_tier("system", [{}])["collected"])
        self.assertFalse(self.run_tier("system", [])["collected"])
        self.assertTrue(self.run_tier("system", [cd.parse_go(GOPROF)])["collected"])

    def test_test_files_and_generated_files_are_never_measured(self):
        d = {"a/b_test.go": {1}, "a/zz_generated.deepcopy.go": {1}, "a/b.go": {1}}
        r = cd.measure(d, [{"a/b.go": {1: True}}], self.T["domains"]["unit"], self.T["excluded_from_coverage"], [])
        self.assertEqual((1, 1, 1), (r["covered"], r["total"], r["domain_files"]))

    def test_an_explicit_map_resolves_a_module_path_that_names_no_directory(self):
        prof = cd.parse_go("mode: set\nagentops/manager/internal/x/x.go:2.1,3.5 2 1\n")
        r = self.run_tier("system", [prof], maps=[("agentops/manager", "platform/manager")])
        self.assertEqual([], r["unmeasured"])

    def test_two_inputs_union_their_coverage(self):
        a = {"f.go": {1: False, 2: True}}
        b = {"f.go": {1: True}}
        r = cd.measure({"f.go": {1, 2}}, [a, b], ["**/*.go"], [], [])
        self.assertEqual((2, 2), (r["covered"], r["total"]))

    def test_a_prefix_anchors_a_report_written_relative_to_a_directory(self):
        with tempfile.TemporaryDirectory() as d:
            lcov = pathlib.Path(d, "l.info"); lcov.write_text("SF:src/a.tsx\nDA:1,1\nDA:2,0\nend_of_record\n")
            diff = pathlib.Path(d, "d.diff"); diff.write_text(DIFF)
            out = io.StringIO()
            with redirect_stdout(out):
                cd.main(["--tier", "e2e", "--lcov", f"{lcov}@platform/console/ui", "--diff-file", str(diff)])
        j = json.loads(out.getvalue())
        self.assertEqual((1, 2, []), (j["covered"], j["total"], j["unmeasured"]))

    def test_every_go_module_in_the_tree_maps_its_import_path_to_its_directory(self):
        with tempfile.TemporaryDirectory() as d:
            root = pathlib.Path(d)
            (root / "a/b").mkdir(parents=True)
            (root / "a/b/go.mod").write_text("module example.test/x/a/b\n\ngo 1.22\n")
            (root / "go.mod").write_text("module example.test/x\n")
            maps = dict(cd.module_maps(root))
        self.assertEqual({"example.test/x/a/b": "a/b", "example.test/x": ""}, maps)

    def test_the_command_line_reads_files_and_prints_json(self):
        with tempfile.TemporaryDirectory() as d:
            diff, prof = pathlib.Path(d, "d.diff"), pathlib.Path(d, "p.out")
            diff.write_text(DIFF); prof.write_text(GOPROF)
            out = io.StringIO()
            with redirect_stdout(out):
                rc = cd.main(["--tier", "system", "--go", str(prof), "--diff-file", str(diff)])
        j = json.loads(out.getvalue())
        self.assertEqual((0, "system", True), (rc, j["tier"], j["collected"]))

    def test_a_missing_input_file_is_an_empty_record_not_a_crash(self):
        with tempfile.TemporaryDirectory() as d:
            diff = pathlib.Path(d, "d.diff"); diff.write_text(DIFF)
            out = io.StringIO()
            with redirect_stdout(out):
                cd.main(["--tier", "e2e", "--lcov", str(pathlib.Path(d, "absent.lcov")), "--diff-file", str(diff)])
        self.assertFalse(json.loads(out.getvalue())["collected"])


def cov(covered, total, files=None, collected=True, domain=1):
    return {"covered": covered, "total": total, "collected": collected, "domain_files": domain,
            "files": files or {}, "unmeasured": []}


class TheVerdict(unittest.TestCase):
    def test_every_tier_green_passes(self):
        j = tr.judge(V, {"unit": cov(95, 100), "system": cov(8, 10), "e2e": cov(0, 0, domain=0)}, dict(PASSED))
        self.assertEqual(["pass", "pass", "not_applicable"], [v["status"] for v in j["verdicts"]])
        self.assertEqual([], j["work"])

    def test_a_shortfall_is_work_for_the_tester(self):
        j = tr.judge(V, {"unit": cov(85, 100), "system": cov(0, 0, domain=0), "e2e": cov(0, 0, domain=0)}, dict(PASSED))
        self.assertEqual([("missing_tests", "unit", "tester")], [(w["kind"], w["tier"], w["session"]) for w in j["work"]])

    def test_a_failed_tier_is_work_for_the_fixer(self):
        j = tr.judge(V, {"unit": cov(95, 100), "system": cov(1, 1), "e2e": None}, {**PASSED, "system": "failed", "e2e": "skipped"})
        self.assertEqual({("failing_tests", "system", "fixer"), ("not_run", "e2e", "fixer")},
                         {(w["kind"], w["tier"], w["session"]) for w in j["work"]})

    def test_a_skipped_tier_with_nothing_to_measure_is_not_applicable_not_not_run(self):
        j = tr.judge(V, {"unit": cov(95, 100), "system": cov(0, 0, domain=0), "e2e": cov(0, 0, domain=0)},
                     {**PASSED, "e2e": "skipped"})
        self.assertEqual("not_applicable", j["verdicts"][2]["status"])

    def test_a_skipped_tier_with_changed_lines_is_not_run(self):
        j = tr.judge(V, {"unit": cov(95, 100), "system": cov(0, 0, domain=0), "e2e": cov(5, 10)},
                     {**PASSED, "e2e": "skipped"})
        self.assertEqual("not_run", j["verdicts"][2]["status"])

    def test_a_missing_outcome_or_an_unknown_tier_is_refused_not_defaulted(self):
        with self.assertRaises(ValueError):
            tr.judge(V, {"unit": cov(95, 100)}, {"unit": "passed"})
        with self.assertRaises(ValueError):
            tr.judge(V, {"units": cov(95, 100)}, dict(PASSED))

    def test_files_with_no_coverage_data_are_reported(self):
        c = cov(0, 10, {"a.go": [1]})
        c["unmeasured"] = ["platform/manager/new.go"]
        j = tr.judge(V, {"unit": c, "system": cov(0, 0, domain=0), "e2e": cov(0, 0, domain=0)}, dict(PASSED))
        self.assertIn("platform/manager/new.go", tr.render(V, j, 1, 5, "running"))

    def test_a_ran_tier_with_no_coverage_file_fails_collection(self):
        j = tr.judge(V, {"unit": cov(95, 100), "system": None, "e2e": cov(0, 0, domain=0)}, dict(PASSED))
        self.assertEqual("collection_failed", j["verdicts"][1]["status"])

    def test_a_tier_with_no_domain_file_never_fails_on_coverage(self):
        j = tr.judge(V, {"unit": cov(0, 0, collected=False, domain=0), "system": None, "e2e": None},
                     dict(PASSED))
        self.assertEqual("not_applicable", j["verdicts"][0]["status"])


PASSED = {"unit": "passed", "system": "passed", "e2e": "passed"}


class TheReport(unittest.TestCase):
    J = tr.judge(V, {"unit": cov(85, 100, {"platform/manager/internal/x/x.go": [4, 5, 6, 9]}),
                     "system": cov(8, 10, {"platform/manager/internal/y/y.go": [1]}),
                     "e2e": cov(0, 0, domain=0)}, dict(PASSED))

    def test_the_report_carries_the_marker_the_round_and_each_tier(self):
        md = tr.render(V, self.J, 2, 5)
        self.assertTrue(md.startswith(V["testing"]["coverage_marker"]))
        self.assertIn("Round 2 of 5", md)
        for tier in ("unit", "system", "e2e"):
            self.assertIn(f"| {tier} |", md)
        self.assertIn("85.0% (85 of 100)", md)
        self.assertIn("| e2e | none | n/a | not applicable |", md)

    def test_the_report_names_files_and_line_ranges_and_quotes_no_source(self):
        md = tr.render(V, self.J, 1, 5)
        self.assertIn("`platform/manager/internal/x/x.go` (unit): lines 4-6, 9", md)
        self.assertNotIn("```", md)

    def test_the_verdict_line_follows_the_state(self):
        self.assertIn("holds its floor", tr.render(V, tr.judge(V, {"unit": cov(1, 1), "system": cov(0, 0, domain=0),
                                                                    "e2e": cov(0, 0, domain=0)}, dict(PASSED)), 1, 5, "passed"))
        self.assertIn("A person decides", tr.render(V, self.J, 5, 5, "capped"))

    def test_ranges(self):
        self.assertEqual("1-3, 7, 9-10", tr.ranges([1, 2, 3, 7, 9, 10]))
        self.assertTrue(tr.ranges(range(1, 100, 2)).endswith("more"))

    def test_a_rendered_report_holds_no_host_and_no_person(self):
        md = tr.render(V, self.J, 1, 5)
        for needle in ("@", "http", "://"):
            self.assertNotIn(needle, md)


class ThePost(unittest.TestCase):
    def test_no_marked_comment_creates_one_and_a_marked_one_is_edited(self):
        calls = []

        def fake(*args, stdin=None):
            calls.append((args, stdin))
            if args[:2] == ("api", "--paginate"):
                return json.dumps(fake.comments)
            return ""
        real = tr._gh
        tr._gh = fake
        try:
            fake.comments = [[{"id": 1, "body": "hello"}]]
            self.assertEqual("created the comment", tr.post(V, 9, "body", repo="o/r"))
            self.assertEqual("POST", calls[-1][0][2])
            fake.comments = [[{"id": 1, "body": "hello"}, {"id": 7, "body": V["testing"]["coverage_marker"] + "\nold"}]]
            self.assertEqual("edited comment 7", tr.post(V, 9, "new", repo="o/r"))
            self.assertEqual(("api", "--method", "PATCH", "repos/o/r/issues/comments/7"), calls[-1][0][:4])
        finally:
            tr._gh = real


def sh(cwd, *args):
    return subprocess.run(args, cwd=cwd, check=True, capture_output=True, text=True,
                          env={**os.environ, "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
                               "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com"}).stdout


class TheSync(unittest.TestCase):
    """A bare `origin` and a clone, so `origin/master` is the base exactly as in a pull request."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        root = pathlib.Path(self.tmp.name)
        sh(root, "git", "init", "-q", "--bare", "-b", "master", "origin.git")
        sh(root, "git", "clone", "-q", "origin.git", "work")
        self.w = root / "work"
        (self.w / "chart/crds").mkdir(parents=True)
        for f, t in (("a.go", "one\n"), ("chart/crds/c.yaml", "base: 1\n")):
            (self.w / f).write_text(t)
        sh(self.w, "git", "add", "-A"); sh(self.w, "git", "commit", "-qm", "base")
        sh(self.w, "git", "push", "-q", "origin", "HEAD:master")
        sh(self.w, "git", "checkout", "-qb", "change/x")
        self.v = json.loads(json.dumps(V))
        self.cwd = os.getcwd()
        os.chdir(self.w)

    def tearDown(self):
        os.chdir(self.cwd)
        self.tmp.cleanup()

    def base_commit(self, f, text):
        sh(self.w, "git", "checkout", "-q", "master")
        pathlib.Path(self.w, f).write_text(text)
        sh(self.w, "git", "add", "-A"); sh(self.w, "git", "commit", "-qm", "base moves")
        sh(self.w, "git", "push", "-q", "origin", "master")
        sh(self.w, "git", "checkout", "-q", "change/x")

    def branch_commit(self, f, text):
        pathlib.Path(self.w, f).write_text(text)
        sh(self.w, "git", "add", "-A"); sh(self.w, "git", "commit", "-qm", "branch moves")

    def test_a_branch_that_has_the_base_is_uptodate(self):
        self.assertEqual("uptodate", ms.sync("master", self.v, "m")["action"])

    def test_a_generated_conflict_no_regenerator_covers_is_asked_not_kept_at_the_bases_copy(self):
        self.v["testing"]["regenerators"] = []
        self.branch_commit("chart/crds/c.yaml", "mine: 1\n")
        self.base_commit("chart/crds/c.yaml", "theirs: 1\n")
        r = ms.sync("master", self.v, "m")
        self.assertEqual("asked", r["action"])
        self.assertIn("no regenerator covers", r["reason"])
        self.assertEqual("mine: 1\n", (self.w / "chart/crds/c.yaml").read_text())

    def test_a_failed_regenerator_is_asked_with_its_output(self):
        self.v["testing"]["regenerators"] = [{"paths": ["chart/crds/**"], "run": "echo boom >&2; exit 1"}]
        self.branch_commit("chart/crds/c.yaml", "mine: 1\n")
        self.base_commit("chart/crds/c.yaml", "theirs: 1\n")
        r = ms.sync("master", self.v, "m")
        self.assertEqual("asked", r["action"])
        self.assertIn("boom", r["reason"])

    def test_a_behind_branch_merges_clean_and_commits(self):
        self.branch_commit("b.go", "mine\n")
        self.base_commit("c.go", "theirs\n")
        r = ms.sync("master", self.v, "merge base")
        self.assertEqual(("updated", []), (r["action"], r["files"]))
        self.assertTrue((self.w / "c.go").exists() and (self.w / "b.go").exists())
        self.assertEqual("merge base", sh(self.w, "git", "log", "-1", "--format=%s").strip())

    def test_a_hand_written_conflict_is_asked_and_leaves_the_tree_untouched(self):
        self.branch_commit("a.go", "mine\n")
        self.base_commit("a.go", "theirs\n")
        head = sh(self.w, "git", "rev-parse", "HEAD").strip()
        r = ms.sync("master", self.v, "m")
        self.assertEqual(("asked", ["a.go"]), (r["action"], r["files"]))
        self.assertEqual(head, sh(self.w, "git", "rev-parse", "HEAD").strip())
        self.assertEqual("mine\n", (self.w / "a.go").read_text())
        self.assertEqual("", sh(self.w, "git", "status", "--porcelain").strip())

    def test_a_generated_only_conflict_is_regenerated_by_the_repositorys_script(self):
        self.v["testing"]["regenerators"] = [{"paths": ["chart/crds/**"], "run": "echo regenerated > chart/crds/c.yaml"}]
        self.branch_commit("chart/crds/c.yaml", "branch: 1\n")
        self.base_commit("chart/crds/c.yaml", "base: 2\n")
        r = ms.sync("master", self.v, "m")
        self.assertEqual(("regenerated", ["chart/crds/c.yaml"]), (r["action"], r["files"]))
        self.assertEqual("regenerated\n", (self.w / "chart/crds/c.yaml").read_text())
        self.assertEqual("", sh(self.w, "git", "status", "--porcelain").strip())

    def test_a_failing_regenerator_asks_and_commits_nothing(self):
        self.v["testing"]["regenerators"] = [{"paths": ["chart/crds/**"], "run": "false"}]
        self.branch_commit("chart/crds/c.yaml", "branch: 1\n")
        self.base_commit("chart/crds/c.yaml", "base: 2\n")
        head = sh(self.w, "git", "rev-parse", "HEAD").strip()
        r = ms.sync("master", self.v, "m")
        self.assertEqual("asked", r["action"])
        self.assertEqual(head, sh(self.w, "git", "rev-parse", "HEAD").strip())

    def test_a_mixed_conflict_is_asked_whole(self):
        self.branch_commit("a.go", "mine\n"); self.branch_commit("chart/crds/c.yaml", "branch: 1\n")
        self.base_commit("a.go", "theirs\n"); self.base_commit("chart/crds/c.yaml", "base: 2\n")
        r = ms.sync("master", self.v, "m")
        self.assertEqual(("asked", ["a.go", "chart/crds/c.yaml"]), (r["action"], r["files"]))

    def test_the_command_line_exits_3_when_it_asks(self):
        self.branch_commit("a.go", "mine\n"); self.base_commit("a.go", "theirs\n")
        out = io.StringIO()
        with redirect_stdout(out):
            rc = ms.main(["--base", "master"])
        self.assertEqual((3, "asked"), (rc, json.loads(out.getvalue())["action"]))

    def test_regenerators_cover_only_the_files_they_name(self):
        rg = [{"paths": ["a/**"], "run": "x"}, {"paths": ["b/**"], "run": "y"}, {"paths": ["a/**"], "run": "x"}]
        self.assertEqual(["x"], ms.regenerators_for(["a/f"], rg))
        self.assertEqual(["x", "y"], ms.regenerators_for(["a/f", "b/g"], rg))

    def test_a_marker_in_a_resolved_file_is_detected(self):
        (self.w / "m.txt").write_text("<<<<<<< ours\nx\n=======\ny\n>>>>>>> theirs\n")
        self.assertTrue(ms.has_marker("m.txt"))
        self.assertFalse(ms.has_marker("a.go"))
        self.assertFalse(ms.has_marker("absent"))


class TheAdapter(unittest.TestCase):
    """testing-station.py with the platform replaced: what it reads, writes and labels."""

    def setUp(self):
        self.cio = getattr(ts, "i" + "o")
        self.saved = {k: getattr(self.cio, k) for k in ("gh", "comment", "apply_events", "issue_labels", "write_output", "rest_pr", "rest_labels", "rest_comment")}
        self.events, self.posted, self.outputs, self.gh_calls = [], [], {}, []
        self.pr = {"isCrossRepository": False, "headRefName": "change/x", "body": "Refs #9", "state": "OPEN", "labels": []}
        self.labels = frozenset({"conveyor:run", "station:implement"})
        self.thread = []
        io = self.cio

        def gh(*a, check=True):
            self.gh_calls.append(a)
            if a[:2] == ("pr", "view"):
                return json.dumps(self.pr)
            if a[0] == "api":
                return json.dumps(self.thread)
            return ""
        io.gh = gh
        io.comment = lambda repo, target, body: (self.posted.append(body), self.thread.append({"body": body}))
        io.apply_events = lambda repo, target, station_event="", loop_event="", vocabulary_path=None: self.events.append(station_event)
        io.issue_labels = lambda repo, issue: self.labels
        # THE STATION READS OVER REST (a cloud sandbox refuses GraphQL), so these are what it calls.
        io.rest_pr = lambda repo, number: self.pr
        io.rest_labels = lambda repo, issue: self.labels
        io.rest_comment = io.comment
        io.write_output = lambda k, v: self.outputs.__setitem__(k, v)
        self.cwd = os.getcwd()
        self.tmp = tempfile.TemporaryDirectory()
        os.chdir(self.tmp.name)

    def tearDown(self):
        for k, v in self.saved.items():
            setattr(self.cio, k, v)
        os.chdir(self.cwd)
        self.tmp.cleanup()

    def begin(self):
        return ts.main(["begin", "--repo", "o/r", "--pr", "5"])

    def round_marker(self, n):
        return {"body": f"{V['testing']['round_marker']} {n} -->"}

    def test_the_implement_session_opening_its_pull_request_starts_the_station(self):
        self.assertEqual(0, self.begin())
        self.assertEqual(["testing:start"], self.events)
        self.assertEqual(("true", "9", "1", "5"), tuple(self.outputs[k] for k in ("active", "issue", "round", "cap")))

    def test_no_standing_instruction_means_no_station(self):
        self.labels = frozenset({"station:implement"})
        self.begin()
        self.assertEqual(("false", []), (self.outputs["active"], self.events))

    def test_a_fork_or_a_non_change_branch_or_a_closed_pull_request_is_ignored(self):
        for patch in ({"isCrossRepository": True}, {"headRefName": "feature/x"}, {"state": "MERGED"}, {"body": "no issue"}):
            self.pr = {**self.pr, **patch}
            self.outputs.clear(); self.begin()
            self.assertEqual("false", self.outputs["active"], patch)

    def test_a_station_already_running_is_not_started_again(self):
        self.labels = frozenset({"conveyor:run", "station:testing"})
        self.thread = [self.round_marker(1), self.round_marker(2)]
        self.begin()
        self.assertEqual(([], "3"), (self.events, self.outputs["round"]))

    def test_the_budget_counts_only_the_stations_own_markers(self):
        self.labels = frozenset({"conveyor:run", "station:testing"})
        self.thread = [{"body": V["round_marker"] + " 1 -->"}] * 7
        self.begin()
        self.assertEqual("1", self.outputs["round"])

    def test_a_spent_budget_with_no_keep_going_is_exhausted(self):
        self.labels = frozenset({"conveyor:run", "station:testing"})
        self.thread = [self.round_marker(i) for i in range(5)]
        self.begin()
        self.assertEqual("true", self.outputs["exhausted"])

    def test_keep_going_on_a_stalled_station_is_consumed_and_resumes_it(self):
        self.labels = frozenset({"conveyor:run", "station:stalled"})
        self.thread = [self.round_marker(i) for i in range(5)]
        self.pr["labels"] = [{"name": "conveyor:keep-going"}]
        self.begin()
        self.assertEqual(["testing:resume"], self.events)
        self.assertIn(("api", "--method", "DELETE", "repos/o/r/issues/5/labels/conveyor:keep-going"), self.gh_calls)
        self.assertEqual(("10", "6", "false"), (self.outputs["cap"], self.outputs["round"], self.outputs["exhausted"]))

    def test_a_stalled_line_without_keep_going_stays_stalled(self):
        self.labels = frozenset({"conveyor:run", "station:stalled"})
        self.begin()
        self.assertEqual(("false", []), (self.outputs["active"], self.events))

    def judge(self, tiers, outcomes=(), rnd=1, cap=5, extra=()):
        self.addCleanup(setattr, ts.report, "_gh", ts.report._gh)   # restored, never left patched for the next test
        files = []
        for t, c_ in tiers.items():
            f = pathlib.Path(f"{t}.json"); f.write_text(json.dumps(c_)); files += ["--tier-file", f"{t}={f}"]
        ts.report._gh = lambda *a, stdin=None: json.dumps([[]]) if a[:2] == ("api", "--paginate") else ""
        return ts.main(["judge", "--repo", "o/r", "--issue", "9", "--round", str(rnd), "--cap", str(cap), *files,
                        *[x for t_ in PASSED for x in ("--outcome", next((o for o in outcomes if o.startswith(t_ + "=")), f"{t_}=passed"))],
                        *extra])

    GOOD = {"unit": cov(95, 100), "system": cov(0, 0, domain=0), "e2e": cov(0, 0, domain=0)}

    def test_a_passing_round_posts_the_report_and_moves_the_line_to_fix(self):
        self.judge(self.GOOD)
        self.assertEqual(("passed", ["testing:passed"]), (self.outputs["ending"], self.events))
        self.assertTrue(pathlib.Path("coverage-report/report.md").read_text().startswith(V["testing"]["coverage_marker"]))

    def test_a_pass_asks_for_the_fixing_loops_carry_on_the_pull_request(self):
        self.judge(self.GOOD, extra=["--pr", "5"])
        self.assertIn(("workflow", "run", "remote-implement.yml", "--repo", "o/r", "-f", "carry_fix_pr=5"), self.gh_calls)

    def test_no_carry_is_asked_for_while_work_is_left(self):
        self.judge({**self.GOOD, "unit": cov(50, 100)}, extra=["--pr", "5"])
        self.assertFalse([c for c in self.gh_calls if c[:2] == ("workflow", "run")])

    def test_a_shortfall_asks_the_tester_and_a_red_asks_the_fixer(self):
        self.judge({**self.GOOD, "unit": cov(50, 100)})
        self.assertEqual(("continue", "true", "false"), tuple(self.outputs[k] for k in ("ending", "tester_work", "fixer_work")))
        self.outputs.clear()
        self.judge(self.GOOD, outcomes=["unit=failed"])
        self.assertEqual(("continue", "false", "true"), tuple(self.outputs[k] for k in ("ending", "tester_work", "fixer_work")))

    def test_the_last_round_with_work_left_stalls_the_line_and_says_how_to_extend(self):
        self.judge({**self.GOOD, "unit": cov(50, 100)}, rnd=5)
        self.assertEqual(("capped", ["testing:capped"]), (self.outputs["ending"], self.events))
        self.assertIn("conveyor:keep-going", self.posted[-1])

    def test_a_round_that_changed_nothing_ends_the_station(self):
        self.judge({**self.GOOD, "unit": cov(50, 100)}, extra=["--no-change"])
        self.assertEqual("capped", self.outputs["ending"])

    def test_record_round_posts_the_budgets_marker(self):
        ts.main(["record-round", "--repo", "o/r", "--issue", "9", "--round", "3"])
        self.assertTrue(self.posted[0].startswith(V["testing"]["round_marker"] + " 3"))

    def test_models_fail_closed(self):
        self.assertEqual(0, ts.main(["models", "--author", "claude-opus-x"]))
        self.assertEqual(5, ts.main(["models", "--author", V["testing"]["tester_model"]]))
        self.assertEqual(5, ts.main(["models", "--author", ""]))

    def test_check_patch_refuses_a_tester_patch_that_touches_production_code(self):
        self.assertEqual(0, self.patch_check("tester", "x_test.go"))
        self.assertEqual(4, self.patch_check("tester", "x.go"))


    def repo_with_changes(self, *paths):
        repo = pathlib.Path(self.tmp.name, "tree")
        repo.mkdir(exist_ok=True)
        sh(repo, "git", "init", "-q")
        (repo / "seed").write_text("x")
        sh(repo, "git", "add", "-A"); sh(repo, "git", "commit", "-qm", "seed")
        for p in paths:
            (repo / p).parent.mkdir(parents=True, exist_ok=True)
            (repo / p).write_text("changed\n")
        return repo

    def check_tree(self, role, *paths):
        repo = self.repo_with_changes(*paths)
        os.chdir(repo)
        try:
            return ts.main(["check-tree", "--role", role])
        finally:
            os.chdir(self.tmp.name)

    def test_check_tree_lets_the_tester_touch_tests_and_refuses_anything_else(self):
        self.assertEqual(0, self.check_tree("tester", "a/x_test.go", ".github/tests/y.test.sh"))
        self.assertEqual(4, self.check_tree("tester", "a/x_test.go", "a/x.go"))

    def real_model(self, paths, token, name="CLAUDE_CODE_OAUTH_TOKEN"):
        repo = self.repo_with_changes(*paths)
        sh(repo, "git", "add", "-A"); sh(repo, "git", "commit", "-qm", "change")
        sh(repo, "git", "update-ref", "refs/remotes/origin/master", "HEAD~1")
        os.chdir(repo)
        names = ("CLAUDE_CODE_OAUTH_TOKEN", "AGENTOPS_REAL_MODEL_TOKEN")
        saved = {n: os.environ.pop(n, None) for n in names}
        if token is not None:
            os.environ[name] = token
        out = io.StringIO()
        try:
            with redirect_stdout(out):
                rc = ts.main(["real-model"])
        finally:
            for n in names:
                os.environ.pop(n, None)
                if saved[n] is not None:
                    os.environ[n] = saved[n]
            os.chdir(self.tmp.name)
        return rc, out.getvalue()

    def test_the_cloud_environments_own_name_for_the_credential_counts_as_present(self):
        rc, out = self.real_model(["runtimes/pi/runtime.js"], "secret-value", name="AGENTOPS_REAL_MODEL_TOKEN")
        self.assertEqual((0, "run"), (rc, self.outputs["real_model"]))
        self.assertNotIn("secret-value", out)

    def test_an_agent_facing_change_without_the_credential_stops_the_station(self):
        rc, out = self.real_model(["runtimes/pi/runtime.js"], None)
        self.assertEqual((6, "stop"), (rc, self.outputs["real_model"]))

    def test_an_agent_facing_change_with_the_credential_runs_and_never_prints_it(self):
        rc, out = self.real_model(["runtimes/pi/runtime.js"], "sk-ant-oat-secret-value")
        self.assertEqual((0, "run"), (rc, self.outputs["real_model"]))
        self.assertNotIn("sk-ant-oat-secret-value", out)

    def test_a_blank_credential_is_absent(self):
        rc, _ = self.real_model(["runtimes/pi/runtime.js"], "   ")
        self.assertEqual(6, rc)

    def test_a_change_the_model_does_not_reach_needs_no_credential(self):
        rc, _ = self.real_model(["docs/concepts.md"], None)
        self.assertEqual((0, "not_required"), (rc, self.outputs["real_model"]))

    def test_the_tester_agent_names_the_vocabularys_model(self):
        text = (SCRIPTS.parents[1] / ".claude/agents/conveyor-tester.md").read_text()
        front = text.split("---")[1]
        model = [l.split(":", 1)[1].strip() for l in front.splitlines() if l.startswith("model:")][0]
        self.assertEqual(V["testing"]["tester_model"], model)
        # A tool list cannot scope a path, so the boundary is `check-tree --role tester`, which the routine runs
        # after every tester session. The agent must say so, and its tools are exactly the ones it needs.
        tools = [l.split(":", 1)[1].strip() for l in front.splitlines() if l.startswith("tools:")][0]
        self.assertEqual("Read, Grep, Glob, Edit, Write, Bash", tools)
        self.assertIn("test files only", text)
        self.assertIn("check-tree --role tester", (SCRIPTS.parents[1] / ".github/routines/test-change.md").read_text())

    def patch_check(self, role, path):
        repo = pathlib.Path(self.tmp.name, "r")
        repo.mkdir(exist_ok=True)
        sh(repo, "git", "init", "-q")
        (repo / path).write_text("a\n")
        sh(repo, "git", "add", "-A")
        patch = pathlib.Path(self.tmp.name, "p.patch")
        patch.write_text(sh(repo, "git", "diff", "--cached", "--binary"))
        sh(repo, "git", "rm", "-q", "-f", "--cached", path)
        (repo / path).unlink()
        os.chdir(repo)
        try:
            return ts.main(["check-patch", "--role", role, "--patch", str(patch)])
        finally:
            os.chdir(self.tmp.name)


class TheRoutine(unittest.TestCase):
    TEXT = (SCRIPTS.parents[1] / ".github/routines/test-change.md").read_text()

    def test_a_real_model_failure_is_re_run_once_before_it_counts_as_work(self):
        flat = " ".join(self.TEXT.split())
        self.assertIn("A real-model failure is re-run once before it", flat)

    def test_every_script_it_names_exists(self):
        import re
        names = set(re.findall(r"\.github/scripts/([A-Za-z0-9_.-]+\.py)", self.TEXT))
        self.assertTrue({"testing-station.py", "coverage-diff.py"} <= names)
        for n in names:
            self.assertTrue((SCRIPTS / n).is_file(), n)

    def test_every_subcommand_it_runs_is_one_the_adapter_has(self):
        import re
        used = set(re.findall(r"testing-station\.py (begin|real-model|judge|record-round|check-tree|check-patch|models)", self.TEXT))
        self.assertEqual({"begin", "real-model", "judge", "record-round", "check-tree", "models"}, used)
        for sub in used:
            with self.assertRaises(SystemExit):   # --help exits 0 only for a subcommand argparse knows
                ts.main([sub, "--help"])

    def test_it_names_no_credential_value_and_the_variable_the_vocabulary_names(self):
        self.assertIn(V["testing"]["credential_env"], self.TEXT)
        self.assertNotIn("sk-ant", self.TEXT)

    def test_the_floors_it_states_are_the_vocabularys(self):
        f = V["testing"]["floors"]
        self.assertIn(f"{f['unit']} unit, {f['system']} system, {f['e2e']} e2e", self.TEXT)

    def test_the_implement_routine_routes_the_testing_label_to_it(self):
        impl = (SCRIPTS.parents[1] / ".github/routines/implement-issue.md").read_text()
        self.assertIn("`station:testing` | testing | `.github/routines/test-change.md`", impl)

    def test_every_tier_runs_in_the_session_and_nothing_is_dispatched_to_ci(self):
        self.assertNotIn("gh workflow run", self.TEXT)
        self.assertIn("Never dispatch the tier to CI", self.TEXT)
        self.assertIn("SYSTEM_K3S_OOM_FIX=1", self.TEXT)
        self.assertIn("-timeout 150m", self.TEXT)

    def test_later_rounds_rerun_only_what_failed_and_one_full_run_judges(self):
        self.assertIn("ROUNDS ARE INCREMENTAL", self.TEXT)
        self.assertIn("-run '^(TestA|TestB)$'", self.TEXT)
        self.assertIn("ONE full run of each tier closes the station", self.TEXT)

    def test_the_routine_reads_the_tracker_over_rest_not_graphql(self):
        self.assertNotIn("gh issue view <n>", self.TEXT)
        self.assertNotIn("gh pr view <", self.TEXT)
        self.assertIn("gh api repos/<owner/name>/issues/<n>", self.TEXT)

    def test_it_never_asks_the_session_to_read_ci(self):
        self.assertIn("CI IS NOT YOUR RUNNER", self.TEXT)


class TheRestReaders(unittest.TestCase):
    """The station reads over REST, because a cloud sandbox refuses GraphQL."""

    def setUp(self):
        import conveyor_io
        self.cio = conveyor_io
        self.saved = conveyor_io.gh

    def tearDown(self):
        self.cio.gh = self.saved

    def pr(self, **over):
        doc = {"state": "open", "merged_at": None, "body": "Refs #1", "labels": [{"name": "conveyor:fix"}],
               "head": {"ref": "change/x", "repo": {"full_name": "o/r"}}, "base": {"repo": {"full_name": "o/r"}}}
        doc.update(over)
        self.cio.gh = lambda *a, check=True: json.dumps(doc)
        return self.cio.rest_pr("o/r", 5)

    def test_an_open_same_repository_pull_request(self):
        got = self.pr()
        self.assertEqual(("OPEN", False, "change/x"), (got["state"], got["isCrossRepository"], got["headRefName"]))

    def test_a_merged_pull_request_reads_as_merged_not_closed(self):
        self.assertEqual("MERGED", self.pr(state="closed", merged_at="2026-11-26T00:00:00Z")["state"])

    def test_a_closed_unmerged_pull_request_reads_as_closed(self):
        self.assertEqual("CLOSED", self.pr(state="closed")["state"])

    def test_a_fork_reads_as_cross_repository(self):
        self.assertTrue(self.pr(head={"ref": "x", "repo": {"full_name": "someone/r"}})["isCrossRepository"])


if __name__ == "__main__":
    unittest.main(verbosity=1)
