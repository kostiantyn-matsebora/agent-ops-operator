## Why

`signal-ha`'s health predicate — "an integration whose config entry sits in
`setup_error`/`setup_retry` is still broken" — never fires for config-entry
setup-failure records, which is exactly the failure class it exists to catch.
The record's `integration` label is derived from the Home Assistant logger
name, and setup-failure messages are logged by the core logger
`homeassistant.config_entries` itself rather than under a
`homeassistant.components.<domain>` / `custom_components.<domain>` prefix — so
the label comes out as the literal string `"homeassistant.config_entries"`,
which never matches any real domain returned by `config_entries/get`. Every
such record therefore falls through to the recurrence-only rung, which fails
for a failure that logs once per retry attempt (retries are commonly minutes
apart, longer than the dwell).

Confirmed against the live install: every `homeassistant.config_entries` dwell
entry recorded by the deployed adapter since 2026-08-29 was dropped as quiet,
including a Tuya config entry that failed at boot (DNS resolution error) and
never recovered — 0 `tuya.*` entities existed hours later — with no signal
ever emitted.

## What Changes

- When normalizing a `homeassistant.config_entries` record, extract the real
  integration domain from the message text (the same message shapes the
  shipped rule regex already matches: `Error setting up entry <title> for
  <domain>`, `Setup failed for '<domain>': ...`, `Config entry ... for
  <domain> ... not ready yet` / `could not ...`) and use it as the
  `integration` label, so the record correctly keys into `config_entries/get`
  and the rung-1 health predicate can confirm or clear it.
- Fall back to the current behavior (logger name as the label) for any
  `homeassistant.config_entries` message shape that does not parse, so
  unrecognized cases are no worse than today.
- No change to the dwell/ladder mechanism itself, the rule vocabulary, or any
  other logger's label derivation.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `ha-signal-adapter`: the "Configuration reuses the cluster-events
  vocabulary exactly" requirement's re-check ladder promises the config-entry
  state as the health predicate "where one exists" — this tightens that
  promise so a config-entry setup-failure record (logged under
  `homeassistant.config_entries`) resolves to its real domain and therefore
  actually has a predicate, instead of silently falling to recurrence-only.

## Impact

- Code: `signals/ha/config.go` (`integrationOf` / `normalize`), plus its unit
  tests.
- No API, CRD, chart or contract change — this is adapter-internal
  normalization logic, invisible outside `signals/ha/`.
- Docs: none of `docs/concepts.md`, `docs/contracts.md`, the Home Assistant
  integration page, or the adopter site describe this internal label
  derivation today, and none becomes untrue — the adapter's documented
  behavior ("config-entry state as the health predicate where one exists")
  is unchanged; only the implementation now actually delivers it for this
  logger. `docs/integrations/home-assistant.md` is checked during
  implementation in case it names this specifically; if it does not, no site
  edit is owed.
