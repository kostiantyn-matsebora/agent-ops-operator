## Context

`signals/pulse/payload.go`'s `initiationID()` always derives the id from
Pulse's alert `.ID` alone when present (`pulse:<resource>:<id>`), falling
back to a hash of `resource|resourceType|severity|message` only when Pulse
sends no `.ID`. See `proposal.md` for why ID-only is wrong per Pulse's own
docs.

The adapter already reads per-source config off `spec.config` as flat JSON
(`baseUrl`, `webhookUrl`, `webhookHeaders`) — no schema, polled via
`GET /signal/sources?adapter=`. `correlationStrategy` joins that same flat
shape.

## Goals / Non-Goals

**Goals:**
- Let an operator pick between the old, collapsing `"id"` strategy and the
  new `"id+startTime"` strategy, per `SignalSource`.
- Default to `"id+startTime"` so a fresh install gets correct
  one-conversation-per-occurrence behavior without extra config.
- Keep the no-ID fallback path untouched — it has no Pulse-supplied id to
  combine with a start time in the first place.

**Non-Goals:**
- Changing the no-ID fallback's own derivation (still
  `resource|resourceType|severity|message`, no time component). That hash
  already exists only because Pulse gave the adapter nothing stable to key
  on. Adding time there would need its own correctness argument this
  change does not make.
- Any manager-side change. `initiationId` is opaque to the manager
  (`MGR-3`). This is entirely adapter-local.
- Retroactively changing `initiationId` for conversations already open —
  a source flipping `correlationStrategy` only affects signals received
  after the flip.

## Decisions

- **Config field, not a build-time flag.** `correlationStrategy` lives on
  `spec.config`, the same place `baseUrl`/`webhookUrl`/`webhookHeaders`
  already live, read the same way (polled, schema-less JSON). A per-source
  choice is required because different Pulse deployments can have
  different `.ID` reuse behavior (e.g. older Pulse versions, or alert
  types whose IDs never get reused) — hardcoding one strategy adapter-wide
  would force operators who need `"id"` to lose correctness on every
  source.
- **`id+startTime` is the default.** Alternative considered: keep `"id"` as
  the default and require an explicit opt-in to the corrected behavior.
  Rejected — that would ship a change whose entire motivation is "the
  current default is wrong" while leaving the wrong behavior as the
  default. The proposal already marks this a breaking default change.
  An operator who has come to rely on same-id continuation across re-fires
  (unlikely, since that reliance would itself be on a documented Pulse
  quirk) sets `"id"` explicitly.
- **Unrecognized or empty `correlationStrategy` value defaults rather than
  rejects.** Mirrors the adapter's existing posture for a missing
  `webhookUrl` ("the same treatment a missing `baseUrl` gets" —
  `pulse-signal-adapter` spec). There is no HTTP request to return a 4xx
  against here — the value is read from a polled `SignalSource`, so a typo
  degrades to the safer default rather than silently adopting the old
  collapsing behavior or crashing the adapter.
- **The id construction is `pulse:<resource>:<id>:<startsAt>`, not a
  re-hash.** Keeps the existing `"id"` strategy's string shape as a
  prefix, so a source switching strategies produces a recognizably related
  but distinct id rather than an unrelated hash — useful when reading logs
  or `Conversation.spec.conversationId` during a migration.

## Risks / Trade-offs

- **[Risk]** `StartTime`'s precision. Pulse's docs note the primary
  `{{.StartTime}}` string has only whole-second precision, so two genuinely
  distinct occurrences starting in the same second could theoretically
  collide. **[Mitigation]** Combined with `.ID` and `.Resource`, a same-ID
  same-second distinct occurrence is already the edge case Pulse's own docs
  flag as not fully solvable ("do not assume the shorter string is
  universally unique") — out of scope to solve further here. `"id"` already
  had the much larger collision window this change is fixing.
- **[Risk]** A source already relying on the current (buggy) collapsing
  behavior changes behavior silently on upgrade, since the default flips.
  **[Mitigation]** `docs/CHANGELOG.md` documents the breaking default and
  the explicit `"id"` opt-in, per this repo's upgrade-notes convention.

## Migration Plan

1. Ship `correlationStrategy` support with the `"id+startTime"` default.
2. An operator who needs the old behavior on a specific source sets
   `spec.config.correlationStrategy: "id"` before or after upgrading —
   no ordering dependency, since the field is read per-signal.
3. No data migration: existing open conversations keep their
   already-assigned `conversationId`. Only signals received after the
   upgrade use the new derivation.
