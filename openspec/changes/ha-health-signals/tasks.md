## 1. Configuration

- [x] 1.1 In `signals/ha/config.go`, add the `surfaces` block — four objects,
      each with `enabled` and its knob (`states`, `severities`,
      `deviceClasses`), defaults as `design.md` states, each list validated
      against Home Assistant's vocabulary — and compile it into `filter`.
      Verify with `go build && go vet` in `signals/ha/` inside the build
      container, run against THIS WORKTREE's path (`docker exec -w "$PWD"`),
      and with config tests for the defaults, a disabled surface, and an
      unknown key or state being refused with the key named.

## 2. Reads

- [x] 2.1 In `signals/ha/ha.go`, add `Repairs` (`repairs/list_issues`),
      `States` (`get_states`, decoded to entity id, state, attributes) and
      `EntityRegistry` (`config/entity_registry/list`, decoded to entity id
      and platform), and extend `configEntry` with `Reason`. Verify with
      session tests against the fake, which learns all three commands.
- [x] 2.2 In `signals/ha/main.go`, read repairs in the existing sweep beside
      the listing and config entries — only when the repair surface is
      enabled — and add a `runStatesLoop` per session at `statesEvery` (one
      minute) that reads states, and the registry on connect and every fifth
      read, only when the sensor or update surface is enabled. Verify by
      reading the diff that a disabled surface issues no command, and by a
      test counting the fake's calls with every surface off.

## 3. Records and the standing set

- [x] 3.1 Add a normaliser per surface producing a `logRecord` shaped for it
      (name, message, level, source location, timestamp) plus the `surface`
      and `integration` labels, and label every log record `surface=log`.
      Verify with unit tests per surface asserting fingerprint, labels, title
      and payload against the spec's table.
- [x] 3.2 Add the standing set per source and surface: a read computes the
      current set, feeds conditions entering it to `consider`, forgets those
      leaving it. Verify with tests: a condition is posted once while it
      stands, again after it left and came back, and a restart (a fresh
      adapter over the same fake) re-posts it.
- [x] 3.3 The update digest: one record per source, fingerprint on the source,
      payload listing every pending update, re-fed when a new entity joins
      the set and not when one leaves. Verify with a test adding, then
      removing, then adding a pending update.
- [x] 3.4 Extend `health` in `signals/ha/main.go` so a config-entry, repair or
      sensor member is unhealthy while its condition is in the current set
      and healthy once it is absent, and apply self-exclusion's marker and
      own-user mechanisms to every kind. Verify with the dwell tests in 5.2
      and a self-exclusion test over a repair naming agent-ops.

## 4. Chart

- [x] 4.1 In `chart/charts/home-assistant/values.yaml`, add
      `logsAdapter.source.surfaces` with the defaults, the four per-surface
      rules ahead of the log rules (config entries and sensors `for: 5m`,
      repairs and updates `for: "0"`), and bump the pinned `signal-ha` tag;
      in `templates/logs.yaml`, render the block into the source config and
      declare every key in `configSchema`. Verify with the chart render tests
      in `platform/manager/internal/integration/` — default render, a surface
      switched off, rule order — run from the worktree.

## 5. Unit tests

- [x] 5.1 Extend `signals/ha/fakeha_test.go` to serve `repairs/list_issues`,
      `get_states` and `config/entity_registry/list` from settable fixtures,
      and run the whole module with `go test -count=3 -race ./...` in the
      container from the worktree. Done; the fake also counts calls per
      command, which is what proves a disabled surface reads nothing.
- [x] 5.2 End-to-end dwell tests per surface against the fake: a config entry
      in `setup_retry` that stays is posted at the close with its reason; one
      that loads before the close is dropped; a `problem` sensor that clears
      is dropped; a repair is posted with its placeholders; the digest is off
      by default and one signal when on. Verify all pass and the module's
      coverage does not drop below where it stands on master.
      Done as `surfaces_test.go`: config defaults, switches and knobs, six
      refused shapes; a failed entry reported once with its reason and again
      after recovery, with the log cursor untouched; a repair with its
      placeholders and a warning-severity one not; two faulting sensors with
      platform and domain grouping; the digest off by default, one when on,
      re-posted on growth and not on shrink; four surfaces off issue no
      reads; the config-entry dwell reporting the stuck entry and dropping
      the one that loaded; a repair naming agent-ops excluded.

## 6. E2E tests

- [x] 6.1 Not applicable — nothing here is decided by a cluster. Every surface
      is a Home Assistant WebSocket command answered by the fake, and the
      chart's rendering is decided by the render tests, per `docs/testing.md`.

## 7. Documentation

### Reference docs

- [x] 7.1 `docs/CHANGELOG.md` — an Unreleased entry naming the new `signal-ha`
      tag, the four surfaces, their defaults and the `surfaces` values.
- [x] 7.2 `.claude/rules/structure.md` — the `signals/ha/` entry says what it
      reads; it now names the four surfaces beside the log, and the two
      cadences.
      `.claude/rules/chart.md` — the bundle section's "the log ingest lane"
      bullet names the surfaces and their switches. Verify by reading both
      back.

### The adopter site

- [x] 7.3 `docs/integrations/home-assistant.md` — the opening paragraph and the
      "What you get" row stop saying "log"; a recipe block under "Tune what
      reaches you" shows each surface's switch and knob, a rule dropping one
      surface, and enabling the digest; the "What differs from the cluster
      lane" table gains the surfaces row; the diagram's caption says it shows
      one of the surfaces, the log included. Verify by reading the rendered
      sections and with
      `python3 .github/scripts/docs-generate.py --check` from the worktree.
      The diagram itself is kept as the design says; its caption now names
      the other surfaces as taking the same path.
- [x] 7.4 Confirm the landing page, Introduction, Getting started and
      Installation name the integration by link only
      (`grep -rn -i 'home assistant' docs/index.md docs/introduction.md
      docs/getting-started.md docs/installation.md`) and need no change.
