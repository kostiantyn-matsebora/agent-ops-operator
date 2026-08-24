## Context

See `proposal.md` — Why. The relevant current state, in one place:

| Piece | Today |
|---|---|
| `AgentRuntime.spec.contextSync.paths` | rendered from the runtime's merged values, which ship `[]` |
| `agentops.renderRuntime` in `chart/templates/_helpers.tpl` | `{{- with (($rt.contextSync \| default dict).paths) }}` — an empty list renders no stanza at all. ONE helper, called by the parent AND by the `claude` bundle |
| `contextsync_validate.go:29` | a hand-written empty list is refused |
| `podspec.go:287` | `sidecar := sync != nil && cfg.ContextSyncImage != ""` |
| `podspec.go:359` | the `case sidecar:` branch mounts `ClaimName: cfg.ContextPVC`, unconditionally |
| `podspec.go:262` | `ContinuityPossible()` returns false when `ContextPVC == ""` |
| `context-sync/main.go:77` | the sidecar exits if `CONTEXT_SYNC_PATHS` is empty |

Three of those already behave correctly for the empty-path case, in three
independent layers. The gap is narrower than it looks: one predicate is missing
one conjunct, and one chart default is empty.

## Goals / Non-Goals

**Goals:**

- A default install with a context volume runs synchronised, without an operator
  setting a value.
- A pod is never constructed referencing a persistent claim by an empty name.
- The pod builder and continuity resolution give the same answer about the
  DURABLE VOLUME: where there is none, neither promises continuity and neither
  references a persistent claim.

**Non-Goals:**

- **Changing what an include list means.** Empty stays invalid in all three
  layers that currently say so.
- **Inferring paths for any runtime the project does not ship.** The chart
  declares the reference runtime's paths because it ships that runtime, not
  because it can work them out.
- **A migration.** See Migration Plan.
- **Making the sidecar's storage layout, retention or checkpoint semantics
  configurable beyond what exists.**

## Decisions

### D1 — The fallback is a conjunct on the existing predicate, not a new branch

```go
sidecar := sync != nil && cfg.ContextSyncImage != "" && cfg.ContextPVC != ""
```

The `switch` beneath it already has the correct arms: with `sidecar` false and no
claim, control reaches the `default:` arm, which is the ephemeral `EmptyDir` pod.
The fallback therefore needs no new code path — only for the predicate to stop
being true in a case it cannot serve.

**THE AGREEMENT IS ABOUT THE VOLUME, NOT ABOUT THE PROMISE**, and the stronger
reading is false. `ContinuityPossible()` also returns false for
`contextStorage: none` — a backend keeping no context on disk — which is
impossible to continue whether or not a volume exists. That runtime still mounts
the claim it is given, because the mount is where the pod's filesystem lives.
Written as "continuity impossible implies no persistent claim", this invariant
fails on a case nothing is wrong with; written as "no volume implies no
persistent claim", it is exactly the property the defect broke.

**Alternative considered: fail loudly at pod construction.** Rejected because the
operator did not misconfigure anything. An install that runs without persistence
is asking for ephemeral context, and the project already has a word for that
answer — never-promised, answer fresh and say so. Failing would convert a chosen
configuration into an outage.

**Alternative considered: build the sidecar against an `EmptyDir` store.** A
sidecar checkpointing to ephemeral storage is a process that reports success and
protects nothing, which is the precise failure the empty-include-list rule exists
to prevent, one layer over.

### D2 — Correctness comes before the default, in that order

The fallback (D1) lands and is covered by a test before the chart default
changes. Today the defect needs someone to declare `contextSync` while running
without persistence; with the default flipped it is every conversation of every
persistence-disabled install. Shipping them in the other order, or together
without the test, makes the blast radius of a regression the whole install base.

This is an ordering constraint on the tasks, not two changes.

### D3 — The VENDOR'S BUNDLE declares the paths; the runtime still owns them

`chart/charts/claude/values.yaml` gains
`contextSync.paths: [".claude/projects/-data-workspace/**"]`, and the render is
untouched — `agentops.renderRuntime` already renders whatever the merged values
hold.

**NOT `global.agentops.runtimeDefaults`, AND THAT IS THE WHOLE PLACEMENT.** Those
defaults are what EVERY runtime inherits, and this value is one vendor's
filesystem layout: it describes where claude-code files transcripts and means
nothing to an Ollama or Copilot backend. `agent-runtime-ownership` states the
rule directly — the vendor's image and model credential already moved to this
bundle for the same reason, and a path is the same kind of fact.

An install running another backend replaces the image, the credential and the
paths together, in one section, because all three describe the same vendor.

The declaration still belongs to the runtime in the sense the spec means: the
runtime states it, in the bundle that ships that runtime — not a human typing it
on the runtime's behalf.

**Alternative considered: default it in the API type or the manager.** Rejected —
that puts one vendor's filesystem layout inside a component that must stay
generic, and it is the argument that correctly keeps paths out of the manager
today.

**Alternative considered: the release-wide defaults.** Rejected on the rule
above, and it is worth naming because it is where this value was ALMOST put: the
key is spelled `contextSync` in both places, so the wrong one renders and looks
correct until a second vendor is installed.

**Alternative considered: have the runtime image report its paths over the work
contract.** Rejected as disproportionate. The sidecar needs the paths at pod
construction, before the agent container has run, so this would need a new
pre-work announcement in the contract to move a constant that is already
expressible where it lives.

### D4 — The value stops being a comment

The correct value currently sits commented out directly beneath the empty
default. Moving it up is most of D3, and the comment that explained why the list
was empty is replaced rather than deleted: a reader needs to know what declaring
it does, and that a different backend must replace it.

## Risks / Trade-offs

- **Every conversation holding a context handle fails its next run** → Accepted,
  not mitigated. The durable layout moves from the claim root to a
  per-conversation path, and the continuity rule turns that into a clean failure
  with the existing reset verb as recovery. The project is pre-1.0 and
  unpublished; `docs/CHANGELOG.md` states it.

- **`$HOME` becomes ephemeral on every conversation** → `liveSizeLimit` bounds
  it, defaulting to 4Gi. The trade is real: node ephemeral-storage pressure
  arrives on every install rather than only on installs that opted in. Named in
  `installation.md` rather than left to be discovered under load.

- **The fallback silently produces a pod with no durable context** → It is not
  silent: continuity resolution already reports the conversation as
  never-promised, and that is what the reply says. This change makes the pod
  agree with a message the manager was already sending.

- **A third-party runtime is now the only unsynchronised case** → Which makes it
  the case least exercised by the maintainers' own install. The
  `agent-runtime.md` guide is where that is taught, and it is named in the
  proposal's Impact for that reason.

## Migration Plan

**There is none, deliberately.**

Opting in relocates the durable layout from the claim root to a per-conversation
subPath. Nothing copies a volume, so a conversation holding a context handle
looks for it under a path that has nothing in it. The continuity rule turns that
into a failed run rather than an answer without memory — which is the designed
behaviour, and the reset verb is the recovery.

The alternative — a render guard in the shape of
`agentops.contextClaimRenameGuard` — is not built here. That guard exists because
a published chart must not silently relocate an adopter's context. This project
is pre-1.0, unpublished, and the decision on record is that existing
conversations are not to be preserved.

`docs/CHANGELOG.md` carries the entry: what moves, what fails, and the reset verb
that recovers it. That is the whole of the migration story.

**Rollback:** clearing `claude.contextSync.paths` restores the direct mount, and
`TestClearingContextSyncRestoresTheDirectMount` already covers it. Rollback has
the same consequence in reverse — the layout moves back, and handles written
under the subPath are not found.
