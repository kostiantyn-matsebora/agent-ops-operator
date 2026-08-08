## Context

Two mechanisms create Conversations today:

```
/signal/inbound ─▶ PipelineForSource (exclusive claim) ─▶ cooldown ─▶ signature
                   ─▶ window reuse ─▶ alert|job|recurrence ─▶ out-of-line payload
                   ─▶ status.receivedTotal / lastReceived

/channel/inbound ─▶ Router.HandleMessage ─▶ Create
                    known thread → reply | unknown thread → adoptThread
                    /<profile> → task    | bare text → PipelineForChannel(oldest Ready)
```

Only the first has claiming, dedup, grouping, or observability. The second
resolves "who answers" by creation timestamp — the very thing
`CapabilityPipelineForProfile` refuses to do forty lines away in the same file.

Facts that constrain the design:

| Fact | Consequence |
|---|---|
| Telegram serves one update stream per bot token; a second concurrent `getUpdates` returns `409`, and passing `offset` destructively confirms updates | Exactly one process may read a token's stream, ever |
| `getUpdates` and `setWebhook` are mutually exclusive, one webhook URL per bot | No webhook escape hatch for a second reader |
| `m.IsTopicMessage` already drives `threadID` in `channel-telegram/main.go:352` | Origination-vs-continuation is decidable **locally**, with no manager state |
| Adapter pods get credentials only by projection from a *served* CR's `credentialsSecretRef` | A component serving no CR has no way to receive the bot token |
| The operator grants adapters no RBAC, ever; `automountServiceAccountToken: false` by default | The router cannot persist its offset in a ConfigMap of its own |
| Adapter tokens are derived per adapter **name and kind** (`channel-adapter:` vs `signal-adapter:` contexts) and scoped to that name | One process posting to both contracts needs two scopes |
| The chart ships no channel-type workload templates — only adapter CRs | The router needs an adapter CR as its deployment vehicle |
| `SignalAdapter` has `spec.port` (reconciler owns a Service); `ChannelAdapter` does not | `channel-telegram` cannot receive forwarded updates without port parity |

Requester decisions carried in: chat origination becomes a signal source; the
Channel keeps everything after origination; three separate components; the
telegram-specific routing rule must not enter the manager; the chart delivers it
all under the existing `telegramAdapter.enabled` flag.

## Goals / Non-Goals

**Goals:**
- One origination rule: a Conversation starts from a claimed signal source.
- The manager gains no transport knowledge — it keeps seeing two generic
  contracts.
- The reply path is untouched, in behavior and in code.
- The single-`getUpdates`-consumer invariant holds through the split and through
  the migration.
- Each process has one job.

**Non-Goals:**
- Changing `POST /task` (a third origination path — see Open Questions).
- Porting other channel types, or generalizing the router beyond Telegram.
- Any change to the ops queue, delivery, mirroring, or thread bindings.
- Chat dedup as a feature — cooldown defaults to off for chat.

## Decisions

### D1. Classification lives in the router, and it is a local decision

The router is the only component that knows what a Telegram topic means:

```
update ─┬─ m.IsTopicMessage == false  ─▶ general surface ─▶ origination ─▶ signal-telegram
        └─ m.IsTopicMessage == true   ─▶ topic           ─▶ continuation ─▶ channel-telegram
```

No manager round-trip, no shared state. This is why the split is possible at
all: earlier framings assumed the adapter could not distinguish the two, which
would have forced the decision into the manager and made it transport-aware.

### D2. The router forwards raw updates and holds no channel config

chatId matching and approver filtering already live in `channel-telegram`'s
`dispatch()` and stay there, duplicated into `signal-telegram`. The router
therefore needs no `GET /channel/channels` read and no config of its own —
it needs the bot token and nothing else. Keeping it configuration-free is what
keeps a third container from becoming a third place to misconfigure a channel.

### D3. Offset persistence is delegated, not owned

The router must own the *offset value* (it is the thing calling `getUpdates`),
but it must not own *storage* — it has no RBAC and no CR of its own to annotate.
It therefore reads the offset once at startup and reports each confirmed offset
downstream, where `channel-telegram` persists it through the existing
`GET/PUT /channel/state/{channel}/{key}` it already uses today. No new
persistence mechanism, and the annotation keeps living on the Channel where the
migration guidance already tells operators to preserve it.

Trade-off: an extra hop on the write path, and an offset that can lag by one
batch on a crash. `getUpdates` is at-least-once by design and the manager
deduplicates on fingerprint, so a replayed batch is harmless for signals; for
forwarded topic messages it means a possible duplicate reply input, which is the
same exposure the current adapter has on restart.

### D4. `channel-telegram` gains a listen port; `ChannelAdapter` gains `spec.port`

`signal-vmalertmanager` already establishes the pattern — a `SignalAdapter` with
`spec.port` whose reconciler owns the Service and injects `LISTEN_ADDR`.
`ChannelAdapter` needs the same field for `channel-telegram` to receive
forwarded updates. This is parity, not a new concept, and it reuses
`adapterworkload.go` machinery wholesale.

### D4b. The router is its own container; the bot credential is shared with the channel

**Settled (requester decision).** The router stays a separate component and a
separate container. Its only job is deciding where a user's update goes —
topic present → `channel-telegram`, absent → `signal-telegram` — forwarding the
raw update either way.

The bot credential is a **shared Secret**: the same Secret backs both the
router's vehicle and the `Channel`, because both talk to Telegram (the router
calls `getUpdates`, the channel adapter calls `sendMessage`/`createForumTopic`).
`signal-telegram` holds **no credentials at all** — it never contacts Telegram,
it only normalizes forwarded updates and posts `/signal/inbound`.

Vehicle, given that credentials reach a pod only by projection from a served CR:
a `SignalAdapter telegram-router` serving a credential-carrying `SignalSource`
whose `credentialsSecretRef` names the same Secret as the `Channel`. That source
emits no signals; it is the projection anchor. The sample Pipeline claims it
alongside the chat source so it does not sit at `Wired=False` forever.

Rejected: collapsing the router into `signal-telegram` (the router is its own
component); giving `signal-telegram` the token (it needs none); putting a
credential field on an adapter CR (breaches "adapter CRs carry no credentials").

Consequence for D3: unchanged — the router posts no content contract and needs
no manager token of its own, so offset persistence stays delegated to
`channel-telegram`.

### D5. The manager's chat signal carries its originating surface in reserved labels

`signal-telegram` posts a normalized signal with:

```json
{"fingerprint": "tg-<update_id>",
 "labels": {"agentops.dev/channel": "telegram-ops", "agentops.dev/sender": "…"},
 "kind": "chat", "payload": "<text>"}
```

`agentops.dev/channel` is what lets the manager answer a command
(`/agents`, unknown agent, usage error) on the surface it came from, via the
ops queue, without creating a Conversation. Labels are already opaque strings in
the contract, so this adds a convention rather than a schema — other chat
adapters adopt the same two keys and get the same behavior.

### D6. `kind: chat` is a distinct lane, not `job` or `alert`

A chat origination is a task from a human: it takes the task-lane prompt, and it
must not be grouped with anything. So the chat lane pins `cooldownHours: 0` and
skips signature grouping entirely — every general-surface message opens its own
conversation, which is exactly today's behavior. Grouping and window reuse stay
available to chat sources that opt in, but the default preserves what users
already experience.

Rejected: reusing `kind: job`. Job carries recurrence-on-session semantics that
would make a second question resume the first question's session.

### D7. Origination-only paths are deleted from the router package, not disabled

`adoptThread`, the bare-text branch, the `/<profile>` branch, and
`defaultPipeline`/`PipelineForChannel` are removed outright. Leaving them behind
a flag would leave the timestamp tiebreak in the codebase as a live fallback,
which is the thing this change exists to delete. `/channel/inbound` keeps the
reply branch and starts rejecting a missing `threadId` with a message naming the
signal path.

### D8. Everything renders under `telegramAdapter.enabled`, as adapter CRs

One flag, three CRs — `ChannelAdapter telegram`, `SignalAdapter telegram`, and
the router's vehicle — plus a sample Pipeline pairing the chat `SignalSource`
with the `Channel`. The existing requirement that the chart ships no
channel-type workload templates is preserved: the reconcilers own all three
workloads. A partial enable (channel without ingest) is not offered — it would
be a chat surface that cannot be talked to.

## Risks / Trade-offs

- [Two consumers of one bot token during migration] → The migration sequences
  old-adapter shutdown before router start, exactly as chart 1.0 and 1.1 did.
  Restated as a requirement and covered by the documented steps, since this is
  the failure that costs the most debugging.
- [Three containers for one chat surface] → Accepted deliberately. Mitigated by
  D2 keeping the router configuration-free, so the operational surface grows by
  one image and one CR, not by a third place to configure Telegram.
- [Offset lag on router crash replays a batch] → Harmless for signals
  (fingerprint dedup); for topic messages it matches today's restart exposure.
- [Duplicated chatId/approver filtering in two adapters] → Real duplication,
  chosen over giving the router config. Both are ~15 lines against the same
  contract listing; drift shows up as messages silently ignored, so both get the
  same test.
- [`/channel/inbound` becomes reply-only, breaking third-party adapters that
  post bare messages] → Breaking, and named as such. The rejection message names
  the signal path, so the failure is self-explaining rather than silent.
- [Commands answered without a conversation depend on a label convention] →
  A chat signal missing `agentops.dev/channel` cannot be answered at all. The
  manager rejects such a signal at `/signal/inbound` rather than accepting it
  and dropping the reply.

## Migration Plan

1. Ship `ChannelAdapter.spec.port` and the reconciler Service parity first —
   inert until used.
2. Publish `telegram-router` and `signal-telegram` images; add the chat
   `SignalSource` and claim it in the existing Pipeline. Old adapter still
   polling, nothing changed behaviorally.
3. **Scale the old adapter to zero**, confirm no `getUpdates` consumer remains,
   then enable the new stack. This is the one step where ordering matters.
4. Carry the `agentops.dev/adapter-state-*` offset annotation across so the new
   poller does not re-read old updates.
5. Remove the origination paths from the manager and delete
   `PipelineForChannel`.

Rollback: re-enable the old single-container adapter and revert the manager —
the origination paths must come back together with it, so steps 3 and 5 are the
rollback boundary. Conversations already created are unaffected either way.

## Open Questions

- ~~The router's deployment vehicle~~ — **settled, see D4b**: separate
  container, `SignalAdapter` vehicle with a credential-carrying `SignalSource`
  sharing the `Channel`'s Secret, `signal-telegram` credential-free. No
  invariant moves.
- Whether `POST /task` also becomes a signal origination. It is the last path
  that creates a Conversation without a source, and `capabilities-are-wiring`
  is already reasoning about it from the capability side.
- Whether the two reserved labels belong in the signal contract as typed fields
  once a second chat adapter exists.
