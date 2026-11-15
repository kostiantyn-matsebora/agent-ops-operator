## Context

See proposal.md for motivation. `signals/alertmanager` and `signals/pulse` are the nearest precedents for the HTTP server, auth and push. Both are vendor-specific. The contract (`docs/contracts/signal-adapter.md`) already fixes the signal fields, bounds and enums, so this adapter adds no contract surface.

## Goals / Non-Goals

**Goals:**
- Accept any authenticated HTTP request as one signal.
- Let the source owner map every normalized field without code.
- Stay stdlib-only, like every adapter module.

**Non-Goals:**
- No grouping, dedup or cooldown. The manager owns them (`ADP-3`, `MGR-2`).
- No signature verification (HMAC) in v1.
- No `chat` lane and no `reader`.
- No batch bodies. One request is one signal.
- No manager, CRD or contract change.

## Decisions

**D-1. Named for the transport, not "webhook".** A webhook implies a subscription the receiver registered. Here anything simply sends a request. Path `POST /request/{source}`, module `signals/http`, image `agentops-signal-http`. Alertmanager and Pulse keep `/webhook/`, which is accurate for them.

**D-2. `text/template` with two functions.** Stdlib, so the module stays dependency-free. `header` and `body` only. Every extra function would become contract. Alternative: JSONPath or a bespoke mini-language. Rejected because it adds a dialect to document and test.

**D-3. A constant is a template without actions.** One field type covers both. The constant case lets load-time validation reject a bad enum value before any request.

**D-4. Unresolved means the field's default, never an error.** A sender's missing header must not make a retry loop on a request that can never succeed. The exception `initiationId` falls back to a fresh id, which is the no-grouping default. Alternative: a 4xx on an unresolved reference. Rejected for the retry reason.

**D-5. Configuration faults go to the status endpoint, request misses do not.** A wrong template is the owner's fault and visible once, on the source. A missing header is a sender fact and per request.

**D-6. `kind` has no default and may be templated.** The adapter cannot infer the subject from the transport. Unset sends nothing and the manager reads `alert`. A templated `kind` lets a token holder pick the prompt, so the mandatory per-source token bounds it and the docs recommend a constant. Alternative: `kind` required (rejected, forces a choice the owner may not need) or defaulted from the id (rejected, a guess).

**D-9. Validate before push.** `API-6` and `API-7` refuse the whole batch for a bad `severity` or `signalId`. The adapter validates both itself and degrades the field, so one bad value never drops a signal.

**D-7. `signalId` is random unless mapped.** `ADP-9` wants it stable across a re-send, and the adapter cannot recognise a retry. A body hash would swallow legitimately repeated payloads. The page states "one request is one message" and recommends mapping the sender's delivery id.

**D-8. Mandatory per-source bearer, header only.** Reuses the `credentialsSecretRef` projection, so the manager still reads no Secret. No URL token (it lands in access logs). No HMAC in v1.

## Risks / Trade-offs

- [A templated `kind` lets a token holder choose the prompt] → mandatory per-source token, a constant recommended, the page says so.
- [An unset `kind` gives fresh-id requests the read-only investigation prompt] → the page tells owners to set `task`.
- [A mis-pointed `initiationId` template silently collapses or splits conversations] → load-time parse errors surface on the source, and the page shows worked examples.
- [A sender aimed at agent-ops' own events loops] → the adapter observes nothing, so `ADP-4` does not apply. The page warns the owner.
- [Random `signalId` duplicates on a sender retry] → map the delivery id.
