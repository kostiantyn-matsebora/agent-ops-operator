# Design: task-is-a-signal

## Context

The manager has three origination paths and three conversation builders:

| Path | Builder | Ingest applied | Addressed by |
|---|---|---|---|
| `POST /signal/inbound` | `routeSignalGroup` (`signals.go:256`) | cooldown, signature grouping, window reuse, recurrence, source status | SignalSource; Pipeline derived from the claim |
| `POST /task` | `handleTask` (`server.go:359`) | none | Pipeline, by name |
| `/<pipeline> <task>` chat | `Router.CreateTaskConversation` (`router.go:157`) | claim check only | Pipeline, by name |

All three end at the same place — a Conversation whose `profileRef`,
`channelRefs`, `toolsets` and `mcpConfigs` are copied off a Pipeline. `/task`
is the only one that reaches it without passing through a `SignalSource` at all,
and the only one whose builder has no other caller.

Two constraints shape the replacement, and both were discovered by reading what
the shipped charts actually depend on rather than what the code appears to
permit:

- **`vm-bundle` ships `grouping: {}`** (`chart/charts/vm-bundle/values.yaml:59`).
  Its Alertmanager source has no `signatureLabels` and relies entirely on
  `ingest.DefaultSignatureLabels` (`alertgroup/alertname/namespace`). Only
  `k8s-bundle` sets labels explicitly.
- **`signal-cron` depends on constant-signature folding.** It fires a *distinct*
  fingerprint per tick (`<source>@<tick>`, `signal-cron/main.go:182`) and relies
  on the empty-label signature collapsing them into one conversation, so later
  ticks resume the session. This is pinned in `cron-signal-adapter/spec.md:29`:
  "the ticks land in the same conversation, later ones resuming the agent
  session as recurrences".

So "key on the fingerprint whenever no `signatureLabels` are set" — the obvious
generalization of today's chat-only rule — would regress both: unrelated alerts
in one alertgroup would stop sharing a conversation, and every nightly cron tick
would open a new one.

## Goals / Non-Goals

**Goals:**

- One origination doorway for machines: `POST /signal/inbound`.
- Preserve `/task`'s useful semantics — task lane, one conversation per call,
  Pipeline-supplied capabilities — without preserving its endpoint.
- Delete `handleTask` and `taskReq`; leave two builders where there were three.
- Leave `vm-bundle` and `signal-cron` behavior bit-for-bit unchanged.

**Non-Goals:**

- **Authentication.** `/work` and `/work/done` are unauthenticated under the
  same posture, and the adapter token is a *scoping* mechanism —
  `scopeAllows` (`server.go:469`) returns true unconditionally for the master
  token. A token arrives with `/signal/inbound` as a consequence, not a reason.
  The real privilege question (origination instructs a `cluster-admin` agent) is
  not answered by a shared bearer token, and `rich-console-ui` says so in its
  own context.
- **Unifying `CreateTaskConversation` with `routeSignalGroup`.** The chat-command
  path short-circuits grouping on purpose — a command is answered, not
  accumulated. Merging them is a separate argument with its own regressions.
- **Shipping a manual-ask SignalSource.** See Decision 4.
- Any change to what the manager reads (still no Secrets) or grants (still no
  RBAC).

## Decisions

### Decision 1 — The replacement is `/signal/inbound`, not a renamed `/task`

**Chosen:** delete the endpoint; programmatic callers post a normalized signal
to a `SignalSource` claimed by a Ready Pipeline.

Keeping an endpoint that merely delegates to `CreateTaskConversation` would fix
the code duplication and leave the architectural one: a caller would still name
a Pipeline and receive its capabilities without any `SignalSource` mediating.
That is the shape `rich-console-ui` already refuses for the console, and the
shape the "conversations originate only from claimed signal sources" invariant
exists to forbid. Fixing the smaller duplication while preserving the larger one
is the wrong trade.

*Alternative considered — keep `/task`, delegate, add auth.* Rejected: it
survives as a third origination path, and it would have to be removed again when
`rich-console-ui` lands its own origination story.

### Decision 2 — A new `kind: task`, not a reuse of `kind: job`

**Chosen:** add `task` to the `kind` enum alongside `alert` and `job`.

`job` is unsuitable in two specific ways. It sets `jobName = source.Name`
(`signals.go:285`), asserting "this is a recurring job named for its source",
which a one-off ask is not. And it routes to `InputJob` — the job lane — while
`/task` used `InputTask`. Reusing `job` would silently change which dispatch
template a programmatic ask renders.

`task` is exactly what `chat` already is — task lane, no `jobName`, no
recurrence-on-session — minus the chat-surface requirement. It carries no
`agentops.dev/channel` label and is not subject to the check at `signals.go:418`,
because replies go to the claiming Pipeline's `channelRefs`, which
`routeSignalGroup` already copies.

*Alternative considered — `kind: chat` with a synthetic Channel.* Rejected:
it would require every programmatic caller to own a Channel, and it would route
through `routeChatSignals`, which parses `/`-prefixed payloads as commands. A CI
task beginning with a slash would be silently reinterpreted.

### Decision 3 — Fingerprint keying splits on lane semantics, not on label presence

**Chosen rule:** when a source declares no `signatureLabels`, key on the
fingerprint for `task` and `chat`; keep the `DefaultSignatureLabels` fallback for
`alert` and `job`.

This is narrower than "generalize the chat rule to all kinds", and the two
constraints in Context are why. Stated as a principle rather than an exception
list:

| Kind | Subject | Keying with no `signatureLabels` |
|---|---|---|
| `alert` | a problem that recurs | `alertgroup/alertname/namespace` — group and resume |
| `job` | a job that recurs | constant signature — group and resume |
| `chat` | a person asking | fingerprint — own conversation |
| `task` | a caller asking | fingerprint — own conversation |

`alert` and `job` are *recurring-subject* lanes: the second signal is more news
about the same thing, so folding it into the open conversation and resuming the
session is the point. `chat` and `task` are *one-shot* lanes: the second signal
is a second request. Today's chat-only special case is that rule applied to one
lane; this change states the rule and lets both one-shot lanes take it.

A source that *does* set `signatureLabels` is unaffected in every lane — an
operator who asks for grouping gets it, including for `task`.

*Alternative considered — key on fingerprint whenever labels are absent.*
Rejected on evidence: regresses `vm-bundle` alert grouping and breaks
`cron-signal-adapter/spec.md:29`.

### Decision 4 — The chart ships no manual-ask SignalSource

**Chosen:** callers post to a source that already exists and is already served —
`k8s-bundle`'s events source, a `cron` source, `vm-bundle`'s alert source.

`SignalSource.spec.adapter` is required and immutable
(`signalsource_types.go:33-35`), and the reconciler derives `Served` from
whether an adapter serves it. A source that exists only to be POSTed to has no
adapter, so it would report `Served=False` permanently on a perfectly healthy
object — a status column that lies by construction. Routing would still work
(`routeSignals` never consults `Served`), which makes it worse, not better: the
condition would be both wrong and ignorable.

The alternative — reserving an adapter name the reconciler exempts from `Served`
— buys clean `kubectl` output at the cost of the first built-in signal type, in
a model whose stated invariant is that there are none.

**Consequence to accept:** a manual ask inherits the target source's `grouping`.
Against a `cron` source (no `signatureLabels`) a `kind: task` post is keyed per
fingerprint by Decision 3 and behaves as `/task` did. Against `k8s-bundle`'s
events source (`signatureLabels: [namespace, workload]`) a task with no such
labels shares the empty-value signature with other unlabelled tasks. Operators
who want an isolated ask lane create their own source against an adapter they
run; the chart does not presume to pick one.

### Decision 5 — `agent` and `channel` are dropped, `title` is kept

`title` already exists on `NormalizedSignal` and is honored by
`routeSignalGroup` (`signals.go:295-300`). No work.

`agent` (per-call role override) has no equivalent on the signal path, and
adding one grows the adapter contract for a field whose live use is the chat
form `/<pipeline>:<agent>`, which is unaffected. Additive later if wanted.

`channel` (add a surface beyond the Pipeline's) is dropped on principle: it let
the caller choose wiring, and `Pipeline` is where wiring is declared. Its removal
is a correction, not a regression.

## Risks / Trade-offs

- **A shipped source's grouping silently mis-keys manual asks** → Decision 4's
  consequence is documented in `docs/contracts.md` next to the `task` kind, with
  the `signatureLabels` interaction stated rather than implied.
- **The keying change is invisible until a lane regresses** → the alert and job
  carve-outs get explicit fixtures: an alert pair with no `signatureLabels`
  sharing a conversation, and two cron-shaped `job` fingerprints folding with
  the second as `InputRecurrence`. Dispatch/ingest semantics are pinned by
  fixtures, so this is a deliberate fixture addition.
- **Scripted `POST /task` callers break at upgrade** → `CHANGELOG.md` carries
  before/after curl. Pre-1.0, provisional API group; no deprecation window is
  offered because a 410 shim would preserve the second doorway it is the point
  of this change to close.
- **The smoke test gets harder** — it now needs `ADAPTER_TOKEN` and a source
  name rather than a bare curl → `CLAUDE.md`'s build/test block carries the
  exact replacement command, and that block's current `{"profile":"stub"}`
  example is already stale (the handler has required `pipeline` since Pipelines
  became addressable), so it needed editing regardless.
- **`rich-console-ui` overlaps this file set** — it also rewrites origination
  and touches `signals.go` → the two changes agree on direction (that proposal
  already forbids `/task`), but whichever lands second rebases. Neither depends
  on the other.

## Migration Plan

1. Land the `task` kind and the keying rule first — additive, no removal, both
   paths work.
2. Move `internal/integration/tooling_test.go:270-298` off `/task`.
3. Delete the route, `handleTask`, `taskReq`, and the package-doc line.
4. Sweep docs, chart comments, NOTES.txt, samples, and the CRD-source comment in
   `conversation_types.go`; regenerate CRDs (that comment renders into
   `chart/files/crds/agentops.dev_conversations.yaml:185`).
5. `CHANGELOG.md` entry, newest first, with the curl migration.

Rollback is the revert: nothing persists, no CRD schema field is added or
removed, and no stored object changes shape.

## Open Questions

None blocking. One deferred: whether `task` should eventually accept an `agent`
override on `NormalizedSignal` — deliberately not decided here, because the
answer belongs to whoever first wants per-call role selection from a machine.
