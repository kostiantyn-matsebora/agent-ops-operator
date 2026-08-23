# All-in-one Pipeline: inline resource definitions that materialize into real CRs

## Why

A minimal working setup today is five separate CRs — `AgentProfile`, `Channel`,
`SignalSource`, and a `Pipeline` to wire them, plus an `MCPToolset` the moment
the agent needs tools. Each is small, each lives in its own file, and the
`Pipeline` that ties them together contains nothing but names. For a
single-purpose flow ("this cron fires this agent into this Telegram topic")
that decomposition costs more than it returns: five objects to apply in the
right order, five names to keep in sync, and no single artifact anyone can read
to understand the flow. Adopters evaluating the operator hit this on day one,
and adopters running one pipeline per team hit it forever.

The reuse-oriented decomposition is right for the general case and is not being
replaced. What is missing is the **collapsed spelling** of it.

## What Changes

- **`Pipeline` gains five optional inline stanzas** alongside its existing
  refs: `spec.channels[]`, `spec.signalSources[]`, `spec.profile`,
  `spec.toolsets.inline[]`, and `spec.mcpConfigs.inline[]`. Each entry is a
  `name` plus exactly the spec of the corresponding CR — the same schema, not a
  parallel one.
- **Inline entries materialize into real CRs** named `<pipeline>-<entry>` and
  owned by the Pipeline (`ownerRef`). Nothing downstream changes: adapters
  still fetch them over `GET /channel/channels?adapter=`, `Served`/`ConfigValid`
  conditions still land on real objects, credential projection is untouched,
  conversations still snapshot refs, and `kubectl get channels` still lists
  them. The Pipeline reconciler, which creates nothing today, becomes an
  owner of child CRs.
- **Inline and ref forms mix freely.** A pipeline may inline its channel and
  reference a shared profile. Materialized children join the ref set, so the
  rest of the reconciler sees one uniform list.
- **Removing an inline block deletes its child** (reconciler-side pruning;
  `ownerRef` GC only covers Pipeline deletion). Deleting the Pipeline deletes
  all of them.
- **Graduation path out of all-in-one**: annotating a materialized child
  `agentops.dev/graduate: "true"` makes the reconciler drop its `ownerRef` and
  stop managing it, then report exactly which spec edit completes the swap
  (remove the inline block, add the ref). Growing out of the collapsed form
  never requires deleting and rebuilding a working flow.
- **Name collisions are refused, never adopted.** If `<pipeline>-<entry>`
  exists and is not owned by this Pipeline, the Pipeline reports `Ready=False`
  with reason `NameConflict` and touches nothing.
- **BREAKING (schema only, backward-compatible in practice)**: `spec.profileRef`
  becomes optional, with a CEL rule requiring exactly one of `profileRef` or
  `profile`. Every existing Pipeline manifest stays valid.
- **Manager RBAC on `mcptoolsets` widens** from read-only to include
  `create`/`update`/`delete`, scoped to toolsets it materializes. This amends a
  stated invariant and is called out as such.

Explicitly preserved: **the manager still reads no Secrets.** Inline blocks
carry `credentialsSecretRef` names and `valueFrom` sources exactly as the
standalone CRs do — never a secret value. Runtime selection stays
`profile.runtimeRef`; the Pipeline still selects no runtime and no
ServiceAccount.

Not in scope: inlining `AgentRuntime`, `ChannelAdapter`, or `SignalAdapter`
(cluster-level implementation, deliberately not per-pipeline); an admission
webhook (validation stays CEL + conditions); any change to conversation
dispatch, grouping, or the adapter contracts.

## Capabilities

### New Capabilities
- `pipeline-inline-resources`: the inline stanzas, materialization into owned
  child CRs, naming and collision rules, pruning, the graduation path, and the
  status surface reporting what a Pipeline owns.

### Modified Capabilities
- `pipeline-model`: the Pipeline CRD requirement currently states the Pipeline
  "SHALL carry no credentials, no server or tool definitions" and that its
  reconciler maintains conditions "without creating any workload" — both change.
  The one-pipeline-per-source claim rule must also count inline sources.

## Impact

- **`api/v1alpha1/pipeline_types.go`**: five new optional fields, four small
  inline wrapper types embedding the existing specs, list-type markers, and a
  spec-level CEL rule. Regenerates `zz_generated.deepcopy.go` and
  `chart/files/crds/agentops.dev_pipelines.yaml`.
- **`internal/controller/pipeline_controller.go`** (162 lines today): grows a
  materialize/prune/graduate loop, `Owns()` watches for five kinds, name-
  conflict detection, and status reporting. This is the bulk of the work.
- **`chart/templates/rbac.yaml`**: `mcptoolsets` gains write verbs.
- **Source-conflict logic**: must treat materialized inline sources as claimed
  by their Pipeline.
- **Tests**: new envtest coverage for materialize / prune / conflict /
  graduate / secret-free-spec; `internal/integration/rbac_test.go` updated for
  the widened toolset verbs.
- **Docs**: `config/samples/samples.yaml` gains a single-file all-in-one
  example; `CLAUDE.md`'s Pipeline and MCPToolset terminology entries and the
  Pipeline invariant need precise amendment; README/`docs/concepts.md` gain the
  collapsed spelling next to the decomposed one.
- **No change** to `internal/dispatch`, `internal/mcpcompile`, `internal/chat`,
  `internal/runtimepod`, the adapter contracts, or any adapter module — which
  is the point of materializing rather than embedding.
