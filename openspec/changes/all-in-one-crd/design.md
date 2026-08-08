## Context

`PipelineSpec` today is five reference fields and nothing else, and
`PipelineReconciler` says so in its doc comment: *"It creates nothing — routing
reads Ready pipelines at decision time."* It resolves each ref with a `Get`,
computes source conflicts across all Pipelines in the namespace, and writes two
conditions. 162 lines, no ownership, no writes outside its own status.

Facts that constrain the design:

| Fact | Consequence |
|---|---|
| `Channel.spec.adapter` and `SignalSource.spec.adapter` carry `XValidation: self == oldSelf` | Immutability is inherited by any struct embedding those specs — and CEL transition rules need correlatable old values |
| Adapters read Channels/SignalSources over the HTTP contract (`GET /channel/channels?adapter=`), write `Served`/`ConfigValid` back to them, and have their credentials projected from them | Anything that is not a real CR would need a synthetic-object layer across the whole adapter contract |
| Conversations snapshot `channelRefs`/`profileRef`/`toolsets`/`mcpConfigs` as materialized state and re-read content per work unit | Real child CRs preserve "refs snapshotted, content not" for free |
| Manager Role already has full CRUD on `channels`, `signalsources`, `agentprofiles`, `mcpconfigs`, `pipelines` | Only `mcptoolsets` (currently `get,list,watch`) needs widening |
| No admission webhooks exist in this project | Validation must be CEL markers plus reconciler conditions |
| `ownerRef` GC fires only when the owner is deleted | Removing one inline block requires explicit reconciler-side pruning |
| Adapter cursor state lives in `agentops.dev/adapter-state-*` annotations on Channels/SignalSources | Any delete-and-recreate of a child silently rewinds an adapter |

Requester decisions carried in: materialize child CRs; all five kinds
inlinable; delete-on-removal with an explicit graduation path.

## Goals / Non-Goals

**Goals:**
- One manifest can describe a complete working flow.
- Zero change to any consumer of the resolved model — dispatch, mcpcompile,
  chat, runtimepod, and both adapter contracts stay untouched.
- The decomposed form remains first-class; inline is a spelling, not a mode.
- A one-way door is avoided: growing out of all-in-one is a supported,
  documented operation on a live pipeline.
- The manager-reads-no-Secrets invariant survives verbatim.

**Non-Goals:**
- Inlining `AgentRuntime`/`ChannelAdapter`/`SignalAdapter` — cluster-level
  implementation shared across pipelines by construction.
- An admission webhook, a conversion webhook, or a new API version.
- Cross-namespace materialization.
- Making inline the recommended form, or deprecating any ref field.

## Decisions

### D1. Inline entries embed the real spec types, flattened, keyed by `name`

```go
type InlineChannel struct {
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=200
    Name        string `json:"name"`
    ChannelSpec `json:",inline"`
}
```

which yields exactly the flattened YAML an author expects:

```yaml
spec:
  channels:
    - name: ops
      adapter: telegram
      credentialsSecretRef: {name: tg-bot}
      config: {chatId: "-100…"}
```

Embedding rather than redeclaring is the whole point: `controller-gen` derives
the inline schema from `ChannelSpec` itself, so a new field on `Channel` is
automatically inlinable and the two spellings can never drift. A parallel
`InlineChannelSpec` struct would need hand-syncing on every future field — the
exact failure this codebase avoids elsewhere by generating CRDs from types.

Same shape for `InlineSignalSource`, `InlineMCPConfig`, `InlineToolset`.
`spec.profile` is a single object, not a list, with an optional `name`
defaulting to the pipeline's own name.

### D2. Inline lists are `listType=map` keyed by `name` — not atomic

```go
// +listType=map
// +listMapKey=name
Channels []InlineChannel `json:"channels,omitempty"`
```

Three things fall out, and the third is the reason:

1. Duplicate entry names are rejected by the API server, not by the reconciler.
2. Server-side apply merges per entry instead of replacing the whole list.
3. **The inherited `self == oldSelf` rule on `adapter` keeps working.** CEL
   transition rules need the API server to correlate an element with its
   previous value; under an atomic list there is no correlation, so an
   inherited transition rule is at best meaningless and at worst rejected.
   Keyed by `name`, `channels[ops].adapter` is correlatable, so editing an
   inline entry's adapter is refused at admission with the same message a
   standalone Channel gives.

That refusal is the desired behavior, not a limitation: silently honouring an
adapter change would mean delete-and-recreate of the child, which discards the
`agentops.dev/adapter-state-*` cursor annotations and makes a Telegram adapter
re-read old updates. The remedy — rename the entry, which is a new child with a
clean cursor — is correct and explicit.

### D3. Child names are `<pipeline>-<entry>`, and collisions are refused

Prefixing keeps two pipelines that both inline a channel called `ops` from
fighting over one object. The reconciler refuses to touch any existing object
of that name it does not already own: `Ready=False`, reason `NameConflict`,
message naming the object and its actual owner. Adoption is never automatic —
silently taking ownership of a hand-made CR, and then deleting it when the
Pipeline goes, is the kind of surprise that loses someone's production channel.

`name` is capped at 200 characters so `<pipeline>-<entry>` fits inside the
253-character object-name limit with room for the pipeline name, and must be
DNS-1123 compliant so the concatenation is always a legal name.

### D4. The reconciler prunes; `ownerRef` GC only backstops Pipeline deletion

`ownerRef` handles the whole-Pipeline case for free. It does **not** fire when
one inline block is removed from a still-existing Pipeline, so each reconcile
lists children owned by this Pipeline (via a label
`agentops.dev/pipeline: <name>` for cheap selection, with ownership verified
from `ownerReferences` before any delete) and deletes those no longer named in
the spec. Order within a reconcile is create/update first, prune second, so a
rename never leaves the flow without a channel for a moment longer than needed.

Ownership is verified from `ownerReferences`, never from the label alone — a
label is user-writable and must not be able to trick the manager into deleting
a foreign object.

### D5. Graduation is a two-step handshake, annotated on the child

Annotate the materialized child `agentops.dev/graduate: "true"`. The reconciler
then:

1. removes its `ownerRef` and the management label, so GC and pruning both
   forget it;
2. drops it from `status.materialized`;
3. sets `Ready=False`, reason `GraduationPending`, with a message naming the
   exact edit that completes the swap — *remove `spec.channels[ops]`, add
   `channelRefs: [{name: my-pipeline-ops}]}`* — and makes no further writes to
   that object while the inline block remains.

Step 3 is what makes this safe rather than clever: between graduating the child
and editing the Pipeline there is a window where the spec still describes an
object the reconciler no longer owns. Recreating it would produce a duplicate
channel; silently ignoring it would leave the operator with no signal. A loud
condition carrying the remedy is the only honest option, and it self-clears the
moment the swap is applied.

Alternative rejected: an annotation on the *Pipeline* listing entries to
release. It requires editing the Pipeline twice (once to graduate, once to
swap) and puts transient bookkeeping in the spec of the object being edited.

### D6. Materialized entries join the ref set before any existing logic runs

The reconcile computes an effective ref set — `channelRefs` ∪ materialized
channel names, and so on — and every existing code path consumes that. In
particular `sourceConflicts` must count inline sources: a pipeline that inlines
its source claims it exactly as if it referenced it, and a second pipeline
referencing `<pipeline>-<entry>` by name gets the normal older-claimant-wins
treatment. Without this, inlining would be a way to bypass the one-pipeline-
per-source rule.

### D7. Toolsets and MCPConfigs materialize too, despite only the manager reading them

`MCPToolset` is an opaque string list with no status and no reader outside the
manager, so its inline form *could* have been a literal `tools: []string` on
the binding, resolved in memory, with no CR and no RBAC change. Rejected: it
would give one field two different inlining semantics — `channels[]` produces
an object you can `kubectl get`, `toolsets` would not — and every debugging
question ("what tools does this pipeline actually grant?") would have two
different answers depending on spelling. One rule, uniformly applied, is worth
the cost.

The cost is precise and small: the manager Role gains `create`/`update`/
`delete` on `mcptoolsets`. `CLAUDE.md`'s "Manager RBAC on it is read-only"
becomes "read-only except toolsets it materializes from a Pipeline", and the
RBAC integration test is updated to pin the new shape deliberately rather than
letting it drift.

### D8. Status reports ownership explicitly

```yaml
status:
  materialized:
    - {kind: Channel, name: my-pipeline-ops}
    - {kind: AgentProfile, name: my-pipeline}
  conditions:
    - {type: Materialized, status: "True", reason: AllCreated}
```

Without this, "which objects will disappear if I delete this Pipeline?" is
answerable only by grepping `ownerReferences` across five kinds. It is also
what the pruning loop reconciles against, so the observable surface and the
internal bookkeeping are the same list.

### D9. Exactly-one-of `profileRef`/`profile`, enforced by CEL at the spec level

```go
// +kubebuilder:validation:XValidation:rule="has(self.profileRef) != has(self.profile)",message="exactly one of spec.profileRef or spec.profile is required"
```

`profileRef` loses its required marker, which is a pure relaxation — every
existing manifest validates unchanged. The same exclusivity is *not* applied to
channels or sources: mixing `channelRefs` with inline `channels` is a genuinely
useful combination (shared team channel plus a pipeline-specific one), and a
profile is singular only because the Pipeline binds exactly one.

## Risks / Trade-offs

- [Deleting a Pipeline now cascades into Channels and Profiles, which used to
  be independent objects] → This is the semantics the requester chose, and D5
  gives the escape hatch. Mitigated by `status.materialized` making the blast
  radius visible before deletion, and by children only ever being objects the
  Pipeline itself created (D3 refuses adoption).
- [An operator hand-edits a materialized child; the reconciler reverts it] →
  Expected for owned objects, but silent. The reconciler emits a Kubernetes
  Event on each corrective update so the overwrite is visible in
  `kubectl describe`, and the docs state that inline children are managed.
- [Inline blocks tempt people to paste secret values into `config`] →
  `spec.config` is `RawExtension` and already accepts anything on a standalone
  Channel, so this is not new; the risk is that a fatter Pipeline looks like the
  place credentials go. Mitigated by an explicit test asserting the Pipeline
  reconciler performs zero Secret reads, plus a comment on each inline type
  pointing at `credentialsSecretRef`.
- [Adapter cursor loss via delete-and-recreate] → Structurally prevented by D2:
  the only spec change that would force a recreate is refused at admission.
  Entry renames still create a fresh child, which is correct and explicit.
- [The Pipeline reconciler becomes the most complex in the codebase] → It is
  the price of the feature. Kept in check by D6 (materialize, then reuse every
  existing path unchanged) so the new logic is confined to one create/prune
  function rather than threaded through validation and conflict detection.
- [Two ways to express the same thing splits documentation and support] →
  Accepted and bounded: the samples show one all-in-one example next to the
  decomposed set, and the docs state the rule of thumb — inline what only this
  pipeline uses, reference what is shared.
- [CEL transition rules on embedded specs behave unexpectedly under
  `listType=map`] → The behavior is asserted directly by an envtest case
  (editing `channels[ops].adapter` must be rejected) rather than assumed, since
  this is the least standard part of the design.

## Migration Plan

Purely additive; no existing manifest changes and no data migration.

1. API types + generated deepcopy/CRD, with `profileRef` relaxed to optional —
   apply to a cluster and confirm existing Pipelines are untouched and still
   Ready.
2. RBAC widening for `mcptoolsets`, ahead of the controller needing it.
3. Reconciler: materialize and update children, with `Owns()` watches and
   status reporting; no pruning yet.
4. Pruning, name-conflict refusal, and inline-source claiming.
5. Graduation handshake.
6. Samples, docs, and the invariant amendments.

Rollback: revert the controller change and the CRD. Materialized children
outlive the rollback as ordinary standalone CRs — nothing is orphaned into an
unusable state, because they were always real CRs. Their `ownerRef` then points
at a Pipeline whose controller no longer manages them, so a subsequent Pipeline
deletion would still GC them; the rollback note says to graduate children first
if the Pipeline may be deleted while rolled back.

## Open Questions

- Whether `spec.profile`'s materialized name should default to the pipeline
  name (chosen) or require an explicit `name`. Defaulting reads better in the
  common case; if two pipelines in a namespace both inline a profile and one is
  named after the other, D3's conflict refusal catches it loudly.
- Whether a future `kubectl agentops explain <pipeline>` style command should
  render the fully-resolved flow. Out of scope, but `status.materialized` is the
  data it would need.
- Whether inline `AgentRuntime` will ever be wanted for single-tenant installs.
  Deliberately deferred — runtime selection carries the ServiceAccount, and a
  Pipeline choosing an SA would make pipeline-edit rights a privilege
  escalation, which is an existing invariant this change does not touch.

### D-interaction: capabilities are moving off AgentProfile

Two changes drafted 2026-08-08 affect this one. Neither conflicts semantically,
but both touch fields D7 assumes.

`builtin-toolsets` briefly added `AgentProfile.spec.toolsets`; it was reverted
before shipping, so **there is no profile-side binding to inline** — D7's scope
is unchanged on that front. What it did ship is a chart-supplied catalog of
built-in `MCPToolset` CRs, which are ordinary referencable objects.

`capabilities-are-wiring` is the one that matters: it removes
`AgentProfile.spec.allowedTools` and `spec.mcp` entirely, and — relevant here —
**removes `ToolingBinding.mode`**, since with the Pipeline as the sole source of
capabilities there is nothing left to merge against. This change's inline
`spec.toolsets.inline[]` / `spec.mcpConfigs.inline[]` entries assume that field
exists.

Sequence `capabilities-are-wiring` first and drop `mode` from the inline shapes
here; reconciling afterwards means editing the same templates and tests twice.
