#!/usr/bin/env python3
"""The conveyor's state machine, over every state, event and combination of facts.

THE MACHINE IS THE RULE, SO THE TEST IS THE RULE'S TABLE. Each block enumerates
what a table or a decision reads and asserts the invariant that must hold for
ALL of it, then names the scenarios measured broken on #248, #254 and #255 as
single cases. Nothing here touches the network or a file: the machine has no I/O.
"""
from __future__ import annotations

import io
import itertools
import json
import pathlib
import sys
import unittest
from contextlib import redirect_stdout

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1] / "scripts"))
import conveyor as c  # noqa: E402

VOCAB_PATH = pathlib.Path(__file__).resolve().parents[1] / "review-triage.json"
V = json.loads(VOCAB_PATH.read_text())
RUN, IMPL, ARCH, FIX, KEEP = (V["run_label"], V["implement_label"], V["archive_label"],
                              V["approve_label"], V["keep_going_label"])

WRITER = c.Placer("maintainer", "write")
READER = c.Placer("stranger", "read")
BOT = c.Placer(c.WORKFLOW_BOT, bot=True)


# ==== the two machines ==========================================================================

class TheStationMachine(unittest.TestCase):
    def test_every_state_and_event_pair_is_defined(self):
        for s, e in itertools.product(c.STATION_STATES, c.STATION_EVENTS):
            self.assertIn((s, e), c.STATION_TABLE)
            nxt = c.station_next(s, e)
            self.assertTrue(nxt is None or nxt in c.STATION_STATES, (s, e, nxt))
        self.assertEqual(len(c.STATION_STATES) * len(c.STATION_EVENTS), len(c.STATION_TABLE))

    def test_done_is_terminal(self):
        for e in c.STATION_EVENTS:
            self.assertIsNone(c.station_next("done", e))

    def test_no_event_moves_a_state_to_none_and_none_is_only_the_start(self):
        for s, e in itertools.product(c.STATION_STATES, c.STATION_EVENTS):
            self.assertNotEqual("none", c.station_next(s, e))

    def test_an_implement_session_never_regresses_the_archive_station(self):
        self.assertIsNone(c.station_next("archive", "fire:implement"))
        for s in ("none", "implement", "fix", "merge", "stalled"):
            self.assertEqual("implement", c.station_next(s, "fire:implement"))

    def test_every_state_is_reachable_from_none(self):
        seen, frontier = {"none"}, ["none"]
        while frontier:
            s = frontier.pop()
            for e in c.STATION_EVENTS:
                n = c.station_next(s, e)
                if n and n not in seen:
                    seen.add(n); frontier.append(n)
        self.assertEqual(set(c.STATION_STATES), seen)

    def test_unknown_pairs_are_errors_not_silence(self):
        with self.assertRaises(ValueError):
            c.station_next("archive", "vanish")
        with self.assertRaises(ValueError):
            c.station_next("limbo", "round:start")

    def test_the_whole_opsx_line_walks_from_proposal_to_done(self):
        walk = [("none", "fire:implement", "implement"), ("implement", "round:start", "fix"),
                ("fix", "pr:green", "merge"), ("merge", "merge:proposal_or_apply", "implement"),
                ("implement", "fire:implement", "implement"), ("implement", "round:start", "fix"),
                ("fix", "pr:green", "merge"), ("merge", "merge:finished_carried", "archive"),
                ("archive", "round:start", "fix"), ("fix", "pr:green", "merge"),
                ("merge", "merge:archive_pr", "done")]
        for start, event, want in walk:
            self.assertEqual(want, c.station_next(start, event), (start, event))

    def test_a_merge_with_nothing_to_carry_is_stalled_and_resumes_by_fire(self):
        self.assertEqual("stalled", c.station_next("merge", "merge:finished_stalled"))
        self.assertEqual("archive", c.station_next("stalled", "fire:archive"))

    def test_the_plain_lane_ends_at_the_merge(self):
        self.assertEqual("done", c.station_next("merge", "merge:plain"))


class TheLoopMachine(unittest.TestCase):
    def test_every_state_and_event_pair_is_defined(self):
        for s, e in itertools.product(c.LOOP_STATES, c.LOOP_EVENTS):
            self.assertIn((s, e), c.LOOP_TABLE)
            nxt = c.loop_next(s, e)
            self.assertTrue(nxt is None or nxt in c.LOOP_STATES, (s, e, nxt))
        self.assertEqual(len(c.LOOP_STATES) * len(c.LOOP_EVENTS), len(c.LOOP_TABLE))

    def test_a_round_owns_the_label_while_it_runs(self):
        for e in ("ci:green_clean", "thread:opened"):
            self.assertIsNone(c.loop_next("running", e), e)

    def test_an_ending_sets_its_state_from_anywhere(self):
        for s in c.LOOP_STATES:
            self.assertEqual("mergeable", c.loop_next(s, "end:mergeable"))
            self.assertEqual("stalled", c.loop_next(s, "end:stalled"))
            self.assertEqual("capped", c.loop_next(s, "end:capped"))
            self.assertEqual("running", c.loop_next(s, "end:continue"))
            self.assertEqual("running", c.loop_next(s, "round:start"))

    def test_226_a_new_thread_demotes_only_a_mergeable_label(self):
        self.assertEqual("stalled", c.loop_next("mergeable", "thread:opened"))
        for s in ("none", "stalled", "capped"):
            self.assertIsNone(c.loop_next(s, "thread:opened"))

    def test_225_a_green_clean_ci_cannot_leave_running_stuck_and_cannot_overwrite_it(self):
        for s in ("none", "stalled", "capped", "mergeable"):
            self.assertEqual("mergeable", c.loop_next(s, "ci:green_clean"))
        self.assertIsNone(c.loop_next("running", "ci:green_clean"))

    def test_recovery_only_corrects_a_running_label(self):
        for s in c.LOOP_STATES:
            for e, want in (("recover:stalled", "stalled"), ("recover:capped", "capped")):
                self.assertEqual(want if s == "running" else None, c.loop_next(s, e))

    def test_every_state_is_reachable(self):
        seen, frontier = {"none"}, ["none"]
        while frontier:
            s = frontier.pop()
            for e in c.LOOP_EVENTS:
                n = c.loop_next(s, e)
                if n and n not in seen:
                    seen.add(n); frontier.append(n)
        self.assertEqual(set(c.LOOP_STATES), seen)

    def test_capped_is_left_only_by_a_new_round_or_a_green_clean_head(self):
        leaving = {e for e in c.LOOP_EVENTS if c.loop_next("capped", e) not in (None, "capped")}
        self.assertEqual({"round:start", "end:continue", "end:mergeable", "end:stalled", "ci:green_clean"}, leaving)

    def test_a_full_fixing_loop_walk(self):
        walk = [("none", "ci:green_clean", "mergeable"), ("mergeable", "thread:opened", "stalled"),
                ("stalled", "round:start", "running"), ("running", "end:continue", "running"),
                ("running", "end:stalled", "stalled"), ("stalled", "round:start", "running"),
                ("running", "end:capped", "capped"), ("capped", "round:start", "running"),
                ("running", "end:mergeable", "mergeable")]
        for start, event, want in walk:
            self.assertEqual(want, c.loop_next(start, event), (start, event))


# ==== the grant rule ============================================================================

class TheGrantRule(unittest.TestCase):
    def test_run_stands_for_every_station(self):
        for station, closes in itertools.product(("fix", "archive", "implement"), (False, True)):
            self.assertEqual(RUN, c.standing_grant(V, {RUN}, station, closes))
            self.assertEqual(RUN, c.standing_grant(V, {RUN, ARCH}, station, closes))

    def test_archive_stands_for_the_archive_station_and_the_archive_pull_requests_fix_alone(self):
        for station, closes in itertools.product(("fix", "archive", "implement"), (False, True)):
            want = ARCH if (station == "archive" or (station == "fix" and closes)) else None
            self.assertEqual(want, c.standing_grant(V, {ARCH}, station, closes), (station, closes))

    def test_nothing_else_is_a_grant(self):
        for station, closes in itertools.product(("fix", "archive", "implement"), (False, True)):
            self.assertIsNone(c.standing_grant(V, {"opsx:review", "station:fix", IMPL, FIX, KEEP}, station, closes))


# ==== the decisions =============================================================================

ISSUE_LABELS = (frozenset(), frozenset({RUN}), frozenset({ARCH}), frozenset({RUN, ARCH}), frozenset({IMPL}), frozenset({"opsx:review"}))
FIRED = (frozenset(), frozenset({"implement"}), frozenset({"archive"}), frozenset({"implement", "archive"}))


def lines():
    for labels, lane, finished, fired, open_pr in itertools.product(ISSUE_LABELS, ("opsx", "plain"), (False, True), FIRED, (False, True)):
        yield c.Line(labels, lane, finished, fired, open_pr)


def pull_requests(state="OPEN", labels=frozenset({FIX})):
    for same, refs, closes, review, thread, ci in itertools.product(
            (True, False), (None, 51), (None, 51), (False, True), (False, True), ("success", "failure")):
        yield c.PullRequest(same, state, labels, refs, closes, review, thread, ci, fork=not same)


class TheFire(unittest.TestCase):
    def expected(self, label, line, placer, grant_placer=None):
        if label not in (RUN, IMPL, ARCH):
            return "ignore", None
        if placer.bot and (RUN not in line.issue_labels or grant_placer is None or not grant_placer.may_push):
            return "refuse", label
        if not placer.bot and not placer.may_push:
            return "refuse", label
        station = "archive" if (label == ARCH or (label == RUN and line.lane == "opsx" and line.change_finished)) else "implement"
        if station == "archive" and (line.lane != "opsx" or not line.change_finished):
            return "refuse", label if label == ARCH else ""
        if station in line.fired and (placer.bot or line.open_pr_from_change):
            return "skip", station
        return "fire", station

    def test_every_combination(self):
        for label, line, placer, grant in itertools.product((RUN, IMPL, ARCH, "bug"), lines(), (WRITER, READER, BOT), (WRITER, READER, None)):
            d = c.fire(V, label, line, placer, grant)
            action, extra = self.expected(label, line, placer, grant)
            with self.subTest(label=label, line=line, placer=placer.login, grant=grant and grant.login):
                self.assertEqual(action, d.action, d.reason)
                if action == "refuse":
                    self.assertEqual(extra, d.remove_label)
                if action in ("fire", "skip"):
                    self.assertEqual(extra, d.station)
                if action == "fire":
                    self.assertEqual(f"fire:{extra}", d.station_event)
                else:
                    self.assertEqual("", d.station_event)

    def test_an_unfinished_change_never_reaches_the_archive_station(self):
        for label, line, placer in itertools.product((RUN, IMPL, ARCH), lines(), (WRITER, BOT)):
            d = c.fire(V, label, line, placer, WRITER)
            if d.action in ("fire", "skip") and d.station == "archive":
                self.assertTrue(line.change_finished and line.lane == "opsx")

    def test_255_run_placed_after_the_proposal_merged_fires_implement_not_archive(self):
        line = c.Line(frozenset({RUN, "opsx:proposed"}), "opsx", change_finished=False)
        d = c.fire(V, RUN, line, WRITER)
        self.assertEqual(("fire", "implement"), (d.action, d.station))

    def test_run_placed_after_the_change_merged_fires_the_archive_station(self):
        line = c.Line(frozenset({RUN, "opsx:review"}), "opsx", change_finished=True)
        d = c.fire(V, RUN, line, WRITER)
        self.assertEqual(("fire", "archive"), (d.action, d.station))

    def test_a_carried_placement_needs_the_person_behind_it_to_still_push(self):
        line = c.Line(frozenset({RUN}), "opsx", change_finished=False)
        self.assertEqual("fire", c.fire(V, RUN, line, BOT, WRITER).action)
        for grant in (READER, None):
            d = c.fire(V, RUN, line, BOT, grant)
            self.assertEqual(("refuse", RUN), (d.action, d.remove_label))

    def test_a_dead_session_is_restarted_by_a_person_and_never_by_a_carry(self):
        base = dict(issue_labels=frozenset({RUN}), lane="opsx", change_finished=False, fired=frozenset({"implement"}))
        self.assertEqual("fire", c.fire(V, RUN, c.Line(**base, open_pr_from_change=False), WRITER).action)
        self.assertEqual("skip", c.fire(V, RUN, c.Line(**base, open_pr_from_change=False), BOT, WRITER).action)
        self.assertEqual("skip", c.fire(V, RUN, c.Line(**base, open_pr_from_change=True), WRITER).action)


class TheCarries(unittest.TestCase):
    def test_fix_is_carried_exactly_where_the_grant_stands_for_this_pull_request(self):
        for pr, line, placer in itertools.product(pull_requests(), lines(), (WRITER, READER, None)):
            d = c.carry_fix(V, pr, line, placer)
            with self.subTest(pr=pr, labels=sorted(line.issue_labels), placer=placer and placer.login):
                grant = c.standing_grant(V, line.issue_labels, "fix", pr.closes is not None)
                if not pr.same_repo_change_branch or (pr.refs is None and pr.closes is None) \
                        or grant is None or placer is None or not placer.may_push:
                    self.assertEqual("nothing", d.action)
                    continue
                self.assertEqual(("carry", grant), (d.action, d.grant))
                self.assertEqual(pr.review_completed, d.dispatch_round, "dispatch only after the review completed")
                green = pr.ci_conclusion == "success" and not pr.thread_open
                self.assertEqual(("ci:green_clean", "pr:green") if green else ("", ""), (d.loop_event, d.station_event))

    def test_archive_is_carried_only_for_a_finished_change_with_the_standing_instruction(self):
        for pr, line, placer in itertools.product(pull_requests("MERGED", frozenset()), lines(), (WRITER, READER, None)):
            d = c.carry_archive(V, pr, line, placer)
            with self.subTest(pr=pr, line=line, placer=placer and placer.login):
                if not pr.same_repo_change_branch:
                    self.assertEqual(("nothing", ""), (d.action, d.station_event))
                elif pr.closes is not None and pr.refs is None:
                    self.assertEqual(("done", "merge:archive_pr"), (d.action, d.station_event))
                elif pr.refs is None:
                    self.assertEqual("nothing", d.action)
                elif line.lane != "opsx":
                    self.assertEqual(("done", "merge:plain"), (d.action, d.station_event))
                elif not line.change_finished:
                    self.assertEqual(("nothing", "merge:proposal_or_apply"), (d.action, d.station_event))
                elif RUN not in line.issue_labels:
                    if ARCH in line.issue_labels:
                        self.assertEqual(("nothing", ""), (d.action, d.station_event))
                    else:
                        self.assertEqual(("stalled", "merge:finished_stalled"), (d.action, d.station_event))
                elif placer is None or not placer.may_push:
                    self.assertEqual("nothing", d.action)
                else:
                    self.assertEqual(("carry", "archive", "merge:finished_carried"), (d.action, d.station, d.station_event))

    def test_whatever_the_carry_carries_the_fire_accepts(self):
        """The carry and the fire agree on the archive station, for every line."""
        for line in lines():
            merged = c.PullRequest(True, "MERGED", frozenset(), refs=51)
            carried = c.carry_archive(V, merged, line, WRITER)
            if carried.action != "carry":
                continue
            after = c.Line(line.issue_labels | {ARCH}, line.lane, line.change_finished, line.fired, line.open_pr_from_change)
            fired = c.fire(V, ARCH, after, BOT, WRITER)
            self.assertIn(fired.action, ("fire", "skip"), fired.reason)
            self.assertEqual("archive", fired.station)

    def test_255_a_proposal_merge_carries_nothing_and_moves_the_line_to_implement(self):
        merged = c.PullRequest(True, "MERGED", frozenset(), refs=255)
        line = c.Line(frozenset({RUN, "opsx:proposed"}), "opsx", change_finished=False)
        d = c.carry_archive(V, merged, line, WRITER)
        self.assertEqual(("nothing", "merge:proposal_or_apply"), (d.action, d.station_event))
        self.assertEqual("implement", c.station_next("merge", d.station_event))


class TheGate(unittest.TestCase):
    def test_a_bot_start_is_accepted_exactly_where_the_grant_stands(self):
        """The #254 invariant, on every path a bot can start a round by: the grant rule decides."""
        for pr, line, placer in itertools.product(pull_requests(), lines(), (WRITER, READER, None)):
            issue = pr.refs or pr.closes
            grant = c.standing_grant(V, line.issue_labels, "fix", pr.closes is not None) if issue else None
            backed = grant is not None and placer is not None and placer.may_push
            for event in ("labeled", "dispatch"):
                trig = c.Trigger(event, BOT, label=FIX if event == "labeled" else "", mode="all")
                d = c.gate(V, trig, pr, line, placer)
                with self.subTest(pr=pr, line=line, event=event):
                    if pr.fork:
                        self.assertEqual("refuse", d.action)
                    elif backed:
                        self.assertEqual("round", d.action, d.reason)
                        self.assertEqual(("round:start", "round:start"), (d.loop_event, d.station_event))
                    else:
                        # A carried label whose grant no longer stands comes off, on every path.
                        self.assertEqual(("refuse", FIX), (d.action, d.remove_label))

    def test_whatever_the_carry_carries_the_gate_accepts(self):
        for pr, line, placer in itertools.product(pull_requests(), lines(), (WRITER, READER, None)):
            if c.carry_fix(V, pr, line, placer).action != "carry":
                continue
            for event in ("labeled", "dispatch"):
                d = c.gate(V, c.Trigger(event, BOT, label=FIX, mode="all"), pr, line, placer)
                self.assertEqual("round", d.action, d.reason)

    def test_the_workflow_carries_the_fix_label_and_nothing_else(self):
        pr = c.PullRequest(True, "OPEN", frozenset({FIX, KEEP}), refs=7)
        line = c.Line(frozenset({RUN}))
        d = c.gate(V, c.Trigger("labeled", BOT, KEEP), pr, line, WRITER)
        self.assertEqual(("refuse", KEEP), (d.action, d.remove_label))

    def test_a_completion_on_a_carried_label_re_checks_the_grant_so_removing_it_stops_the_loop(self):
        """The stop button: take `conveyor:run` off the issue and the next start refuses, removes the carried label and stalls."""
        pr = c.PullRequest(True, "OPEN", frozenset({FIX}), refs=248)
        for event in ("review_completed", "ci_failed"):
            standing = c.gate(V, c.Trigger(event, label_carried=True), pr, c.Line(frozenset({RUN})), WRITER)
            withdrawn = c.gate(V, c.Trigger(event, label_carried=True), pr, c.Line(frozenset()), None)
            lost_write = c.gate(V, c.Trigger(event, label_carried=True), pr, c.Line(frozenset({RUN})), READER)
            self.assertEqual("round", standing.action)
            for d in (withdrawn, lost_write):
                self.assertEqual(("refuse", FIX, "end:stalled"), (d.action, d.remove_label, d.loop_event))

    def test_a_completion_on_a_label_a_PERSON_placed_needs_no_grant(self):
        pr = c.PullRequest(True, "OPEN", frozenset({FIX}), refs=248)
        for event in ("review_completed", "ci_failed"):
            self.assertEqual("round", c.gate(V, c.Trigger(event, label_carried=False), pr, c.Line(), None).action)

    def test_a_hand_run_in_threads_mode_needs_no_label_and_in_all_mode_needs_the_label_and_the_bound(self):
        unlabelled = c.PullRequest(True, "OPEN", frozenset(), refs=7)
        labelled = c.PullRequest(True, "OPEN", frozenset({FIX}), refs=7)
        hand = lambda mode: c.Trigger("dispatch", WRITER, mode=mode)
        self.assertEqual("threads", c.gate(V, hand("threads"), unlabelled, c.Line(), None).action)
        self.assertEqual("none", c.gate(V, hand("all"), unlabelled, c.Line(), None).action)
        self.assertEqual("round", c.gate(V, hand("all"), labelled, c.Line(), None).action)
        capped = c.PullRequest(True, "OPEN", frozenset({FIX}), refs=7, rounds_used=5, max_rounds=5)
        self.assertEqual("none", c.gate(V, hand("all"), capped, c.Line(), None).action)

    def test_a_fork_is_refused_on_every_path(self):
        fork = c.PullRequest(True, "OPEN", frozenset({FIX}), refs=7, fork=True)
        for trig in (c.Trigger("labeled", WRITER, FIX), c.Trigger("comment", WRITER, comment_is_dispatch=True),
                     c.Trigger("review_completed"), c.Trigger("dispatch", WRITER, mode="threads")):
            self.assertEqual("refuse", c.gate(V, trig, fork, c.Line(), None).action)

    def test_a_non_change_branch_in_this_repository_is_not_a_fork(self):
        """A person may label any same-repository pull request; only the carries need change/*."""
        pr = c.PullRequest(same_repo_change_branch=False, state="OPEN", labels=frozenset({FIX}), refs=None)
        self.assertEqual("round", c.gate(V, c.Trigger("labeled", WRITER, FIX), pr, c.Line(), None).action)

    def test_a_review_or_ci_completion_starts_a_round_only_on_a_labelled_open_pull_request_under_the_bound(self):
        for labels, state, used in itertools.product((frozenset(), frozenset({FIX}), frozenset({FIX, KEEP})),
                                                     ("OPEN", "MERGED", "CLOSED"), (0, 4, 5, 6)):
            pr = c.PullRequest(True, state, labels, refs=7, rounds_used=used, max_rounds=5)
            for event in ("review_completed", "ci_failed"):
                d = c.gate(V, c.Trigger(event), pr, c.Line(), None)
                with self.subTest(labels=sorted(labels), state=state, used=used, event=event):
                    if state != "OPEN" or FIX not in labels:
                        self.assertEqual("none", d.action)
                    elif used >= 5 and KEEP not in labels:
                        self.assertEqual(("none", "end:capped"), (d.action, d.loop_event))
                    else:
                        self.assertEqual("round", d.action)

    def test_the_bound_is_enforced_before_the_round_and_a_grant_extends_it_by_a_full_set(self):
        for grants in (0, 1, 2):
            cap = c.cap_for(5, grants)
            self.assertEqual(5 * (1 + grants), cap)
            under = c.PullRequest(True, "OPEN", frozenset({FIX}), refs=7, rounds_used=cap - 1, grants=grants)
            at = c.PullRequest(True, "OPEN", frozenset({FIX}), refs=7, rounds_used=cap, grants=grants)
            self.assertEqual("round", c.gate(V, c.Trigger("review_completed"), under, c.Line(), None).action)
            self.assertEqual("none", c.gate(V, c.Trigger("review_completed"), at, c.Line(), None).action)

    def test_248_a_capped_loop_does_not_start_the_round_after_the_cap(self):
        pr = c.PullRequest(True, "OPEN", frozenset({FIX}), refs=248, rounds_used=5, max_rounds=5)
        d = c.gate(V, c.Trigger("review_completed"), pr, c.Line(), None)
        self.assertEqual(("none", "end:capped"), (d.action, d.loop_event))

    def test_a_writers_label_starts_a_round_and_a_readers_is_stripped(self):
        pr = c.PullRequest(True, "OPEN", frozenset({FIX}), refs=7)
        self.assertEqual("round", c.gate(V, c.Trigger("labeled", WRITER, FIX), pr, c.Line(), None).action)
        d = c.gate(V, c.Trigger("labeled", READER, FIX), pr, c.Line(), None)
        self.assertEqual(("refuse", FIX), (d.action, d.remove_label))

    def test_a_comment_is_a_dispatch_only_in_the_stated_form_from_a_writer(self):
        pr = c.PullRequest(True, "OPEN", frozenset(), refs=7)
        self.assertEqual("none", c.gate(V, c.Trigger("comment", WRITER, comment_is_dispatch=False), pr, c.Line(), None).action)
        self.assertEqual("threads", c.gate(V, c.Trigger("comment", WRITER, comment_is_dispatch=True), pr, c.Line(), None).action)
        self.assertEqual("refuse", c.gate(V, c.Trigger("comment", READER, comment_is_dispatch=True), pr, c.Line(), None).action)

    def test_254_the_archive_pull_request_under_the_archive_grant_alone(self):
        pr = c.PullRequest(True, "OPEN", frozenset({FIX}), closes=245)
        line = c.Line(frozenset({ARCH, "opsx:archived"}), "opsx", change_finished=True)
        for event in ("labeled", "dispatch"):
            d = c.gate(V, c.Trigger(event, BOT, label=FIX), pr, line, WRITER)
            self.assertEqual("round", d.action, d.reason)

    def test_the_archive_grant_does_not_stand_for_an_ordinary_pull_request(self):
        pr = c.PullRequest(True, "OPEN", frozenset({FIX}), refs=255)
        line = c.Line(frozenset({ARCH}), "opsx", change_finished=True)
        self.assertEqual("refuse", c.gate(V, c.Trigger("dispatch", BOT), pr, line, WRITER).action)


class TheGuard(unittest.TestCase):
    def test_the_check_never_reads_a_running_round_and_the_archive_command_always_does(self):
        for labels, state, running, disputes in itertools.product(
                (frozenset(), frozenset({FIX})), ("OPEN", "MERGED"), (0, 1, 3), (0, 1)):
            pr = c.PullRequest(True, state, labels)
            ci = c.guard(V, "ci", pr, running, disputes)
            archive = c.guard(V, "archive", pr, running, disputes)
            with self.subTest(labels=sorted(labels), state=state, running=running, disputes=disputes):
                if state != "OPEN" or not labels:
                    self.assertEqual(("allow", "allow"), (ci.action, archive.action))
                    continue
                self.assertEqual("refuse" if disputes else "allow", ci.action)
                self.assertEqual("refuse" if (disputes or running) else "allow", archive.action)
                self.assertNotIn("round", ci.reason)

    def test_248_a_running_round_never_turns_the_check_red(self):
        pr = c.PullRequest(True, "OPEN", frozenset({FIX}))
        self.assertEqual("allow", c.guard(V, "ci", pr, running_rounds=3, unanswered_disputes=0).action)

    def test_an_unknown_purpose_is_an_error(self):
        with self.assertRaises(ValueError):
            c.guard(V, "sometimes", c.PullRequest(), 0, 0)


class TheEndings(unittest.TestCase):
    def test_every_ending_sets_one_loop_event_and_spends_what_it_should(self):
        for kind, number, cap, thread, waiting in itertools.product(c.ENDINGS, (1, 3, 5, 6), (5,), (False, True), (False, True)):
            d = c.ending(kind, number, cap, thread, waiting)
            with self.subTest(kind=kind, number=number, thread=thread, waiting=waiting):
                self.assertIn(d.loop_event, ("end:continue", "end:mergeable", "end:stalled", "end:capped"))
                self.assertEqual(kind not in c.NO_MODEL_RAN, d.counted)
                if d.counted and number >= cap:
                    self.assertEqual("end:capped", d.loop_event)
                elif kind == "landed":
                    self.assertEqual("end:continue", d.loop_event)
                elif kind == "clean":
                    self.assertEqual("end:stalled" if (thread or waiting) else "end:mergeable", d.loop_event)
                else:
                    self.assertEqual("end:stalled", d.loop_event)

    def test_248_five_timeouts_reach_the_cap(self):
        for n in range(1, 5):
            d = c.ending("timed out", n, 5)
            self.assertEqual(("end:stalled", True), (d.loop_event, d.counted))
        self.assertEqual("end:capped", c.ending("timed out", 5, 5).loop_event)

    def test_a_round_in_which_no_model_ran_spends_nothing(self):
        for kind in ("failed", "clean"):
            self.assertFalse(c.ending(kind, 5, 5).counted, kind)

    def test_every_round_that_ran_a_model_counts_and_can_reach_the_cap(self):
        for kind in ("landed", "timed out", "no report", "disputed", "stale patch", "waiting"):
            self.assertTrue(c.ending(kind, 1, 5).counted, kind)
            self.assertEqual("end:capped", c.ending(kind, 5, 5).loop_event, kind)

    def test_a_loop_that_only_disputes_is_bounded_too(self):
        # a person's answer restarts the loop each time, and the fifth still ends it
        used = [c.ending("disputed", n, 5) for n in range(1, 6)]
        self.assertEqual(["end:stalled"] * 4 + ["end:capped"], [d.loop_event for d in used])

    def test_an_unknown_ending_is_an_error_not_a_state(self):
        with self.assertRaises(ValueError):
            c.ending("vanished", 1, 5)

    def test_226_a_clean_round_with_a_thread_open_is_stalled_never_left_running(self):
        d = c.ending("clean", 2, 5, thread_open=True)
        self.assertEqual("stalled", c.loop_next("running", d.loop_event))


class TheCounters(unittest.TestCase):
    def test_one_counter_for_rounds_and_grants_since_the_label(self):
        comments = [{"body": "<!-- conveyor:round 1 -->", "created_at": "2026-08-29T11:00:00Z"},
                    {"body": "<!-- conveyor:round 2 -->", "created_at": "2026-08-29T12:00:00Z"},
                    {"body": "<!-- conveyor:round 9 -->", "created_at": "2026-08-28T12:00:00Z"},
                    {"body": "<!-- conveyor:grant -->", "created_at": "2026-08-29T13:00:00Z"},
                    {"body": "an ordinary comment", "created_at": "2026-08-29T13:00:00Z"}]
        self.assertEqual(2, c.count_marked(comments, V["round_marker"], "2026-08-29T10:00:00Z"))
        self.assertEqual(3, c.count_marked(comments, V["round_marker"], ""))
        self.assertEqual(1, c.count_marked(comments, V["grant_marker"], "2026-08-29T10:00:00Z"))

    def test_finished_means_every_box_ticked_and_at_least_one_box(self):
        cases = [("", False), ("no boxes here", False), ("- [x] a\n- [x] b", True), ("- [x] a\n- [ ] b", False),
                 ("## 1\n- [X] a\n  - [x] nested", True), ("- [ ] only", False)]
        for text, want in cases:
            self.assertEqual(want, c.is_finished(text), text)


class TheChecks(unittest.TestCase):
    REQUIRED = {"operator", "docs-task", "images"}

    def test_which_red_job_is_work(self):
        guard = "No change is archived while a dispute is unanswered"
        cases = [
            ("ci-green", "failure", [], "skip"),
            ("review-clean", "failure", [], "skip"),
            ("images (manager)", "failure", ["Build"], "work"),
            ("operator", "success", [], "skip"),
            ("operator", "cancelled", [], "skip"),
            ("docs-task", "failure", [guard], "waiting"),
            ("docs-task", "failure", ["Every change ends in finished tasks"], "work"),
            ("docs-task", "failure", ["Every change ends in finished tasks", guard], "work"),
            ("docs-task", "failure", [], "work"),
        ]
        for job, conclusion, steps, want in cases:
            with self.subTest(job=job, steps=steps):
                self.assertEqual(want, c.check_is_work(job, conclusion, steps, guard, self.REQUIRED).action)

    def test_without_a_named_guard_step_nothing_is_waiting(self):
        self.assertEqual("work", c.check_is_work("docs-task", "failure", ["anything"], "", self.REQUIRED).action)


class TheCorrections(unittest.TestCase):
    def test_recover_and_refresh_only_touch_the_label_they_own(self):
        for current, used, cap, thread in itertools.product(c.LOOP_STATES, (3, 5, 6), (5,), (False, True)):
            event = c.recover(current, used, cap, thread)
            if current != "running":
                self.assertEqual("", event)
            elif used >= cap:
                self.assertEqual("recover:capped", event)
            else:
                self.assertEqual("recover:stalled" if thread else "", event)
            if event:
                self.assertIn(event, c.LOOP_EVENTS)
        for current, thread in itertools.product(c.LOOP_STATES, (False, True)):
            event = c.refresh(current, thread)
            self.assertEqual("thread:opened" if (thread and current == "mergeable") else "", event)


class TheCommandLine(unittest.TestCase):
    def run_cli(self, *argv, stdin=""):
        out = io.StringIO()
        real = sys.stdin
        sys.stdin = io.StringIO(stdin)
        try:
            with redirect_stdout(out):
                rc = c.main(["--vocabulary", str(VOCAB_PATH), *argv])
        finally:
            sys.stdin = real
        return rc, out.getvalue().strip()

    def test_next_prints_the_state_after_an_event_or_the_state_unchanged(self):
        self.assertEqual((0, "fix"), self.run_cli("next", "station", "merge", "round:start"))
        self.assertEqual((0, "done"), self.run_cli("next", "station", "done", "round:start"))
        self.assertEqual((0, "running"), self.run_cli("next", "loop", "running", "ci:green_clean"))

    def test_decide_round_trips_facts_as_json(self):
        facts = {"label": RUN, "line": {"issue_labels": [RUN], "lane": "opsx", "change_finished": True},
                 "placer": {"login": "maintainer", "permission": "write"}}
        rc, out = self.run_cli("decide", "fire", stdin=json.dumps(facts))
        d = json.loads(out)
        self.assertEqual((0, "fire", "archive", "fire:archive"), (rc, d["action"], d["station"], d["station_event"]))

    def test_decide_refuses_an_unknown_fact(self):
        with self.assertRaises(ValueError):
            self.run_cli("decide", "fire", stdin=json.dumps({"label": RUN, "line": {"nonsense": 1}, "placer": {"login": "x"}}))


if __name__ == "__main__":
    unittest.main(verbosity=1)
