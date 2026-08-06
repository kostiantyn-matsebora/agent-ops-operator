# Design: make-signal-source-as-well-extendable-as-channel

## Context

The channel side has a complete extensibility stack (landed and live): open `type` + opaque `config` on the CR, a pull-based HTTP contract with master + HMAC-derived type-scoped tokens, `envFrom`-prefix credential projection, and a `ChannelAdapter` CRD whose reconciler owns the workload. Signal sources predate all of it:

- `SignalSourceSpec.Type` is a closed enum (`alertmanagerWebhook|cron|k8sEvents`); only Alertmanager is implemented. `CronSpec`/`EventsSpec` are dead typed sub-structs (`api/v1alpha1/signalsource_types.go:34-50`).
- Alertmanager parsing (`amAlert`, firing filter, title derivation) is compiled into `internal/httpapi/server.go:handleAlertmanager`; the actually-generic machinery — `ingest.Signature`/`SignatureHash`/`Cooldown` and `routeGroup` (window reuse, recurrence-on-session, `ConversationInput` payloads) — is entangled behind it.
- `spec.grouping` (signatureLabels/windowDays/cooldownHours) is already type-agnostic policy.

Binding constraints: manager performs zero Secret API reads; adapters need zero Kubernetes access; at-least-once semantics; existing `alertmanagerWebhook` CRs and Alertmanager webhook URLs must keep working with no migration (this change is compatible where the channel one was breaking — nothing forces a break here).

## Goals / Non-Goals

**Goals:**
- Plug a signal kind by applying CRs only: one `SignalAdapter` + N `SignalSource`s.
- Keep grouping/cooldown/window/recurrence manager-side for every source type — that's the operator's value, adapters only normalize.
- Same security model as channels: projected credentials, derived type-scoped tokens, zero-authority adapter pods.
- Prove the contract with a shipped reference adapter (cron).

**Non-Goals:**
- No change to Alertmanager ingest behavior or its URL (`/ingest/alertmanager/{source}`).
- No k8sEvents implementation (becomes "just another adapter" someone can build; the dead enum member goes away).
- No outbound operations to signal adapters (signals are one-directional; no ops queue).
- No unifying `ChannelAdapter`/`SignalAdapter` into one CRD (separate concerns, shared implementation helper instead).
- No per-signal routing overrides (channel/profile stay per-source metadata).

## Decisions

### D1: SignalSource splits like Channel did — but compatibly
```yaml
spec:
  type: pagerduty                  # open string (enum relaxed; MinLength=1, immutable via CEL)
  channelRef: {name: home-ops}     # typed, generic
  profileRef: {name: ha-engineer}  # typed, generic
  grouping: {signatureLabels: [...], windowDays: 7, cooldownHours: 6}   # typed, generic policy
  credentialsSecretRef: {name: pd-api-key}   # projected, never read
  config:                          # opaque: x-kubernetes-preserve-unknown-fields
    serviceIds: [...]
```
`type: alertmanagerWebhook` keeps its exact string as the built-in in-process type, so the live CR is untouched. `spec.cron`/`spec.events` are deleted (**BREAKING** on paper; both were unimplemented placeholders — no behavior existed to break; migration note says "cron is now the `signal-cron` adapter with `config: {schedule, input}`"). `type` gains CEL immutability to match Channel.

### D2: Normalized signal — the contract's core object
```json
POST /signal/inbound
{"source": "am-prod", "signals": [
  {"fingerprint": "abc123", "labels": {"alertname": "DiskFull", "namespace": "ha"},
   "title": "🔍 DiskFull — ha", "payload": "<raw JSON/text for the agent>", "kind": "alert"}
]}
```
The manager applies, per the source's `grouping`: fingerprint cooldown → signature from `labels` × `signatureLabels` → window reuse / create → alert vs recurrence input (session present) — i.e. exactly today's `routeGroup`, extracted to accept normalized signals from any caller. `kind: job` (optional, default `alert`) routes as an `InputJob` (task-lane template, not read-only investigation) — what cron ticks need; `title` overrides the built-in title derivation. Payload lands as a `ConversationInput` object as today.

*Alternative considered:* adapters compute signatures themselves — rejected: grouping policy belongs in the CR where operators can see and tune it; adapters normalize, the manager groups.

### D3: The rest of the contract mirrors channels, minus ops
- `GET  /signal/sources?type=<t>` → `[{name, config, credentialEnvPrefix}]`
- `GET/PUT /signal/state/{source}/{key}` → cursor state as SignalSource annotations (`agentops.dev/adapter-state-*`, same scheme)
- `POST /signal/sources/{name}/status` → Ready condition (config validation results)
- No long-poll ops endpoint: nothing flows manager→signal-adapter. Adapters that need pacing poll their own upstream; `/signal/inbound` is fire-when-you-have-something.

### D4: Auth — same scheme, distinct derivation context
`/signal/*` accepts the master token (full scope) or per-adapter derived tokens scoped to the SignalAdapter's `spec.type`. Derivation context is `"signal-adapter:"+name` (channel adapters use `"adapter:"+name`) so a `ChannelAdapter` and a `SignalAdapter` that happen to share a name can never share a token; each surface validates only against its own CRD list. 403 on cross-type, 401 otherwise — same constant-time, stateless re-derivation.

### D5: `SignalAdapter` CRD + reconciler, sharing the channel machinery
Spec/status shape is `ChannelAdapter`'s exactly (`type` immutable, `image`, non-secret `env`, `singleton` default true, `resources`; `Deployed`/`Ready`/`TypeConflict`, `servedSources`). The workload rendering (zero-RBAC SA, `automountServiceAccountToken: false`, derived token, `envFrom` prefix projection, singleton discipline, collision detection) is extracted from `ChannelAdapterReconciler` into a shared internal helper; both reconcilers become thin wrappers binding their CRD kind, their credential-bearing CR list (Channels vs SignalSources), their env var (`CHANNEL_TYPE` vs `SOURCE_TYPE`), and their token context. Projection prefix stays `AGENTOPS_CRED_<NAME>_` (no cross-pod collision — different pods).

### D6: Built-in registry, Served condition
`alertmanagerWebhook` is registered as the one in-process source type (a plain set in the manager — no provider interface needed, since built-ins are HTTP handlers, not op consumers). The SignalSource reconciliation (a small reconciler like `ChannelReconciler`) sets `Served`: True for built-ins, True when a Ready `SignalAdapter` claims the type or the source's own Ready condition was adapter-reported, False otherwise — same reasons vocabulary as channels.

### D7: `signal-cron/` reference adapter
Own module (precedent `channel-telegram/`), image `agentops-signal-cron`. Parses `config: {schedule (cron expr), input, title?}` per served source, reports invalid schedules via status, keeps `last-fire` in the state API (restart-safe, no double-fire within a tick), and posts `kind: job` signals with `fingerprint: <source>@<scheduled-time>` — the natural fingerprint makes at-least-once delivery idempotent under cooldown, and the stable signature (source name) groups recurring runs into one conversation with session context (a recurring job that *remembers*). Singleton via the CR default. Cron parsing: minimal five-field parser in-module (dependency-free rule).

## Risks / Trade-offs

- [Removing `cron`/`events` sub-structs is a schema break] → no implementation ever consumed them; CRD regen drops them; release note + the cron adapter as the replacement. Verified no live CR sets them before upgrade.
- [Shared helper refactor could disturb the live ChannelAdapter reconciler] → extraction is move-only; existing channeladapter envtests must pass unchanged (they pin the Deployment shape).
- [Alertmanager refactor onto the normalized core could shift ingest behavior] → dispatch/ingest fixtures and the alert-grouping integration test pin semantics; refactor is behavior-preserving by test.
- [Signal adapters can flood /signal/inbound] → cooldown + signature grouping already bound conversation creation; body size limits as elsewhere (1MB). Rate limiting deferred.
- [Two derivation contexts double the token list scan on auth] → trivial (lists are tiny, constant-time compares).
- [Cron in-module parser correctness] → five-field subset only, table-tested; anything fancier belongs to a real cron library in the adopter's own adapter.

## Migration Plan

1. Ship CRD updates (relaxed enum + new fields + `signaladapters` CRD), manager, chart minor bump. `helm upgrade` — existing `alertmanager` SignalSource and webhook URL work unchanged; nothing to migrate.
2. Optional adoption: apply a `SignalAdapter` (e.g. cron) + `SignalSource {type: cron, config: {...}}`.
3. Rollback: previous chart/manager — new CRD fields are ignored by the old manager; only sources using new adapter types stop being served.

## Open Questions

- Should `/signal/inbound` also accept a per-signal `channelRef`/`profileRef` override for multi-tenant fan-in sources? (Deferred — per-source metadata suffices for known cases.)
- `receivedTotal`/`lastReceived` bookkeeping for adapter-fed sources: manager-side on `/signal/inbound` (current plan) — confirm no double-count when built-in alertmanager path also updates it.
