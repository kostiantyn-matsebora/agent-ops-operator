# Design: wire-it-up

## Context

All pipeline elements are extensible CRs, but the wiring between them is implicit and single-lane:

- Routing metadata is scattered: `SignalSource.spec.channelRef`/`profileRef` (one channel per source), `Channel.spec.defaultProfileRef`, `AgentProfile.spec.runtimeRef`.
- `Conversation.spec.channelRef` is a single optional ref; `status.threadId` a single opaque string. The Router (`internal/chat/router.go`) creates conversations bound to one channel; `convByThread` resolves inbound by thread id; the reconciler enqueues one `ensure-topic` op (`topic:<conv>` stable id) and the dispatch gate waits for the one thread id.
- Outbound agent replies today: `delivery.mode: agent` channels have the agent post directly (the live `home-ops` Telegram does this); `result` mode leaves the answer in `status.runs[].result` — **the manager never fans a reply out to a chat surface**.
- Ops carry `Channel` and complete per-op; sends are fire-and-forget; everything is at-least-once with stable ids.

Binding constraints: strictly serial per conversation; at-least-once ops; manager reads no secrets; adapters unchanged (the `/channel/*` contract must not change shape — adapters like `channel-telegram` 0.2.0 must keep working as deployed).

## Goals / Non-Goals

**Goals:**
- One declared object answering "what feeds what, who answers, and where is it visible".
- N sources × M channels: every signal → one conversation visible on every bound channel; chat-started conversations equally mirrored (Telegram shows the web chat's conversations and vice versa).
- Zero adapter changes — mirroring is composed entirely from existing ops (`ensure-topic`, `send`).
- Existing single-channel setups keep working untouched until a Pipeline is applied.

**Non-Goals:**
- No per-source or per-channel overrides inside a Pipeline (uniform fan-out; overrides are future work).
- No selective mirroring (all bound channels get everything).
- No cross-channel identity (relayed messages are attributed text, not impersonation).
- No runtime binding on the Pipeline — runtime selection stays `profile.runtimeRef → "default"` (the Pipeline binds a profile; the profile already binds its runtime).
- No changes to the `/channel/*` or `/signal/*` adapter contracts.

## Decisions

### D1: Pipeline is pure wiring
```yaml
apiVersion: agentops.dev/v1alpha1
kind: Pipeline
spec:
  signalSourceRefs: [{name: alertmanager}, {name: daily-healthcheck}]
  channelRefs: [{name: home-ops}, {name: web}]
  profileRef: {name: ha-engineer}        # the pipeline's agent; sources' own profileRef is ignored inside a pipeline
status:
  conditions: [Ready, SourceConflict]
```
Resolution rule, applied at signal-routing and chat-inbound time: **pipeline first, source/channel-level refs as fallback**. A `SignalSource` referenced by a Ready Pipeline routes to that Pipeline's channels + profile; an inbound message on a Channel referenced by a Pipeline starts conversations bound to all the Pipeline's channels (the pipeline's profile is the default for bare messages; `/profile` commands still pick any profile). Sources/channels referenced by no Pipeline behave exactly as today.

*Alternative considered:* keeping per-source refs and adding `mirrorChannelRefs` on Channel — rejected: the binding belongs to neither end, that's the point of the Pipeline.

### D2: One pipeline per source, guarded like adapter types
Two Pipelines claiming one source would double-route every signal. The Pipeline reconciler applies the adapter-style guard: oldest claimant wins, the newer gets `SourceConflict=True` and its claim on the contested source is inert. Channels MAY appear in several Pipelines (a channel is a surface, not a stream); the conversation's binding set comes from the pipeline that *originates* it.

### D3: Conversations go multi-channel — **BREAKING**
`spec.channelRef *ObjectRef` → `spec.channelRefs []ObjectRef`; `status.threadId *string` → `status.threads []ThreadBinding{channel, threadId}`. Everything per-channel:
- **Topic ensure**: the reconciler enqueues one `ensure-topic` per bound channel lacking a binding; op id becomes `topic:<conv>:<channel>` (still stable/dedupable); completion writes the `{channel, threadId}` binding (the op already carries its Channel).
- **Inbound resolution**: `convByThread(channel, threadId)` matches bindings pairwise — thread ids stay opaque strings scoped per channel (unchanged capability).
- **Dispatch gate**: a channel-bound conversation dispatches once **at least one** binding exists (waiting for all would deadlock on one broken channel); channels whose topics land later just start receiving from that point. `WorkUnit.ThreadID` carries the binding of the single channel when the conversation is single-channel-agent-mode; empty otherwise.
- Migration: live ACTIVE conversations with the old fields lose their chat binding on upgrade (fields removed); recipe = one-line `kubectl patch` per active conversation or simply reply in the topic (re-adoption creates a fresh binding). Idle/completed conversations need nothing.

*Alternative considered:* keeping single `channelRef` + a parallel mirror list — two code paths through router/reconciler/dispatch forever; rejected.

### D4: Mirroring = manager-composed ops; adapters untouched
- **Agent replies**: on `POST /work/done`, when the conversation is multi-channel, the manager fans the result out as `send` ops (one per bound channel, to that channel's thread). This is the first manager-driven reply path — composed from the existing op pipeline, so external adapters and in-process providers need nothing new.
- **Acks**: router acks go to every bound channel (the originating channel keeps today's wording; siblings receive the same ack).
- **User-message relay**: an inbound message on channel A is relayed to channels B… as a `send` with attribution (`💬 <channel>: <text>`). No loop risk: bots don't receive their own posts (Telegram getUpdates semantics), and future in-process providers must follow the same rule (noted in the web-chat rebase).
- Multi-channel conversations **force `result` delivery** in dispatch (the agent's printed answer is the deliverable; the manager owns distribution). Per-channel `delivery.mode: agent` keeps meaning only for single-channel conversations — unchanged live behavior for `home-ops` until it joins a Pipeline. A multi-channel fan-out message is formatted once (chat HTML subset) and sent verbatim to all channels.

### D5: Reply fan-out formatting
`/work/done` results are plain text today (2000-char cap). Fan-out sends the result as-is (chat HTML subset expected per format.md; adapters already HTML-escape-tolerate via Telegram parse mode). The result cap stays; the web-chat change's cap raise composes later. Empty/failed results fan out a short status line (`❌ run failed (<status>)`) so silence never looks like success on mirrored surfaces.

### D6: PipelineReconciler — validation and visibility only
Watches Pipelines (+ referenced kinds mapped back): validates every ref exists, sets `Ready` (all refs resolve, no conflict) and `SourceConflict`. It does NOT create anything — routing reads Pipelines at decision time (cached client, tiny lists). Printcolumns: sources/channels/profile counts for `kubectl get pipelines`.

### D7: Entry-point integration
- Signal routing (`routeSignalGroup`): resolve source → Pipeline; if found, conversation gets pipeline channels + pipeline profile (else today's source-level refs).
- Router inbound: channel → Pipeline (a channel in several pipelines: inbound *originating* on it uses the OLDEST Ready pipeline referencing it — deterministic; documented) for new-conversation binding + default profile; replies resolve by thread binding regardless of pipelines.
- `POST /task`: optional `pipeline` field → bound to its channels + its profile as default when `profile` omitted... `profile` stays required (explicit is better); pipeline supplies channels only.
- SignalSource/Channel `Served` conditions unchanged (orthogonal).

## Risks / Trade-offs

- [Conversation CRD break on a live install] → pre-1.0 policy; only ACTIVE chat-bound conversations are affected; documented patch/re-adopt recipe; ships in the same release train as everything else this week.
- [Fan-out send storms (N channels × chatty conversation)] → sends are cheap ops; N is small (2-3 surfaces); at-least-once dup risk unchanged (stable send ids per enqueue).
- [Relay echo loops if a future in-process provider re-ingests bot posts] → contract note: providers/adapters MUST NOT feed their own outbound posts back as inbound; web-chat rebase carries the note.
- [First-binding dispatch gate means an early reply may miss a slow channel's topic] → the miss window is one ensure-topic round-trip; subsequent messages land; accepted over deadlock-on-broken-channel.
- [Pipeline-first resolution surprises a source with its own channelRef] → Ready Pipeline wins is a single documented rule; `kubectl get pipeline` shows the claim, and the source's old refs are simply inert while claimed.
- [Result-mode forcing changes reply formatting for telegram-in-pipeline] → the agent's printed answer already follows the same format spec templates; acceptable, documented.

## Migration Plan

1. Ship CRDs (pipeline + conversation change), manager 0.6.0, chart 1.3.0. Upgrade: nothing changes behaviorally — no Pipeline CRs exist; single-channel flows use the fallback path (source-level refs), identical semantics on the new fields (`channelRefs` with one entry).
2. Migrate active conversations: patch `spec.channelRef`→`channelRefs[0]` + `status.threadId`→`threads[0]` (script in release notes), or let users re-adopt by replying in the topic.
3. Opt into mirroring: apply a Pipeline CR binding the existing source(s) + channels. Rollback: delete the Pipeline (new conversations fall back to source-level routing; existing multi-channel conversations keep their bindings).

## Open Questions

- Should the attributed relay include sender identity when adapters supply it (`inbound.sender` is already in the contract but unused)? Leaning yes-when-present (`💬 telegram/@user:`), trivial to add during implementation.
- `/work/done` fan-out of intermediate acks vs final results only — v1 fans out final results only; live verification will show whether "working…" progress markers are wanted on mirrored surfaces.
