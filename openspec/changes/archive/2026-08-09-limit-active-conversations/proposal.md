## Why

A burst of signals can open conversations faster than they finish, and nothing
in the system bounds how many run at once at a level an operator recognises:
the only cap is `MAX_RUNTIMES` (default 8), named after pods rather than
conversations, and an idle runtime pod holds its slot for ten minutes after the
agent has stopped working. Conversations also never end — there is no way for a
person to say "this one is done", so threads accumulate and capacity is
reclaimed only by eviction.

## What Changes

- Cap **simultaneously active conversations** — a conversation is active while
  it holds a runtime pod — at a configurable limit, **default 5** (down from
  the current pod-pool default of 8). The cap is expressed and reported in
  conversation terms, not pod terms.
- **Queue rather than drop**: a conversation that cannot be admitted is created
  in a new `Pending` phase — no runtime pod, no chat topic, no dispatch — and is
  admitted FIFO as slots free. The Conversation CR *is* the queue, so the
  backlog survives manager restarts with no new CRD.
- **Release slots faster**: the runtime idle TTL default drops from 10 minutes
  to **1 minute** (`runtimeIdleTtlMinutes` / `RUNTIME_IDLE_TTL_M`, still
  overridable per `AgentRuntime`). An idle conversation returns to `Idle` when
  its pod exits and stops counting against the cap; it resumes on the next
  input.
- **`/close` ends a conversation**: sent in a conversation's thread it deletes
  the Conversation CR (ownerRef GC takes the runtime pod and MCP ConfigMap) and
  **archives the chat topic** on every bound channel.
- **New `close-topic` outbound op** on the channel adapter contract, implemented
  by `channel-telegram` (`closeForumTopic`) and by the console (thread marked
  closed). **BREAKING** for third-party adapters only in the sense that an
  adapter which ignores the new kind leaves topics open; the op is failed
  gracefully, not retried forever.
- Chart exposes the cap as `maxActiveConversations` (default 5), with
  `maxRuntimes` accepted as a deprecated alias for one release.

## Capabilities

### New Capabilities
- `conversation-capacity`: how many conversations may run at once, what
  "active" means, how over-cap conversations queue in `Pending` and get
  admitted, and how idle conversations release capacity.
- `conversation-close`: ending a conversation from chat — the `/close` command,
  CR deletion and its GC consequences, and topic archiving across every bound
  channel.

### Modified Capabilities
- `channel-adapter-contract`: outbound operation kinds gain `close-topic`
  alongside `ensure-topic` and `send`, with its completion and failure
  semantics.
- `telegram-channel-adapter`: the reference adapter serves `close-topic` by
  closing the forum topic.
- `multi-channel-conversations`: closing a conversation archives the thread on
  every bound channel, not only the one the command arrived on.

## Impact

- **Manager**: `internal/controller/conversation_controller.go` (admission gate,
  `Pending` phase, topic/pod suppression while pending, FIFO admission),
  `internal/chat/router.go` (`/close` interception on the reply path),
  `internal/chat/ops.go` (`OpCloseTopic`), `cmd/manager/main.go`
  (`MAX_ACTIVE_CONVERSATIONS` with `MAX_RUNTIMES` fallback).
- **API**: `api/v1alpha1/conversation_types.go` gains the `Pending` phase —
  deepcopy and CRD regeneration required.
- **Adapters**: `channel-telegram/` and `console/` handle the new op kind.
- **Chart**: `values.yaml` (`maxActiveConversations: 5`,
  `runtimeIdleTtlMinutes: 1`), `templates/deployment.yaml`.
- **Docs**: `docs/contracts.md` (op kinds), `README.md` / `docs/concepts.md`
  (capacity model and `/close`), `CLAUDE.md` map notes.
- **Tests**: `internal/integration/` (admission under cap, pending→active
  promotion, close deletes + archives), `internal/chat` unit tests.
