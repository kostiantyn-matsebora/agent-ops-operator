# Proposal: signal-triage

## Why

Every signal that survives its adapter's filters becomes a Conversation if its signature is new. The manager's only defenses are exact-fingerprint cooldown and label-hash signature grouping — both purely syntactic. Three gaps follow, and none of them belong in an adapter:

1. **No ceiling.** Nothing caps how many Conversations a source may create. A runaway — a self-referencing loop, a cluster-wide incident, an agent whose own remediation provokes the events that wake it — fills etcd with Conversations and ConversationInputs while `MAX_RUNTIMES` throttles only the pod pool. Every adapter-side mitigation is per-adapter and per-implementation; the ceiling has to be generic.
2. **Dedup is syntactic.** `OOMKilled on api-server` and `Unhealthy on api-server` are one incident, and no label hash can know that. Two alerts from different sources about the same outage are two conversations. The signature is a hash, so *similar* is indistinguishable from *unrelated*.
3. **Importance is a static rule.** Adapters can drop by matcher, but "is this worth waking an agent for, given what is already open" is a judgment, not a predicate.

Deduplication is manager-side by design — the manager is the only component that sees every source, every open Conversation, and the window they live in. This change makes that responsibility explicit and complete, and adds an **optional** layer where the judgment itself is made by an agent.

## What Changes

### A hard ceiling on conversation creation (deterministic, always on)

- `SignalSource.spec.grouping` gains a cap on **new Conversation creation** per source per window. Signals that would exceed it do not create; the source reports a `Throttled` condition naming the count and the window, and continues attaching to conversations that already exist.
- The cap is deterministic and independent of every other feature here. It is the floor that makes the rest safe to fail.

### Deduplication becomes a first-class manager concern

- The dedup decision moves behind one explicit seam in `internal/ingest`, rather than being implied by the order of checks in `routeSignals`. Today's behavior — fingerprint cooldown, then signature grouping, then window reuse — becomes the default *deterministic* strategy, unchanged in effect.
- That seam is what makes an alternative strategy possible without touching the routing code, and it is where the AI verdict below plugs in.

### Optional AI triage on the default AgentRuntime

- A source may opt in to **agent-decided triage**. For a signal that would otherwise create a NEW conversation, the manager asks an agent for a verdict: **create**, **attach to an existing conversation**, or **drop with a reason**. That single verdict covers semantic dedup and importance filtering together.
- The agent runs on the **`default` AgentRuntime** — the existing namespace fallback (`profile.runtimeRef` → CR named `default` → bootstrap config). Nothing new executes anywhere: triage is an ordinary work unit on the machinery that already exists.
- This is forced by an invariant, not chosen for convenience: **the manager reads no Secrets and holds no LLM credentials.** It cannot call a model. Anything that needs a model must be a work unit dispatched to a runtime pod, which receives credentials as `valueFrom`. The architecture leaves exactly one shape available.
- Triage needs **no tools**. It reads text and returns a verdict, so it runs with an empty allowlist under `--permission-mode dontAsk` — the cheapest possible agent, and the only agent in the system that provably cannot touch anything.
- **Off by default.**

### Ordering: the model is asked last, and rarely

```
signal
  └─ self-exclusion            (adapter — smart-k8s-events)
  └─ fingerprint cooldown      deterministic, unchanged
  └─ signature grouping        deterministic, unchanged
  └─ window reuse → ATTACH ────────────► no verdict needed, no cost
  └─ creation cap → THROTTLE ──────────► the floor
  └─ AI triage (opt-in) ──► create | attach | drop
```

The model is consulted **only for signals that would open a new conversation**, which bounds cost to novel problems rather than to event volume. Verdicts are cached by signature for a TTL so a flapping new problem is not re-asked.

### Failing open, and being auditable about it

- Triage unavailable, timed out, or returning an unparsable verdict SHALL **create the conversation**. A missed incident is worse than a surplus one, and the deterministic cap underneath means failing open cannot become unbounded.
- Every AI **drop** is recorded with its reason where an operator can find it. A silently dropped incident is the one outcome this change must never produce — "why was I not paged" has to have an answer.

### Loop safety

- Triage runs as an agent, so it has a pod, so its pod can fail — which is the very cycle `signal-self-exclusion` exists to break. Triage SHALL never triage its own lane, and a triage failure SHALL never produce a signal.

## Capabilities

### New Capabilities

- `signal-creation-cap`: the deterministic ceiling — per-source Conversation-creation limit, `Throttled` condition, and the rule that throttling never blocks attachment to conversations that already exist.
- `signal-dedup-strategy`: dedup as an explicit manager-side seam — the default deterministic strategy (today's behavior, restated normatively) and the contract an alternative strategy must satisfy.
- `ai-signal-triage`: the opt-in agent verdict — the `create`/`attach`/`drop` contract, execution on the `default` AgentRuntime as a toolless work unit, ordering after every deterministic check, verdict caching, fail-open posture, auditability of drops, and self-exclusion of the triage lane.

### Modified Capabilities

- `signal-source-model`: `spec.grouping` gains the creation cap and the triage opt-in; `status` gains the `Throttled` condition and the record of recent triage verdicts.

## Impact

- `api/v1alpha1/signalsource_types.go`: `GroupingSpec` gains the cap and triage settings; `SignalSourceStatus` gains the condition and verdict record (+ deepcopy/CRD regen).
- `internal/ingest/`: the dedup seam and the creation-rate accounting (in-memory, like `Cooldown` — the manager is a singleton under leader election, and a restart costs at most one duplicate investigation, which matches the existing tolerance).
- `internal/httpapi/signals.go`: `routeSignals`/`routeSignalGroup` consult the cap and, when enabled, the verdict, before creating.
- `internal/dispatch/`: a triage lane template (prompt + expected verdict shape), toolless.
- `internal/controller/`: hosting the triage work unit and reaping it (shape decided in design.md).
- `chart/`: an opt-in triage profile, values gating, and documentation of the cost model.
- Docs: `docs/concepts.md` (cap, dedup, triage), `docs/contracts.md` (the verdict contract), `CHANGELOG.md` if any default changes.
- Depends on nothing in `smart-k8s-events` and conflicts with nothing in it. That change's adapter-local emit cap is a stopgap for one adapter; this cap is generic and supersedes its role without requiring its removal.
