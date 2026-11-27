## Why

The testing station was agreed to run every tier inside the remote session and to loop on
its own results. Two things broke that. The system tier is dispatched to CI, because k3s
pods fail in the cloud sandbox, so the session is a wrapper around CI for its slowest tier.
And every round re-runs every tier in full, so a round that fixes one failing test costs
the whole suite again, more than an hour for the system pack.

## What Changes

- The system tier runs in the remote session. The session finds a way to start pods in the
  sandbox (the `oom_score_adj` failure) and the pack takes it as a default for that host.
  If the sandbox cannot run it, the station stops and names why. It never falls back to CI.
- The routine, `remote-session.md` and the `gotchas.md` row stop telling a session to
  dispatch `system-full.yml`.
- The loop is incremental. Round 1 runs each tier in full and measures coverage. Later rounds
  re-run only the tests that failed, and the tests the tester added, and merge their coverage
  into the round 1 profile. One full run of each tier closes the station, since a fix can
  break a test it never touched.
- The system pack can run a named set of lanes against a cluster that is already up.

## Capabilities

### New Capabilities
- `conveyor-testing-session-runs-all`: every tier runs in the session, and rounds re-run only what failed.

### Modified Capabilities

## Impact

- `.github/routines/test-change.md`, `.github/scripts/testing-station.py`, `coverage-diff.py` (profile merge).
- `platform/manager/test/system/` (cluster creation for the sandbox, lane selection).
- `.claude/rules/remote-session.md`, `.claude/rules/gotchas.md`, `docs/testing.md`.
