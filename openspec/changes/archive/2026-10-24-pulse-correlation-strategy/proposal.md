## Why

`signals/pulse/` derives `initiationId` from Pulse's alert `.ID` field alone
(`pulse:<resource>:<id>`).

Pulse's own documentation (`docs/WEBHOOKS.md` in `rcourtman/Pulse`) says
`.ID` "can be reused when the same condition fires again." It also says "a
later start with the same ID must be processed as a new occurrence."

ID-only correlation collapses genuinely distinct occurrences of a recurring
alert into one `initiationId`. That defeats the one-conversation-per-problem
intent of `signal-correlation-id` (#387): a Pulse alert that resolves and
later fires again as a new incident keeps landing in the same, possibly
stale, conversation.

## What Changes

- Add a second `initiationId` derivation strategy for `signals/pulse/` that
  folds Pulse's `StartTime` into the id (`pulse:<resource>:<id>:<startsAt>`),
  so a firing/resolved pair of one occurrence still shares an id, but a
  later re-fire with a new `StartTime` gets a fresh `initiationId` and opens
  a new conversation.
- Make the strategy a per-`SignalSource` config choice,
  `spec.config.correlationStrategy`, values `"id"` (today's behavior) or
  `"id+startTime"`. **BREAKING default**: absent/unset now means
  `"id+startTime"`, not today's implicit `"id"` behavior — an operator who
  wants the old collapsing behavior must set `"id"` explicitly.
  - The no-ID fallback path (hash of resource/type/severity/message, used
    only when Pulse sends no `.ID` at all) is unaffected by this choice —
    it has no id to correlate on in the first place.
- Update `openspec/specs/pulse-signal-adapter/spec.md`'s `initiationId`
  requirement to describe both strategies and the new default.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `pulse-signal-adapter`: the `initiationId` derivation requirement changes
  from "always the Pulse alert id" to "one of two sender-chosen strategies,
  selected by `spec.config.correlationStrategy`, defaulting to
  `id+startTime`."

## Impact

- Code: `signals/pulse/payload.go` (`initiationID()`), `signals/pulse/config.go`
  or equivalent (new field parsing/validation/defaulting), `signals/pulse/main.go`
  (wiring the source's config into the parser), plus their test files.
- Spec: `openspec/specs/pulse-signal-adapter/spec.md`.
- Reference docs: `docs/contracts/signal-adapter.md` (verify only —
  `initiationId` is already documented there as opaque per-adapter, so this
  is expected to need no edit, but must be checked since it is the law for
  this surface per `.claude/rules/signal-rules.md`), `docs/configuration.md`
  (new reversible chart/source value), `docs/integrations/pulse.md` if one
  exists (adopter-facing explanation of the correlation strategy choice).
  `docs/CHANGELOG.md` (breaking default change).
- Adopter site: whichever page documents configuring a Pulse `SignalSource`
  (e.g. a `docs/integrations/pulse.md` or `docs/guides/` page) needs the new
  `correlationStrategy` field and its default explained in prose, not just
  a generated values table.
- No manager-side change: the manager treats `initiationId` opaquely
  (`MGR-3`), so `platform/manager/` is untouched.
