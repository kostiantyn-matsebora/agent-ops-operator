## 1. Poll the listing

- [x] 1.1 In `signals/ha/main.go`, split today's `backfill` into a `sweep`
      that reads `system_log/list` once, stores the records half of the
      source's `healthSnapshot` from that read (config entries refreshed as
      `refreshSnapshot` does), recovers a stale cursor, and considers every
      record newer than the cursor. `startSession` calls it once in place of
      `backfill`. Verify with `go build ./... && go vet ./...` in
      `signals/ha/` inside the build container, run against THIS WORKTREE's
      path (`docker exec -w "$PWD"`), never the main checkout's.
      **Amended at review:** the two arrival paths deduplicate PER RECORD on
      the occurrence's timestamp (`servedSource.seen`, pruned to the listing
      on every sweep), not through the cursor — the cursor alone left an
      event racing a sweep to a double post, which the spec's "not
      considered again by the other" does not allow. `design.md` has the
      reasoning; the event-then-poll test now covers both orders.
- [x] 1.2 Add `pollEvery = 15 * time.Second` beside the other constants, with
      a comment stating why it is a constant, and a `runPoller` goroutine
      started by `runSession` after `startSession` succeeds that calls
      `sweep` on each tick and returns on the session's `Done()` or the
      context. Verify by reading the diff that the goroutine cannot outlive
      its session and that a sweep failure is logged and does not end the
      session.
- [x] 1.3 Make `backfill: false` seed the cursor to the newest record in the
      connect-time listing instead of returning early, so later ticks still
      consider new records. Verify with the unit test in 2.3.
- [x] 1.4 Bump `chart/charts/home-assistant/values.yaml`'s pinned
      `logsAdapter.image.tag` to `0.3.0` and verify the chart's render tests
      still pass (`go test ./internal/integration/...` for the chart tests
      in `platform/manager/`, inside the container, from the worktree).

## 2. Unit tests

- [x] 2.1 Extend `signals/ha/fakeha_test.go` so a fake Home Assistant can be
      told to fire no events (the default install), and make `SetRecords`
      usable after connect so a test can make a record appear or recur in
      the listing. Verify `go test ./...` in `signals/ha/` passes.
      **Found already true, so no extension was needed:** the double fires
      an event only when a test pushes one, and `SetRecords` is mutex-guarded
      and served on every `system_log/list`. The one edit made is a
      registration-order fix — a connection is listed BEFORE `auth_ok` is
      sent, because the client returns from connect on that frame and
      `TestSessionErrReportsWhyItEnded`'s `Close()` could race the append
      and time out (seen once in a `-count=3 -race` run) — and, found by the
      next run, `PushLogEvent` now writes under the fake's mutex, because an
      event written while a command's result was mid-write interleaved the
      two frames and killed the session. Both are defects of the double,
      neither of the adapter; `-count=3 -race` is clean with them fixed.
- [x] 2.2 Add `TestPolledRecordBecomesASignal` — no events, a record added to
      the listing after connect is posted — and
      `TestPolledRecurrenceSurvivesTheDwell` — no events, a record under a
      dwell rule with no config-entry predicate recurs in the listing
      through the window and is posted at the close, while one that stops
      recurring is dropped as quiet. Verify both pass with `pollEvery`
      shortened for the test.
- [x] 2.3 Add `TestEventDeliveredRecordIsNotPolledTwice` — events on, one
      occurrence delivered by event, the same entry in the listing, exactly
      one post — and `TestBackfillOffStillPolls` — `backfill: false`, the
      connect-time listing is not reported, a record logged after connect
      is. Verify both pass, then run the module's whole suite with
      `-count=3 -race` in the container from the worktree.

## 3. E2E tests

- [x] 3.1 Not applicable — nothing here is decided by a cluster. The change is
      inside one dependency-free process talking to Home Assistant's
      WebSocket API; the unit suite's fake Home Assistant is the deciding
      double, per `docs/testing.md`.

## 4. Documentation

### Reference docs

- [x] 4.1 `docs/CHANGELOG.md` — an Unreleased entry naming `signal-ha` 0.3.0:
      the lane now polls the listing and no longer depends on `fire_event`,
      and what an install saw before (no signals on a default Home
      Assistant, `Ready=True` throughout). Verify the entry sits under
      `## [Unreleased]`.
- [x] 4.2 `.claude/rules/structure.md` — rewrite the `signals/ha/` bullet that
      names `system_log_event` "with `system_log/list` for backfill" so it
      says the listing is POLLED and the event is the optional fast path,
      with the one-line reason (`fire_event` defaults off). Verify by
      reading the bullet back.
- [x] 4.3 `.claude/rules/chart.md` — the operator-credential paragraph names
      `subscribe_events` as the admin-only command; add `system_log/list`,
      which the lane now depends on and is admin-only too. Verify by
      reading it back.

### The adopter site

- [x] 4.4 `docs/integrations/home-assistant.md` — three edits: the callout on
      the operator token names `system_log/list` beside `subscribe_events`;
      the "which two users to mint" section says the operator user is a
      DEDICATED admin user, because the log lane drops any record naming its
      own user and a token from the household owner's account silences every
      error that mentions them; the "What differs from the cluster lane"
      table gains a row saying records are observed by polling the log
      listing and that `system_log: fire_event: true` in Home Assistant's
      configuration only lowers latency. Verify with the site's lint
      (`docs/CLAUDE.md`) and by reading the rendered sections.
- [x] 4.5 Confirm the landing page, Introduction, Getting started,
      Installation and the guides say nothing about how the Home Assistant
      lane observes records (`grep -rn 'system_log\|fire_event' docs/`
      returns only the integration page), and run
      `python3 .github/scripts/docs-generate.py --check` from the worktree.
      Result: the integration page plus the two changelogs — this change's
      Unreleased entry and an archived 5.22–7.0 entry recording the lane's
      first release — and 49 generated files up to date.
