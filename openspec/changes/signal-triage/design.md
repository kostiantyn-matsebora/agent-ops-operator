# Design: signal-triage

## Context

`routeSignals` in `internal/httpapi/signals.go` is the single funnel every signal passes through, regardless of source type. Its current decisions are:

```go
cd.Fresh(fingerprints)                      // exact-match cooldown, in-memory, per source
ingest.Signature(labels, signatureLabels)   // label-hash grouping key
// then routeSignalGroup:
list conversations by LabelSignatureHash    // window reuse (7d default)
  found  → append ConversationInput (InputRecurrence once a session exists)
  absent → CREATE Conversation
```

Both dedup mechanisms are syntactic. Neither has a ceiling. The `CREATE` branch is unbounded.

Three constraints shape everything below, and the third is the one that dictates the design rather than merely bounding it:

- **The manager is a singleton** under leader election, so in-memory state is acceptable — `Cooldown` already relies on this, tolerating one duplicate investigation across a restart.
- **`spec.config` is opaque; `spec.grouping` is typed manager-side policy.** Anything generic across source types belongs in `grouping`.
- **The manager reads no Secrets — zero Secret API reads.** It therefore cannot hold an LLM credential and cannot call a model. Every LLM call in this system happens inside a runtime pod, which receives credentials as `valueFrom` resolved by the kubelet. This is not a preference to trade off; it eliminates every design in which the manager "just asks a model".

## Goals / Non-Goals

**Goals**

- A deterministic ceiling on Conversation creation that works for every source type.
- Make dedup an explicit, replaceable manager-side decision rather than an emergent property of check ordering.
- Let an agent make the create/attach/drop judgment, opt-in, on existing execution machinery.
- Make the model the last and rarest step, never the first.
- Make every drop auditable.

**Non-Goals**

- Changing today's deterministic behavior. The default strategy is a restatement, not a revision.
- Adapter-side changes of any kind.
- A model call per signal.
- Cross-namespace or cross-cluster correlation.
- Replacing `smart-k8s-events`' adapter-local emit cap (harmless, differently scoped).

## Decisions

### Decision 1 — The cap is deterministic, always on, and independent

Everything else here can fail, be disabled, or be wrong. The cap is the floor that makes those failures survivable, so it must not depend on any of them.

Placement: `GroupingSpec`, alongside `cooldownHours` and `windowDays` — typed manager-side policy that applies to every source type. Accounting is in-memory per source, matching `Cooldown`'s precedent and its stated tolerance.

**Throttling caps CREATION, never attachment.** A throttled source keeps appending inputs to conversations that already exist. The failure mode being prevented is unbounded object creation, not the loss of information about a problem already under investigation — and inverting that would silence an ongoing incident precisely when it is escalating.

### Decision 2 — Dedup becomes a seam, and the default strategy is today's behavior

```
type Strategy interface {
    // Decide what to do with a signature group that has no live conversation.
    Decide(ctx, source, group, candidates) (Verdict, error)
}

Verdict = Create | Attach(conversationName) | Drop(reason)
```

The deterministic strategy returns `Create` unconditionally at that point — which is exactly what the code does today, so the default path is behavior-identical and the seam is provably free. Everything upstream of it (cooldown, signature, window reuse) is unchanged and stays outside the strategy: those are cheap and correct, and running them first is what keeps the expensive strategy rare.

`candidates` are the open conversations the strategy may attach to: live, within the window, in the same namespace. Supplying them is the manager's job because only the manager can see them — this is the concrete sense in which dedup is manager-side.

### Decision 3 — AI triage is a work unit on the `default` AgentRuntime

Forced by the no-Secrets invariant (Context). The `default` runtime is an established convention: `conversation_controller.go:294` resolves `profile.runtimeRef` → CR named `default` → bootstrap config, and `agentruntime_types.go:74` documents it as the namespace fallback.

**Triage is toolless.** It reads text and returns a verdict — no repository, no MCP servers, no `allowedTools`. Empty allowlist, `--permission-mode dontAsk`. It is the only agent in the system that provably cannot affect anything, which is what makes running it on arbitrary incoming signal text acceptable.

**Alternatives for hosting the unit:**

| host | reuses | cost |
|---|---|---|
| **reserved Conversation** (chosen) | pool, dispatch, `/work` long-poll, `/work/done`, `MAX_RUNTIMES` accounting, ownerRef GC — all of it | strictly serial per conversation, so triage is a queue under load |
| new dispatch lane + `agentops-triage` pod | isolation from the conversation pool | a second pod lifecycle, a second work contract consumer, new GC |
| manager calls a model directly | — | **impossible** — violates the no-Secrets invariant |

The reserved Conversation wins because it adds no new lifecycle. Its serialism is arguably correct (a triage decision should see the decisions before it), and the queue it forms is bounded by the timeout in Decision 5. It binds no channels — `channelRefs` empty — so nothing is posted anywhere; the verdict is read from `RunStatus.Result`.

Open: whether one triage Conversation serves the namespace or one serves each source. Per-namespace is cheaper and gives the agent a cross-source view (which is where semantic dedup earns its keep); per-source isolates a stuck lane. Leaning per-namespace, recorded as an Open Question.

### Decision 4 — The model is asked last, and only about new conversations

```
signal
  └─ self-exclusion            adapter (smart-k8s-events)
  └─ fingerprint cooldown      unchanged
  └─ signature grouping        unchanged
  └─ window reuse → ATTACH ────────────► returns here; no verdict, no cost
  └─ creation cap → THROTTLE ──────────► the floor; no verdict, no cost
  └─ STRATEGY → create | attach | drop
```

Cost scales with **novel problems**, not with event volume — the case that motivated this whole line of work (hundreds of events per rollout) never reaches the model, because those signals either collapse into an existing conversation or are throttled.

Verdicts cache by signature with a TTL, so a problem that flaps between novel signatures is asked once, not once per flap.

### Decision 5 — Fail open, with the cap as the backstop

Triage unavailable, timed out, unparsable, or naming a conversation that does not exist → **Create**. A missed incident is worse than a surplus one.

Failing open is only safe *because* Decision 1 is unconditional: an unavailable triage agent during a storm degrades to today's behavior with a ceiling, not to unbounded creation. This is the whole reason the cap is specified as independent rather than as one strategy among others.

The timeout must be short relative to how long an operator will tolerate not being told. Triage that takes longer than a minute has already failed at its job even if it eventually answers.

### Decision 6 — Drops are auditable or they are not allowed

An AI drop is the one outcome with no artifact — no Conversation, no input, nothing to `kubectl get`. Unaudited, it makes "why was I not paged" unanswerable, which would make the feature unsafe to enable.

Every verdict SHALL be recorded where an operator looks: recent verdicts on `SignalSourceStatus` (bounded ring — enough to answer the question, not an audit log), plus the reason text the agent gave. A drop with no recorded reason is a bug, not a terse verdict.

### Decision 7 — Triage never triages itself

The triage Conversation has a pod; the pod can fail; a failing pod emits events. That is the exact cycle `signal-self-exclusion` breaks, and it applies here unchanged (the pod is agent-ops-owned). Two rules on top:

- Signals whose subject is the triage lane SHALL NOT be triaged — they take the deterministic path.
- A triage failure SHALL NOT produce a signal. It is reported as a condition, per the same principle that governs the other change: **agent-ops' own health is status, not signal.**

## Risks / Trade-offs

- **[An LLM decides whether you get paged.]** The single largest risk in this change. → Off by default; drops are audited with reasons; fail-open on every error path; the deterministic cap is independent of it. An operator can enable triage, read a week of verdicts, and disable it having lost nothing.
- **[Prompt injection from signal content.]** Signal payloads are attacker-influenced in the general case (an event message, an alert annotation), and they are fed to a model that decides whether an incident is seen. → Triage is toolless, so injection cannot cause action — only a wrong verdict. Fail-open bounds the worst wrong verdict toward creating conversations. Verdicts naming a conversation outside the supplied candidate set are rejected.
- **[Triage becomes a bottleneck under storm.]** Strictly serial, and a storm is exactly when novel signatures arrive fastest. → Short timeout → fail open → the cap absorbs the rest. Worth measuring before the per-namespace/per-source question is settled.
- **[Cost is unpredictable.]** Novel-signature rate is not something an operator can estimate in advance. → Verdict caching, model-last ordering, and the cap all bound it; the values file should state the cost model plainly rather than leaving it to be discovered on a bill.
- **[In-memory accounting resets on restart.]** A manager restart resets both the cooldown window and the creation cap. → Matches the documented `Cooldown` tolerance; the exposure is one window's worth of extra creation, not unbounded.
- **[The seam could grow into a plugin system.]** → Two strategies, one interface, no registry. If a third arrives, revisit deliberately.

## Migration Plan

1. Cap and `Throttled` condition first, defaulted **off** (unset = no cap), so nothing changes for existing installs until a value is set.
2. Dedup seam second — behavior-identical by construction; verify by test, not by inspection.
3. Triage last, opt-in, with the chart shipping the toolless profile but not enabling it.
4. Rollback for each stage is independent: unset the cap, or disable triage; the deterministic path is what remains and it is today's path.

## Open Questions

- One triage Conversation per namespace, or per source? Per-namespace gives the cross-source view that makes semantic dedup valuable; per-source contains a stuck lane. Leaning per-namespace.
- Should the cap window be fixed (per hour) or configurable? Fixed is one fewer knob and one fewer way to misconfigure a safety floor.
- What does the agent see about candidates — titles and signatures only, or recent input payloads too? Payloads make the judgment better and the cost higher, and they widen the injection surface.
- Should a `drop` verdict still bump `receivedTotal`/`lastReceived` on the source? Arguably yes: the signal *was* received, and hiding it would make the source look idle during a storm the agent is suppressing.
- Does the console (`platform/console/`) need a triage-verdict view? A verdict record nobody reads is not really an audit.
