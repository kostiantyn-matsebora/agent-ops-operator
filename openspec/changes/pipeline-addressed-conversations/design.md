# pipeline-addressed-conversations — design

## Context

`capabilities-are-wiring` made the Pipeline the sole source of a conversation's capabilities. Two origination paths address a **profile** rather than a route and therefore have no Pipeline: `POST /task` without one, and `/<profile>` chat commands. Rather than change what they address, that change invented the capability-only Pipeline — sourceless, channelless, naming a profile — as a baseline for them.

Two things came out of that:

- The baseline is the profile's default capabilities in a different object. It was defended as "reviewable and overridable", but the honest description is a workaround for paths that address the wrong thing.
- It did not reach routing Pipelines at all. Both bundles render Pipelines that declare no capabilities, so in `master` today every signal-driven conversation dispatches `allowedTools: ""`.

`chat-signal-origination` (drafted) independently removes the second path: general-surface chat messages become signals routed by a claiming Pipeline, and it already notes that this removes the baseline's reason to exist for commands.

## Goals / Non-Goals

**Goals:**

- A conversation is addressed by the thing that initiates it: the Pipeline.
- No defaults, no inheritance, no per-profile fallback — a Pipeline's declared capabilities are its conversations' capabilities, full stop.
- Fix the regression: signal-driven conversations get capabilities again.
- Keep the demo working, by shipping a Pipeline to address rather than a baseline to inherit.

**Non-Goals:**

- Chat addressing. `chat-signal-origination` restructures origination on channels and owns `/<pipeline>` there; duplicating it here would conflict.
- No CRD changes. A sourceless, channelless Pipeline stays *legal* — it simply stops carrying special meaning.
- Not reintroducing capabilities on `AgentProfile`.

## Decisions

### D1: `POST /task` names a Pipeline, not a profile

```json
{"pipeline": "k8s-ops", "task": "why is pod X crashlooping?", "agent": "optional"}
```

The profile, channel set, and capabilities all come from the named Pipeline — one lookup, one source. The `profile` field is removed rather than deprecated: leaving it would mean supporting a request that cannot produce a capable agent, which is the failure this change exists to remove.

A request naming no Pipeline, or an unknown one, is rejected with 400/404. That is a deliberate trade: the previous shape accepted anything and produced a toolless conversation, which looked like success.

Rejected alternative — keep `profile` and resolve *some* Pipeline for it. That is the baseline again, with the same ambiguity (which of several routes?) and the same silent-default failure mode.

### D2: Delete the baseline, do not soften it

`CapabilityPipelineForProfile`, `IsCapabilityPipeline`, `ConditionBaselineConflict`, `baselineConflicts`, the k8s-bundle baseline template and its `profile.baseline.*` values all go. Keeping any of it as a fallback would preserve exactly the property being removed: a conversation whose capabilities do not come from the Pipeline that made it.

The `BaselineConflict` condition disappears with the concept. Nothing replaces it — with no per-profile default there is no duplicate to conflict over.

### D3: Every routing Pipeline declares its capabilities — including the chart's

There is no inheritance, so a Pipeline that declares nothing grants nothing. That is the model, and it is only safe if the shipped Pipelines are explicit. Both bundles gain bindings:

- k8s-bundle's `cluster-events` Pipeline binds the built-in toolsets (`grantShell` controls whether execution is among them).
- vm-bundle's default-source Pipeline binds the built-ins plus its own `vm-observability` toolset and MCPConfigs when those components are active.

A Pipeline that declares no capabilities gives its conversations none, and that is a legitimate configuration rather than a mistake to guard against — the operator said what this route may do, and the answer was nothing. No fallback, no inference, and no warning: an agent's abilities are declared explicitly, the same way an agent or skill definition declares its tools. Inference would mean the system granting something the operator did not write down, which is the property being removed.

The bundles are a separate matter. They ship an agent and therefore must declare what it can do; a bundle whose Pipeline granted nothing would be shipping a broken product, not exercising a configuration choice. That is why D3 obliges the CHART to be explicit while the model stays silent about it.

### D4: The chart ships addressable Pipelines

The demo needs something to address. k8s-bundle renders a Pipeline per agent — currently one, for `k8s-engineer` — carrying the built-in toolsets, under demo mode and behind `pipelines.create` so a production install can opt in or supply its own. The README's five-minute curl names it.

This is the same object the baseline was, minus the special meaning: it is addressable, it is a route, and it declares capabilities like any other. That is the point — one concept instead of two.

## Risks / Trade-offs

- [Breaking `POST /task` callers] → Unavoidable if the task lane is to address a route. The request gains one field and loses one; the README and NOTES change together, and a request in the old shape fails loudly rather than producing a toolless agent.
- [A Pipeline with no bindings grants nothing] → Intended. Capabilities are declared, never inferred; a route that declares nothing has nothing, and the system does not second-guess that.
- [Landing without `chat-signal-origination` leaves chat commands capability-less] → Named in the proposal; the two should land together, and the chat path is already capability-less for any profile without a baseline today.
- [Deleting a concept archived hours ago] → It was archived with a live regression and a workaround shape. Correcting it now costs one more change; leaving it costs the model.

## Open Questions

- None. An earlier draft asked whether a Pipeline declaring no capabilities should raise a warning condition; the answer is no (user direction 2026-08-08): a toolless agent is the operator's choice to make, and flagging it would reintroduce the system having an opinion about what a route ought to grant.
