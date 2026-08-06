# Proposal: wire-it-up

## Why

Every element of the pipeline is now a first-class, extensible definition — `SignalSource` (what fires), `Channel` (where humans talk), `AgentProfile` (who answers), `AgentRuntime` (what executes) — but nothing *binds* them: routing is scattered across per-source `channelRef`/`profileRef` and per-channel `defaultProfileRef`, every conversation is welded to at most ONE channel, and there is no way to say "these sources feed these agents, visible on all of these surfaces". A `Pipeline` CRD makes the wiring itself a declared object — and makes conversations multi-channel, so Telegram and the web chat (and anything else bound) each carry the full conversation.

## What Changes

- New **`Pipeline` CRD**: `signalSourceRefs[]` × `channelRefs[]` + `profileRef` — every referenced source's signals become conversations bound to **all** referenced channels; conversations started from any bound channel are likewise bound to all of them. A source belongs to at most one pipeline (conflict surfaces as a condition, adapter-style); source-level `channelRef`/`profileRef` remain the fallback for sources outside any pipeline.
- **BREAKING** — Conversations become multi-channel: `spec.channelRef` → `spec.channelRefs[]`, `status.threadId` → `status.threads[]` (`{channel, threadId}` bindings). Topic creation, inbound thread resolution, and the dispatch gate become per-channel; the runtime env keeps a thread id only for single-channel agent-direct delivery.
- **Full mirroring** across a conversation's channels: agent replies are fanned out by the **manager** (from the `/work/done` result) as `send` ops to every bound channel; router acks go to every channel; a user message arriving on one channel is relayed to the sibling channels as an attributed message ("channels fully repeat the whole conversation").
- Multi-channel conversations force `delivery: result` (manager-captured fan-out) — per-channel `delivery.mode: agent` applies only to single-channel conversations, where behavior is unchanged.
- `POST /task` gains an optional `pipeline` field (conversation bound to the pipeline's channels + profile); alert ingest resolves source → pipeline before falling back to source-level refs.
- New `PipelineReconciler`: reference validation, source-conflict guard, Ready condition; chart ships the CRD + RBAC.

## Capabilities

### New Capabilities

- `pipeline-model`: the Pipeline CRD — shape, resolution rules (pipeline-first, source-level fallback), one-pipeline-per-source guard, Ready/conflict conditions.
- `multi-channel-conversations`: the multi-channel Conversation model — per-channel thread bindings, per-channel topic ensure, inbound from any bound channel, reply/ack fan-out, attributed cross-channel relay, forced result delivery.

### Modified Capabilities

- `signal-adapter-contract`: the inbound-routing requirement resolves conversation binding pipeline-first (Pipeline channels + profile when the source is claimed; source-level refs as fallback).
- `channel-type-model`: delivery-instruction selection gains the multi-channel rule (result mode forced; `agent` mode is single-channel only).

## Impact

- `api/v1alpha1/`: new `pipeline_types.go`; `conversation_types.go` channel/thread model (**BREAKING** — live active conversations need a one-line migration or re-adoption); regenerated deepcopy/CRDs.
- `internal/chat/`: Router (multi-channel create/adopt/resolve, relay, ack fan-out), OpQueue (per-channel ensure-topic ids + thread-binding completion).
- `internal/controller/`: Conversation reconciler per-channel topic flow; new `PipelineReconciler`.
- `internal/httpapi/`: dispatch gate + delivery forcing, `/work/done` reply fan-out, `/task` pipeline field, signal routing via pipeline; `internal/dispatch/` WorkUnit thread selection.
- `chart/`: `pipelines` CRD + RBAC; chart 1.3.0, manager 0.6.0; samples get a Pipeline wiring the existing pieces.
- Pending `add-web-chat-channel` change: needs a rebase note (conversation listing/transcript per channel binding instead of single `channelRef`).
- Live install: `home-ops` keeps working single-channel with zero changes until a Pipeline CR is applied; active multi-channel adoption is opt-in.
