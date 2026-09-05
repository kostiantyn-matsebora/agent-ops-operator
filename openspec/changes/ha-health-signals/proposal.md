## Why

**The Home Assistant lane watches the log and nothing else, and Home Assistant
reports most of what is wrong with a house somewhere other than the log.** On
the reference install, on 2026-09-05, four things stood that the adapter could
not see:

| What Home Assistant knew | Where it said so | What the adapter saw |
|---|---|---|
| the `fully_kiosk` integration cannot reach its tablet and is in `setup_retry` | config entry state, with the reason | nothing — a retrying entry logs one line at startup, then nothing |
| an automation calls `browser_mod.more_info`, a service that no longer exists, since 2026-07-08 | a repair issue, severity error, marked fixable | nothing, for two months |
| a Tion device reports a warn-out through its `problem` binary sensor | entity state | nothing |
| seven HACS packages have a newer version | `update.*` entities in state `on` | nothing |

The log is one of five surfaces the instance reports on, and the one that
misses a whole class: an
integration that fails quietly, a fault a device reports as a state, a problem
Home Assistant has already diagnosed and filed as a repair. A person opening
Settings sees all four as badges. The lane that exists to spare them that
reads the one place none of the four is written.

## What Changes

- **The adapter observes four more surfaces, as four record kinds beside log
  records**, on the same served `SignalSource`, through the same rules
  vocabulary, the same dwell ladder and the same self-exclusion. Every signal
  gains a `surface` label — `log`, `config-entry`, `repair`, `sensor`,
  `update` — which is how a rule selects one:
  1. **Config entry state.** An entry entering a failed state (`setup_retry`,
     `setup_error`, `migration_error` by default) is one signal per entry,
     fingerprinted on domain and entry id, carrying the entry's own reason as
     its message.
  2. **Repairs.** One signal per issue from the issue registry, fingerprinted
     on domain and issue id, severity mapped to level, carrying the issue's
     placeholders (the automation, the missing service, the edit link).
  3. **Problem and connectivity sensors.** A `binary_sensor` of device class
     `problem` turning on, or `connectivity` turning off, is one signal per
     entity, labelled with the integration that owns it.
  4. **Pending updates.** ONE digest signal listing every `update.*` entity in
     state `on`, re-posted when a new one joins the set. **Off by default.**
- **Each surface is configurable, in the source config and as chart values.**
  `surfaces.<name>.enabled` per surface (the first three default on, updates
  off), plus the knob that makes sense for each: the config entry states that
  count, the repair severities that count, the sensor device classes watched.
  A disabled surface issues no reads at all.
- **The bundle ships default rules per surface**, ahead of the log rules:
  config entries and sensors dwell (Home Assistant's restart churns both),
  repairs and the update digest do not (a repair is a standing fact).
- **A standing condition is reported when it appears and again only after it
  went away and came back.** The manager's fingerprint cooldown and signature
  window absorb what an adapter restart re-reports.
- **Explicitly NOT observed, and why:** entity availability (272 of 703
  entities on the reference install are unavailable, nearly all from disabled
  integrations and powered-off devices, and an integration whose entities all
  drop is one that failed, which kind 1 catches), battery levels and
  persistent notifications (a threshold rule and a duplicate of repairs; each
  is a later rule on this source, not a record kind), disabled devices (a
  choice, not an incident).
- Not breaking. The adapter's name, the `SignalSource`, its config keys and
  the chart's `logsAdapter` values path all stay; the new keys have defaults.
  Existing log signals gain the `surface=log` label, which no shipped grouping
  or rule reads.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `ha-signal-adapter`: gains the requirement that the adapter observes the
  four health surfaces beside the log, each selectable by a `surface` label,
  each enable-able and tunable, standing conditions reported once per
  standing period, and the dwell re-check consulting each surface's own
  current truth.
- `ha-bundle`: gains the requirement that the bundle exposes each surface's
  switch and knobs as values under the source, with the stated defaults, and
  ships default rules per surface ahead of the log rules.

## Impact

**Code**

- `signals/ha/` — a `surfaces` config block (`config.go`, validated like the
  rest); a states read and an entity-registry read on their own cadence beside
  the listing sweep (`ha.go`, `main.go`); a normaliser per surface producing
  the same `Signal` shape with a `surface` label; standing-set bookkeeping per
  surface and source; the health ladder extended so a config-entry, repair or
  sensor condition is re-checked against its own surface at the close
  (`pending.go`, `main.go`); self-exclusion applied to every kind
  (`selfexclude.go`). Tests per kind against the fake Home Assistant, which
  learns `repairs/list_issues`, `get_states` and the entity registry.
- `chart/charts/home-assistant/values.yaml` and `templates/logs.yaml` — the
  `surfaces` values, the default per-surface rules, the `configSchema`
  entries, and the pinned `signal-ha` tag.

**Reference docs**

- `docs/CHANGELOG.md` — the entry.
- `.claude/rules/structure.md` — the `signals/ha/` entry says the adapter
  reads the log; it reads the four surfaces beside it now.
- `.claude/rules/chart.md` — the bundle section's "the log ingest lane" bullet.
- `docs/configuration.md` is NOT affected: the values are a subchart's, so
  they belong on the integration page.

**The adopter site**

- `docs/integrations/home-assistant.md` — the opening ("Home Assistant logs
  its own failures... this bundle reads them"), the "What you get" row "Log
  failures that reach you", a new recipe block under "Tune what reaches you"
  for the surfaces and their switches, the "What differs from the cluster
  lane" table, and the diagram's caption. The diagram itself
  (`assets/img/integrations/home-assistant-*.svg`) draws a log record as the
  example; it stays, with a caption saying the other surfaces take the same
  path — see
  the design's risks.
- The landing page, Introduction, Getting started and Installation name the
  integration by link only and describe nothing this changes. Checked.

**Not affected**

- The manager, the CRDs, the signal contract, the log rules that ship today,
  the cursor and its state key, `signal-k8s-events`.
