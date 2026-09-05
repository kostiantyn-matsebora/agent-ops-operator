## Context

See `proposal.md` — Why. The relevant code:

- `signals/ha/config.go`'s `integrationOf(logger string) string` derives the
  `integration` label from the HA logger name via two known prefixes; any
  other logger (core loggers, including `homeassistant.config_entries`)
  returns the logger name itself.
- `signals/ha/config.go`'s `normalize(source string, rec *logRecord) Signal`
  calls `integrationOf` once and uses the result for both the `integration`
  label and the pending-queue grouping key (`pending.go`'s `Add`, keyed by
  `sig.Labels["integration"]`).
- `signals/ha/main.go`'s `health(ref recordRef, since time.Time) verdict`
  looks up `snap.entries[ref.integration]` — a map from HA domain (from
  `config_entries/get`) to that domain's entry states — to decide rung 1.

Home Assistant's own format strings for `homeassistant.config_entries` are (as
observed in the running instance and matched by the shipped rule regex):

- `Error setting up entry %s for %s` — title, then domain
- `Setup failed for '%s': %s` — domain in quotes, then error text
- `Config entry '%s' for %s integration not ready yet: %s; Retrying in %d
  seconds` — title, domain, error
- variants of "Config entry ... could not ..."

## Goals / Non-Goals

**Goals:**
- Make config-entry setup-failure records resolve to their real domain so the
  existing rung-1 health predicate can apply to them.
- Change nothing else about the ladder, the rule vocabulary, or any other
  logger's label.

**Non-Goals:**
- Guaranteeing extraction for every possible Home Assistant core message
  shape, present or future — only the shapes already recognized by the
  shipped default rules (`.claude/rules/signal-rules.md` scope /
  `chart/charts/home-assistant/`), with a safe fallback for anything else.
- Changing how `config_entries/get` itself is read, cached, or degraded when
  the token lacks admin rights — that behavior (rung 2 fallback with a logged
  reason) is correct and untouched.

## Decisions

**Extract the domain from the message text, at normalize time, scoped to the
`homeassistant.config_entries` logger only.**

Alternatives considered:

- *Match on config-entry Title instead of domain.* Rejected: Title is
  arbitrary user-facing text (an account email, a device name) with no fixed
  relationship to the domain string `config_entries/get` actually keys its
  results by, so matching on it would require the same live lookup this
  option was meant to avoid, and would still fail to disambiguate two
  integrations sharing a title convention.
- *Call `config_entries/get` inside `normalize()` to cross-reference domains
  against the message.* Rejected: normalization runs on every incoming
  record, including the high-volume core loggers; adding a live WebSocket
  round-trip there turns a pure function into one with I/O and latency on the
  hot path, and duplicates the snapshot `refreshSnapshot` already maintains
  for the dwell tick. It also does not help when the token lacks the admin
  scope `config_entries/get` needs — the message text is the only domain
  source available regardless.
- *Keep the logger name but special-case `health()` to treat
  `homeassistant.config_entries` members as always needing rung 2.* Rejected:
  this documents the bug rather than fixing it — it leaves the one predicate
  built for config-entry failures permanently unusable for the case it names
  in its own doc comment.

**A small ordered set of regexes, one per known message shape, tried in
order; first match wins; no match falls back to today's `integrationOf`
result (the logger name).** Anchored to the literal format-string structure
Home Assistant emits (a fixed prefix, `for`/`for '...'` before the domain,
end-of-token boundaries) rather than a loose "any word after for" pattern, so
a coincidental " for " inside unrelated free text does not misattribute a
domain.

**Scope: only the `homeassistant.config_entries` logger gets this
treatment.** Every other logger's `integrationOf` behavior — including the
`homeassistant.components.<domain>` / `custom_components.<domain>` prefix
stripping — is unchanged. Those loggers' names already carry the domain
correctly; this bug is specific to the one logger whose name never does.

## Risks / Trade-offs

- **[Risk] Home Assistant changes its `config_entries` message wording in a
  future core version, silently reverting to logger-name fallback.** →
  Mitigation: fallback is exactly today's behavior (rung 2, unchanged), so a
  wording change degrades gracefully rather than breaking; unit tests pin the
  currently-observed shapes so a future HA upgrade that changes wording is
  caught by a spec/test mismatch rather than by silence in production.
- **[Risk] A regex anchored too loosely misattributes a domain from
  unrelated text embedded in an error message (e.g. a URL or exception
  string containing "for X").** → Mitigation: anchor each pattern to the
  specific format-string shape (fixed literal segments either side of the
  domain capture), not a bare `for (\w+)`; the unit tests include a message
  carrying an embedded "for" inside an exception string as a negative case.

## Migration Plan

None required. This is adapter-internal normalization logic with no CRD, API,
or configuration-shape change — the next `signals/ha` image release picks up
the fix. No data migration, no backward-compatibility concern: a record
normalized under the old code path and one normalized under the new code path
carry the same fingerprint (`source@logger@location`, unaffected by this
change), so an in-flight dwell entry straddling a rolling restart still
coalesces correctly.
