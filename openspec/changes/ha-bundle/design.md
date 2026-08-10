# Design: ha-bundle

## Context

The previous version of this change is superseded twice over: it declares `allowedTools` and `mcp.configRefs` on the `AgentProfile` (both removed by `capabilities-are-wiring` — capabilities are wiring now, carried by the Pipeline), and it assumes `POST /task` as an origination doorway (deleted, not renamed; a Pipeline is REACHED, never NAMED over HTTP). Its single `ha-engineer` profile also models one job where a Home Assistant install has two.

What the codebase gives us to build on:

- `k8s-bundle` is the reference domain bundle: self-gated components, a risk-split pair of `MCPToolset`s, an `MCPConfig` with a fixed server key, and an identity-only profile with an inline `systemPrompt` because it has no repository. This bundle is the second instance of that pattern, and the pattern is what makes it cheap.
- `signal-k8s-events` is the reference observing adapter: `rules` (Prometheus vocabulary — matchers plus `for:` dwell) and `route` (Alertmanager vocabulary — inhibition), cursor via `/signal/state`, and three independent self-exclusion mechanisms.
- `Router.HandleCommand` resolves `/<pipeline> <task>` against ANY Pipeline by name — the addressed pipeline need not claim the surface's source — and `boundChannels` folds the originating channel into the binding even when the addressed Pipeline does not declare it.
- `openspec/specs/pipeline-model/spec.md` currently forbids a subchart from rendering a Pipeline at all.

Two constraints from the sketched design are not negotiable in the current model, and the design works around them rather than against them. They are the first two decisions below.

## Goals / Non-Goals

**Goals:**

- Enabling the bundle with credentials produces a working install, not components that look installed and drop everything.
- A privilege split that is visible in the objects: who may read the house, who may change it, and what it takes to move between them.
- An ingest lane whose configuration a `signal-k8s-events` user already knows.
- Partial configuration renders valid objects — no dangling references, ever.

**Non-Goals:**

- Creating Secrets. Referenced by name; prerequisites documented.
- Making bundle-shipped wiring the norm. The relaxation is conditional and says so.
- Changing `/<pipeline>` addressing, the Pipeline CRD, or any contract.
- Shipping a Home Assistant instance, or authoring the user's automations.

## Decisions

### D1: Capabilities move from the profiles to the pipelines

The sketch gives `ha-user` "mcpconfig as tools". An `AgentProfile` has no such field — `allowedTools` and `mcp` were deleted, and what an agent may DO comes exclusively from the Pipeline routing it. So:

- **Profile** = identity: inline `systemPrompt` role, connectivity `env`, `maxTurns`, optional `runtimeRef`.
- **Pipeline** = `toolsets` + `mcpConfigs`, which is where `ha-observability` / `ha-admin` and `ha-api` are bound.

This is not a downgrade of the intent — it is the same privilege split expressed where the model puts it, and it has a property the sketch does not: the same profile routed by a different Pipeline gets different power, so the split is enforced by the wiring rather than by remembering which profile is which.

### D2: Both lanes serve one surface; only `ha-control` lists it as a source

Claiming and addressing are independent, and conflating them is what made an earlier draft of this design wrong. Two separate mechanisms:

- **Claiming** (`PipelineForSource`, `signals.go:330`) decides who answers an UNADDRESSED message on a surface. It reads Ready pipelines only.
- **Addressing** (`HandleCommand`, `router.go:205`) resolves `/<pipeline> <task>` with a plain `Get` — no claim check and no Ready check. `signals.go:352-354` states it directly: an addressed command opens the conversation "on the pipeline it names rather than the one claiming the source." `boundChannels` then folds the originating channel in, so the reply lands in the thread the request came from whatever the addressed Pipeline declares.

So both pipelines ARE reachable from one shared console or telegram surface, by name. That was never in question and needs no separate sources.

What listing the same chat source on both pipelines would actually do is narrower than "break it": `sourceConflicts` (`pipeline_controller.go:104-132`) flags any source an older Pipeline already lists, and lines 95-99 set `Ready=False, reason=SourceConflict` on the younger. The agent still answers when addressed, because the command path never consults Ready. The costs are that `/agents` lists Ready pipelines only (`router.go:178-181`), so `/ha-ops` vanishes from the listing people use to find it, and the Pipeline reads as broken in the console and in `kubectl get pipeline`.

Hence: `ha-control` lists the chat sources, `ha-ops` lists `ha-logs` only. Not because `ha-ops` could not otherwise answer, but because listing them **grants nothing** — addressing already ignores claims — while costing a permanent unready condition and discoverability. Both pipelines still list the console and telegram CHANNELS, so alert-driven conversations from `ha-logs` deliver to the surfaces people watch, and both stay Ready, so `/agents` shows both.

*Alternatives considered:* separate chat sources per lane (`console-ha-control`, `console-ha-ops`, plus a second telegram surface) — the console values already document declaring several sources, so it works, but it buys default-addressability for `ha-ops` at the price of a second telegram chat or topic. Rejected: the privilege split is better served by escalation being a deliberate act than by the admin agent answering casual messages.

### D3: Wiring ships with the bundle, behind a flag defaulting on

`pipelines.enabled: true`. Turning it off renders neither Pipeline and leaves every other component intact.

*Why relax the rule at all:* the rule's stated harm is a subchart wiring only its own lane because it cannot see the others. This bundle's lane is substantially its own — `ha-logs`, both profiles and both toolsets are all bundle-rendered — and the only outside references are chat surfaces, which arrive as values-supplied names. The `k8s-bundle` events template already articulates the counter-case in its own comment: "a SignalSource nobody claims reports Wired=False and DROPS every signal, so shipping the source alone would look installed and quietly do nothing."

*What keeps it from becoming the norm:* the modified requirement states three conditions — an explicit flag, values-named outside references omitted when unset, and a Pipeline that renders only when its profile does. A bundle whose sources and channels all come from elsewhere cannot meet them and gains nothing from trying. `k8s-bundle` and `telegram-bundle` keep shipping none.

### D4: Toolsets split by risk, enumerated; MCP server key fixed

`ha-observability` (read state, history, logbook) and `ha-admin` (call services, change configuration), as enumerated patterns rather than one server-wide wildcard — a wildcard spans both halves and defeats the split, which is exactly why `k8s-bundle` enumerates its sixteen read tools and six mutating ones.

`MCPConfig ha-api` fixes its server key in the template. If the key were a value, a toolset pattern and a bound server could drift apart and the split would silently stop applying.

`ha-admin` renders only when a server registering those operations exists — same guard `k8s-bundle` uses.

### D5: The credential path is wider than the toolset path, and the docs say so

`ha-ops` carries an admin API credential in `env` because REST access is its prerequisite. A Pipeline binding a shell tool then reaches Home Assistant's full API regardless of what `ha-admin` allows.

This is not a flaw to engineer away here — it is the same asymmetry `k8s-bundle` documents, where MCP reach is the server SA's RBAC intersected with the toolset (two walls) while `kubectl` plus Bash has only the runtime SA's (one). Writing it down is the deliverable; pretending the toolset split is the whole boundary would be the defect.

### D6: The adapter borrows the cluster-events vocabulary verbatim

`rules` (ordered, first-match-wins, matchers plus `for:` dwell with a re-check before emitting) and `route` (inhibition). Not a new spelling: the vocabulary was worked out for this exact problem, and two spellings would be two things to learn and two places to fix.

The rules that vocabulary carries come with it. `for:` is Prometheus and `group_wait` is Alertmanager and they are not the same mechanism, so the dwell keeps the Prometheus name. Conditions describing something already completed carry `for: 0`, because a dwell re-checks and finds the recovered state, erasing the incident. The last rule is a catch-all with a dwell, never a drop, so an unanticipated condition is verified rather than discarded.

`kubernetesAccess: false` — the data source is Home Assistant. The no-signal-loops rule still applies: nothing agent-ops-attributable becomes a signal.

## Risks / Trade-offs

- **Relaxing a stated invariant invites drift** → the relaxation is written as three testable conditions in the `pipeline-model` delta, not as a general permission, and the two existing bundles stay at zero pipelines as the working counter-example.
- **A new Go module is the larger half of this change** → separable: the bundle can ship against `signal-vmalertmanager` first and adopt `signal-ha` when it lands, since the `SignalSource` adapter reference is a value. Named in the proposal so the split is available rather than discovered late.
- **`/ha-ops` is discoverable only if people look** → `/agents` lists every Ready Pipeline, and the bundle documentation leads with the escalation step rather than burying it in a values table.
- **An admin credential plus a shell tool bypasses the toolset split** → D5; documented, not hidden. An install wanting the tighter boundary omits the shell toolset from the `ha-ops` Pipeline.
- **Conditional rendering has many combinations** → the render matrix is enumerated in the tasks, including every "component off" case, because dangling references are exactly what partial enablement produces when untested.
- **Home Assistant's MCP surface may not match the enumerated patterns** → the toolsets are values, so a mismatch is a values edit; the enumeration is the shipped default, not a hard-coded contract.

## Migration Plan

1. The adapter module and its image ship first; nothing references it until the bundle does.
2. Subchart added, default-disabled. Existing installs render nothing new.
3. `pipeline-model` spec delta and the corresponding `CLAUDE.md` invariant rewrite land with the bundle, so the rule and the code never disagree.
4. New installs: create the referenced Secrets, set the endpoint and surface names, enable the bundle.
5. The live install already runs same-shaped hand-applied CRs. Enabling the bundle with matching names would hit server-side-apply ownership conflicts, so the options are, in order: keep it disabled there; adopt by matching the live names and upgrading once with `--force-conflicts`; or install side-by-side under fresh names and retire the old CRs. Default-disabled means an upgrade never forces the choice.
6. Rollback: disable the bundle. Helm removes bundle-owned CRs; hand-applied ones are untouched.

## Open Questions

- **What exactly does the adapter watch?** Home Assistant exposes an error log, a `system_log` component, and a WebSocket event stream. The event stream is the richest and the closest analogue to watching cluster Events; the error log is the simplest and matches "listening ha logs" most literally. Leaning to the event stream with the log as a second configurable source, but this wants one look at a live instance before it is settled.
- **Should `ha-user` get the shell toolset at all?** Without it the read path is MCP-only and the boundary in D5 is clean for that profile. With it, "ask the house something" can fall back to REST. Leaning MCP-only for `ha-user`, which makes the D5 caveat apply to `ha-ops` alone.
- **Profile and pipeline both named `ha-ops`.** Legal — different kinds — but it makes prose ambiguous. Keeping the names as specified; worth one look before implementation.
