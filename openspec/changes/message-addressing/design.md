# Design: message-addressing

## Context

- Every entry into the manager is typed: `/signal/inbound` takes a payload
  string from a signal adapter authenticated per source's adapter,
  `/channel/inbound` takes `{channel, threadId, text}` authenticated per
  channel's adapter. Tokens are derived per adapter name; nothing holds a
  token for a surface it does not serve.
- The bare-chat lane answers several claimants with a tap menu, and
  `signals/telegram` reads the answer back through Telegram's reply chain.
  That is addressing by FORM; this change adds addressing by WORDS.
- `coordinated-agents` D-F/D-I give the read: `mcp-aops` forwards a
  `channel-reader:<channel>` token and the manager answers the brief
  projection, no verbs.
- `runtimes/ollama` holds the `chatter` interface — one file knows the vendor.

## Goals / Non-Goals

**Goals:**
- One synchronous contract, one decision per call, questions as decisions.
- The analyzer holds no credential; the caller's identity delivers.
- Latency in the low seconds on a CPU-only Ollama, so a spoken exchange works.

**Non-Goals:**
- Routing typed messages on Telegram or the console through it (a surface may
  adopt it later; nothing here requires it).
- Translation, or any rewriting of the instruction beyond removing the
  addressing.
- Durable dialogue state.

## Decisions

### D-A — The decision is the response; the caller delivers

- `POST /utterance` → `{decision, ...}` within the request. No callback URL,
  no job queue: a decision is one model turn, and the caller is waiting
  anyway to speak or display the result.
- The caller posts the decision to the manager under its own tokens
  (signal identity for `originate`, channel identity for `reply` and
  `command`). The analyzer never learns those tokens.
- Alternative rejected: the analyzer as the signal adapter of every source it
  routes for. It would then hold a channel token per surface for the reply
  path, and be the one component that can post as everyone.

### D-B — The reader token is the caller's, forwarded

- The request carries the caller's `channel-reader:<surface>` token; the
  analyzer forwards it to `mcp-aops` `list_conversations` and holds nothing.
  Same thin-forwarder shape as `mcp-aops` itself (coordinated-agents D-F).
- The chart renders that token into each calling adapter beside its other
  derived tokens.

### D-C — Pending intents are in-memory, per `(surface, speaker)`, TTL'd

- One pending intent per key: the candidates offered and the instruction
  held. Default window 120 s (`ANALYZER_ASK_TTL`). Replaced by a newer
  question, dropped on expiry, gone on restart.
- Alternative rejected: persisting them. A question answered after a restart
  minutes later is worse than a fresh start, and there is nothing in the
  Kubernetes API this state could be derived from.

### D-D — The model is asked for structure, and the structure is validated

- The prompt carries the Ready Pipelines/Coordinators the surface's sources
  are claimed by (from `mcp-aops` `list_agents`/pipelines), the brief
  projection, the pending intent if any, and the utterance; it asks for one
  JSON object matching the decision schema.
- The analyzer validates every name in the answer against what it listed; an
  unlisted name becomes `ask`, never a decision. The model proposes, the list
  disposes.
- Backends: `ollama.go`, `openai.go`; interface `chatter` copied from
  `runtimes/ollama` rather than shared — one module, zero requires, per
  `structure.md`.

### D-E — Language stays on the utterance

- `lang` in, `lang` out on every `ask`; the prompt instructs the model to ask
  in that language. The analyzer stores no language per speaker beyond the
  pending intent.

## Risks / Trade-offs

- **A wrong resolution posts to the wrong conversation.** Mitigated by the
  list validation (never an unknown name), by `ask` on any tie, and by
  `/close` existing. Not eliminated — it is a judgement.
- **CPU model latency.** A 3B-class model answers a short structured prompt in
  1–3 s on the reference VM; the prompt is kept small (briefs, not
  transcripts) for this reason as much as for reach.
- **Prompt injection through a brief.** Briefs are agent-written. The
  validation step is the bound: the worst a brief can do is steer a choice
  among listed names or force an `ask`.

## Migration Plan

1. `coordinated-agents` merged and released (`mcp-aops`, reach class, `brief`).
2. `helm upgrade` with `analyzer.enabled: true`. Nothing calls it until a
   surface is wired to; `voice-conversations` is the first.

Rollback: disable; no state, no CRD.

## Open Questions

- Default model for the reference install — the smallest that resolves the
  fixture set reliably; measured in 3.4.
