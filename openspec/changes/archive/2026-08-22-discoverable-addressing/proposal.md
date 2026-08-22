# Discoverable addressing

## Why

Addressing works and nobody can find it. A person on a Telegram surface sees a
plain text box: nothing says that `/` starts anything, that a Pipeline can be
named, or that `/exit` and `/close` are the two words that end a conversation.
Discovery today is entirely REACTIVE and fires on exactly one path — a bare
message on a surface several Pipelines serve. On a single-pipeline surface a
person can use the system for months and never learn any of it is there.

The console is half-served: `NewConversation` has a real typeahead, and the
reply composer beside every conversation has nothing at all — the one place
`/exit` and `/close` do something is the one place that never mentions them.

The reason this cannot be fixed surface by surface is an access asymmetry. The
console reads Pipelines from its own watch. **A channel adapter is granted no
Kubernetes access** — `channel-telegram` cannot read a Pipeline and never will.
So the only component that can tell a surface what may be addressed is the
manager, and it currently tells them nothing.

Two things have to be corrected before that vocabulary is published, because
publishing them registers them permanently into the composer of every install.

**The listing command names the wrong thing.** `/agents` lists PIPELINES. That
word is already taken: an agent is a definition in `.claude/agents/` inside the
profile's repo. One word, two meanings, and the more visible one is wrong —
everything runs in a Pipeline, and a Pipeline is what a message addresses.

**The `/<pipeline>:<agent>` form lets a caller reach past its wiring.** A
Pipeline names one profile and a profile names one agent. A per-message override
lets whoever types it select an agent definition the wiring did not declare —
the same shape as the deleted `POST /task`, which `pipeline-model` already
forbids in so many words. It is documented nowhere, specified nowhere, and
advertised only in the listing reply it should never have been in.

## What Changes

- **The manager publishes a command vocabulary.** One list, namespace-wide,
  covering both halves of what a person may type: the built-in commands and
  every Ready Pipeline. Each entry carries a name, a description (a Pipeline's
  is its answering profile — no new CRD field), and the POSITION it is valid in:
  `general` (the surface a conversation starts from) or `thread` (inside one).

- **`/pipelines` replaces `/agents`.** `/agents` keeps working — it is a
  published word in installs already — but it is never offered, never
  registered, and never printed. The name people learn from now on is the one
  that matches what they get.

- **The addressed form becomes one segment.** `/<pipeline> <task>`, full stop.
  The `:<agent>` override is **REMOVED** from parsing, from the listing reply,
  and from what is written onto an input.

- **The manager does NOT pre-filter the vocabulary.** Which entries a surface
  can express is transport knowledge. Telegram's command names admit no hyphen,
  but that rule is Telegram's and stays inside `channel-telegram`.

- **Changes are pushed, not waited for.** The manager cannot dial an adapter —
  the contract is pull-only — so the vocabulary REVISION rides the ops long-poll
  the adapter already holds open, on both the `200` and the `204`. A changed
  revision means refetch. The revision is DERIVED from the published entries,
  never stored.

- **Telegram autocompletes commands AND pipelines.** Registering the built-ins
  makes Telegram render its own `/` control in the composer — a permanent
  affordance costing no message. Pipelines are registered too, under a
  transport-local spelling the adapter translates back on receipt, so a
  hyphenated Pipeline autocompletes without being renamed and without the
  manager ever seeing the alternate form.

- **Outbound messages may carry CHOICES.** `Message` gains a structured
  `choices` field beside `labels`. An adapter that can render controls does; one
  that cannot renders the same list as text. The ambiguity refusal earns most
  from it: the person has already typed their task, so one selection can send
  it rather than making them type it again.

- **Telegram handles selections.** The router's `allowed_updates` widens, under
  the same `is_topic_message` rule it already applies.

- **The console's reply composer gets the same vocabulary** — typeahead and a
  visible hint for `/exit` and `/close`, filtered to `position: thread`. Its
  `/api/agents` is reprojected onto the vocabulary, so the Telegram menu and the
  console typeahead cannot drift.

Not in scope: completion of anything inside a profile's repo. Nothing outside
the runtime holds the checkout.

## Capabilities

### New Capabilities

- `chat-command-vocabulary`: the manager-supplied list of what may be typed —
  built-ins and Ready Pipelines, each with a position — its derived revision,
  and the push that tells a surface to refetch. Covers what the manager states
  and what it deliberately leaves to the adapter.

### Modified Capabilities

- `pipeline-model`: the addressed form is a single segment naming a Pipeline,
  with no per-message agent override — a caller SHALL NOT select an agent
  definition the wiring did not declare. The listing is named for what it
  lists.
- `chat-addressing-discovery`: **"Discovery reads what the surface already
  knows"** required that discovery add no new operator endpoint. That holds only
  for the console. An adapter with no Kubernetes access cannot see a Pipeline,
  so the requirement is replaced: discovery adds no adapter PERMISSION and no
  CRD field, and what a surface cannot see for itself the manager supplies.
  Adds the entry-point requirement, the positional split, and offered choices as
  a discovery path. Renames the listing command.
- `channel-adapter-contract`: adds the vocabulary endpoint and the revision
  carried on the ops long-poll.
- `adapter-rendered-messages`: adds `choices` as a structured message field and
  `inReplyTo` as an opaque transport handle, plus the rule that an adapter
  without controls renders choices as a list.
- `telegram-channel-adapter`: registers built-ins and Pipelines per chat scope
  under a transport-local spelling it translates back, renders choices as inline
  controls, and handles selections.
- `telegram-ingest-router`: the selection update kind joins the forwarded kinds,
  under the existing classification rule.

## Impact

| Area | Change |
|---|---|
| `internal/addressing` | the `:<agent>` capture is removed; `Command.Agent` goes |
| `internal/chat` | vocabulary derivation + revision; `Message.Choices`, `Message.InReplyTo`; `/pipelines`; listing and refusal carry choices |
| `internal/dispatch` | the per-input agent override is no longer honoured; the profile's own agent is the only source |
| `internal/httpapi` | `GET /channel/vocabulary`; revision header on `/channel/ops` |
| `api/v1alpha1` | `InputItem.agent` deprecated — no longer written, read for one release so in-flight inputs still dispatch |
| `channel-telegram` | command registration per chat scope; the spelling map; inline controls; selection handling |
| `telegram-router` | `allowed_updates` widens; selections acknowledged and classified |
| `signal-telegram` | one opaque origin-message label |
| `console`, `console/ui` | `/api/agents` reprojected; reply-composer typeahead and hint; screenshots regenerated |
| Docs | `docs/concepts.md` (the addressed form), `docs/contracts.md`, `docs/telegram-bundle.md`, `docs/console-guide.md`, `CHANGELOG.md` |
| `CLAUDE.md` | the reserved command set gains `pipelines`; the `addressing/` map line loses `[:<agent>]` |

No chart change and no new adapter permission. The outbound contract version
stays at `2`: every wire addition is optional and an adapter that ignores all of
it behaves exactly as today.

Noted, not done: `openspec/specs/ha-bundle/spec.md` has one scenario saying
"chat listing of available agents". It is descriptive prose in an unrelated
bundle spec and is left for whoever next edits that capability.
