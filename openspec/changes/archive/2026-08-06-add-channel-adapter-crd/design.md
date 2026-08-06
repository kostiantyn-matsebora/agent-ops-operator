# Design: add-channel-adapter-crd

## Context

The landed architecture (archived `make-channel-type-architecture-extendable`) made channel types pluggable at the contract level: `spec.type` is an open routing key, the registry is empty by default, and any pod long-polling `/channel/ops?type=X` serves that type with zero operator code. What is *not* pluggable is the lifecycle: adapters are wired at the deployment layer (the chart even ships a Telegram-specific Deployment template), credentials are per-adapter-deployment env (one bot token per adapter — no multi-bot), auth is one shared token, and an unserved `type` fails silently. Precedent for the fix: Crossplane's `Provider` CR ("package image in, running controller out") and this repo's own `AgentRuntime` (implementation declared as a CR).

Binding constraints: the manager performs **zero Secret API reads**; adapters need **zero Kubernetes API access**; exactly one getUpdates consumer per bot token; contract is at-least-once with stable op ids; existing hand-deployed adapters must keep working.

## Goals / Non-Goals

**Goals:**
- Plug a channel type by applying CRs only: one `ChannelAdapter` (implementation) + N `Channel`s (surfaces).
- Clean implementation/usage split: credentials live on `Channel`, never on `ChannelAdapter`.
- Multi-credential support (several bots/workspaces of one type through one adapter).
- Per-adapter, type-scoped auth without introducing manager secret reads.
- Make "nobody serves this type" observable.

**Non-Goals:**
- No in-process plugin loading (Go plugins/wasm) — out-of-process stays the model.
- No config-schema admission validation (adapter-side validation + Ready condition remains; CR-carried schemas are future work).
- No change to op semantics, Router, dispatch, or the `/work` contract.
- The built-in `web` type (sibling change) stays a Registry built-in — in-process is deliberately special (it serves HTTP from the manager itself).
- No multi-namespace adapter federation.

## Decisions

### D1: `ChannelAdapter` is pure implementation
```yaml
apiVersion: agentops.dev/v1alpha1
kind: ChannelAdapter
metadata: {name: telegram}
spec:
  type: telegram            # immutable (CEL self == oldSelf); one adapter per type enforced by validation at reconcile
  image: kmatsebora/agentops-channel-telegram:0.2.0
  # non-secret tuning only; anything secret-shaped belongs on Channel.credentialsSecretRef
  env: []                   # e.g. LOG_LEVEL — reconciler rejects valueFrom.secretKeyRef here
  singleton: true           # default: replicas 1 + strategy Recreate (pull-based transports)
  resources: {...}
status:
  conditions: [...]         # Deployed, Ready (from the adapter's own contract reports)
  servedChannels: 3
```
Two adapters claiming one type would split the op queue randomly — the reconciler sets a `TypeConflict` condition on the newer one and does not deploy it.

*Alternative considered:* credentials on the adapter (v1 sketch) — rejected in review: a token is per-surface/per-bot (usage), not per-implementation; it also blocks multi-bot.

### D2: Credentials on `Channel`, materialized by projection
`ChannelSpec` gains `credentialsSecretRef` (LocalObjectReference — a secret *name*; typed metadata, NOT inside opaque `config`, because the reconciler must see it without interpreting the config). For every `Channel` of its type, the reconciler projects each key of that secret into the adapter pod spec:

```
envFrom:  prefix AGENTOPS_CRED_<CHANNEL>_ (upper-snake sanitized channel name)
          secretRef {name}                  ← kubelet injects EVERY key as <prefix><key>
```

> **Resolved at implementation (was: per-key `valueFrom`):** enumerating a Secret's keys to render per-key `valueFrom` entries would itself require a Secret read — exactly what the invariant forbids. `envFrom` with a prefix projects *all keys* (the design's chosen answer to open question 2) while the reconciler only ever handles the Secret *name*. Key case is preserved (`botToken` → `AGENTOPS_CRED_HOME_OPS_botToken`).

Nobody reads secret contents through the API: the operator writes *names* into a pod spec, the kubelet resolves, the adapter reads its own env. The contract's channel listing (D4) tells the adapter where to look. Channel add/remove/credential change → pod template hash changes → rollout (Recreate under singleton). Restart-on-channel-change is the accepted cost; chat transports tolerate it.

### D3: Per-adapter auth by derivation, not storage
Minting per-adapter token Secrets would eventually force the manager to *read* them back (restart). Instead: `token = base64(HMAC-SHA256(masterKey, "adapter:" + name))` where `masterKey` is the existing `ADAPTER_TOKEN` env. The reconciler (same process, same key) derives and injects it; the manager validates any presented token by re-derivation against the `ChannelAdapter` list and scopes it to that adapter's `spec.type` (its ops/state/status calls are rejected for other types). Stateless, no storage, survives restarts. The bare master token remains valid with full scope — hand-deployed adapters keep working unchanged.

### D4: Contract addition — credential env prefix in the channel listing
`GET /channel/channels?type=` entries gain:
```json
{"name": "home-ops", "config": {...}, "credentialEnvPrefix": "AGENTOPS_CRED_HOME_OPS_"}
```
> **Resolved at implementation (was: a `credentialEnv` key→var map):** the manager cannot enumerate Secret keys without reading the Secret, so the contract carries the deterministic *prefix* instead — derived purely from the Channel name via the same function the reconciler uses for projection. The adapter, which knows which keys it needs, resolves `os.Getenv(prefix + key)` (e.g. `prefix + "botToken"`).

### D5: Unserved types become visible
The Channel reconciliation (piggybacked on the existing flows) sets a `Served` condition: False when `spec.type` has neither a Registry entry nor a Ready `ChannelAdapter`. The ensure-topic path already surfaces per-conversation `TopicReady`; this adds the channel-level answer to "why is nothing happening".

### D6: Adapter pods run with zero ambient authority
Reconciler-created Deployments use a dedicated SA with no RBAC and `automountServiceAccountToken: false` — running arbitrary images named in CRs is acceptable because those pods hold only their projected transport credentials and the type-scoped contract token.

### D7: Chart ships CRs, not channel workloads
`telegram-adapter.yaml` (Deployment) is deleted; the same `telegramAdapter.*` values now render a `ChannelAdapter` CR (plus the new CRD in `chart/files/crds/`). Manager RBAC grows `deployments` and `serviceaccounts` (create/update/patch/watch, own namespace). Chart 1.1.0 — additive API, values-compatible.

### D8: Telegram adapter learns multi-credential
`channel-telegram` runs one getUpdates loop per **distinct token** among its served channels (single-consumer holds per token — the correct unit). Per-channel token from `credentialEnv[botToken]`; `TELEGRAM_BOT_TOKEN` env stays as the fallback for all channels lacking `credentialsSecretRef` (hand-deployed back-compat).

## Risks / Trade-offs

- [Upgrade double-poller window: helm deletes the old Deployment while the reconciler creates the new one] → documented sequence: upgrade with `telegramAdapter.enabled=false` in the same revision that installs the CRD, verify old Deployment gone, then apply/enable the `ChannelAdapter` CR. Brief 409s self-heal if the window is missed, but the ordered path avoids it.
- [Pod restarts on every Channel credential change] → inherent to projection; acceptable for chat; documented. Batched by normal reconcile debouncing.
- [Env-name collisions after sanitization (`home-ops` vs `home.ops`)] → reconciler detects collisions and reports a condition instead of silently overwriting.
- [HMAC master key rotation invalidates all derived tokens at once] → rotation = rolling restart of manager + reconciler-triggered adapter rollouts; document as the rotation procedure (it is the same blast radius the shared token has today).
- [Reconciler now runs images from CRs] → mitigated by D6 (no ambient authority); image trust is the cluster owner's policy question, same as `AgentRuntime` images today.
- [One adapter per type may not fit push-based scale-out transports] → `singleton: false` exists, but type-conflict rule still binds; revisit sharding when a real push transport lands.

## Migration Plan

1. Ship CRD + reconciler + chart 1.1.0. Upgrade with the telegram `ChannelAdapter` disabled → old chart's Deployment template is removed by helm (poller stops; getUpdates slot free).
2. Add `credentialsSecretRef: {name: agentops-telegram}` to the `home-ops` Channel; apply the `ChannelAdapter` CR (or re-enable the value) → reconciler deploys the adapter with the projected token.
3. Rollback: delete the `ChannelAdapter` CR (reconciler GCs the Deployment) and redeploy chart 1.0.x whose template restores the old wiring.

## Open Questions

- Should `ChannelAdapter.status` mirror the adapter's self-reported per-channel readiness (aggregated), or is the Channel-level Ready condition enough? *(Implemented: `Ready` reflects workload availability; per-channel readiness stays on each Channel's Ready condition.)*
- ~~`credentialEnv` key enumeration~~ **Resolved**: project all keys via `envFrom` prefix — no enumeration anywhere (see D2/D4).
