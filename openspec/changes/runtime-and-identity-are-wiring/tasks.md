## 1. API

- [ ] 1.1 `PipelineSpec.RuntimeRef *ObjectRef` — optional. Doc comment states
      what it replaces and why: an `AgentRuntime` carries the ServiceAccount, so
      selecting one is selecting the agent's power.
- [ ] 1.2 `PipelineSpec.ServiceAccountName string` — optional, OVERRIDES the
      runtime's. Doc comment states that naming is not creating: no reconciler
      makes one, and the grant stays external.
- [ ] 1.3 `ConversationSpec` gains the materialized pair. Doc comment says these
      are RESOLVED names, not the Pipeline's raw fields — see design D4.
- [ ] 1.4 `AgentProfileSpec.RuntimeRef` marked `Deprecated:` with the release it
      is removed in, and a pointer to the Pipeline field.
- [ ] 1.5 Regenerate deepcopy and CRDs into `chart/files/crds`, then re-run
      `python3 .github/scripts/docs-generate.py`.

## 2. Resolution

- [ ] 2.1 `runtimepod.ResolveFor` takes the CONVERSATION, not the profile.
      Every caller moves with it — `/exit`, dispatch's continuity check, the
      admission path.
- [ ] 2.2 Precedence exactly as design D3, in one function, in this order:
      conversation → pipeline → profile (deprecated) → `default` → bootstrap.
- [ ] 2.3 The pod's `ServiceAccountName` prefers the conversation's over the
      runtime's.
- [ ] 2.4 `ContinuityPossible()` keeps its meaning — it reads the resolved
      runtime's `contextStorage`, and only the resolution changed.
- [ ] 2.5 Table-driven test for every rung of both chains, INCLUDING the
      deprecated one and a conversation that predates the snapshot.

## 3. Materialization

- [ ] 3.1 Snapshot both at creation on the SIGNAL path
      (`internal/httpapi/signals.go`).
- [ ] 3.2 Snapshot both on the CHAT-COMMAND path. Every origination path reads
      the same Pipeline for the same fields, or the two drift.
- [ ] 3.3 Snapshot the RESOLVED names, so a conversation created while the
      Pipeline named nothing keeps the default it ran with.
- [ ] 3.4 Test: editing a Pipeline's `serviceAccountName` does NOT change an
      existing conversation's next pod. This is the privilege case — it is why
      the snapshot exists.
- [ ] 3.5 Test: correcting the `AgentRuntime`'s image DOES reach existing
      conversations. Ref frozen, content not.

## 4. Chart

- [ ] 4.1 `runtime-rbac.yaml` renders mode-driven bindings for MORE THAN ONE
      service account.
- [ ] 4.2 `global.agentops.runtime.serviceAccountName` documented as the
      DEFAULT rather than the only account.
- [ ] 4.3 The parent's `pipelines:` values gain both optional fields.
- [ ] 4.4 `k8s-bundle`, `prometheus-bundle` and `ha-bundle` pipeline values gain
      them too, unset.
- [ ] 4.5 Chart render test: unset changes nothing, and set reaches the rendered
      Pipeline.
- [ ] 4.6 Chart render test: a second SA renders with its own RBAC and no second
      `AgentRuntime`. That is the case the change exists for.
- [ ] 4.7 Confirm NO bundle renders a ServiceAccount, still.

## 5. Runtime RBAC — no cluster-admin, no Secrets

- [ ] 5.1 **DELETE the `cluster-admin` ClusterRoleBinding** from
      `chart/templates/runtime-rbac.yaml`. It is what `rbacMode: full` renders
      today.
- [ ] 5.2 Write the ENUMERATED acting ClusterRole. Start from what
      `k8s-bundle`'s MCP server is already permitted to do — that list exists
      and is in use — and add the mutating verbs an agent fixes things with.
- [ ] 5.3 **NO verb on `secrets`, in any mode.** No wildcard on `resources` or
      `apiGroups`, because `*` reaches Secrets without naming them.
- [ ] 5.4 No `escalate` or `bind` on `rbac.authorization.k8s.io`, or the role
      widens itself.
- [ ] 5.5 Chart render test: `rbacMode: full` renders NO binding to
      `cluster-admin`.
- [ ] 5.6 Chart render test that walks EVERY rendered runtime role and fails on
      a `secrets` resource or a `*` in `resources`/`apiGroups`. Pin it across
      all modes, not just `full` — this is the test that has to survive someone
      adding a mode.
- [ ] 5.7 `docs/installation.md` — what the acting role grants, and how to add
      a grant it omits (own ClusterRole, own account, named on a Pipeline).
- [ ] 5.8 `CHANGELOG.md` — breaking, with the upgrade step for an install
      running `full`.
- [ ] 5.9 Live check: `kubectl auth can-i get secrets --as=system:serviceaccount:<ns>:<sa>`
      returns **no**, in every mode. Note the `pods/eviction` gotcha if the
      check extends to subresources — use `--subresource=`.

## 6. Rules

- [ ] 6.1 `.claude/rules/wiring.md` — **DELETE** "the SA stays runtime-level on
      purpose" and the "runtimes are generic" consequence that followed from it.
      Replace with the precedence and the reason the placement moved. Keep the
      history: the old rule's argument was that a Pipeline choosing an SA would
      be escalation, and it failed the symmetry test.
- [ ] 6.2 `.claude/rules/invariants.md` — the substrate section says how agents
      execute is release-wide. It is now release-wide by DEFAULT and per-route
      by choice.
- [ ] 6.3 `.claude/rules/terminology.md` — the `AgentProfile` row lists what
      identity holds. Remove execution preference from it.

## 7. Deprecation

- [ ] 7.1 The dual-read is time-boxed to ONE release and the deletion is a task,
      not a later sweep — same posture as the retired `sessionId`.
- [ ] 7.2 Test that pins the dual-read working AND a second test, skipped with a
      note, that fails once the field is gone. The removal must be visible.

## 8. Verification

- [ ] 8.1 Integration test: two Pipelines, one runtime, two service accounts →
      two pods with different identities.
- [ ] 8.2 Integration test: a Pipeline naming neither field behaves exactly as
      before, byte for byte in the rendered pod spec.
- [ ] 8.3 Live smoke: move one real Pipeline onto an explicit SA, confirm the
      pod runs under it and the agent's cluster reach changes as expected.
      A rendered pod is not a running one.

## 9. Open question to close before the resolution work

- [ ] 9.1 Does `Ready` refuse a Pipeline naming a nonexistent ServiceAccount?
      It needs an API read the manager has no RBAC for. Design leans NO — the
      pod fails at admission naming the account. Decide, then write it into
      `pipeline-model` either way.

## 10. Documentation — THE LAST TASK, and it is not optional

**Both halves, and they are skipped independently.** A change is not finished
while a reader meets the old model.

### Reference docs

- [ ] 10.1 `docs/concepts.md` — `Pipeline.spec.runtimeRef` and
      `serviceAccountName`, the precedence chain, and the Conversation snapshot.
      Remove `runtimeRef` from what an `AgentProfile` holds.
- [ ] 10.2 `docs/installation.md` — the runtime SA is a DEFAULT, not a
      singleton. What the enumerated acting role grants, and how to add a grant
      it omits.
- [ ] 10.3 `docs/CHANGELOG.md` — BOTH breaking halves, newest first: the moved
      field with its one-release deprecation, and the loss of `cluster-admin`
      with its upgrade step.
- [ ] 10.4 Re-run `python3 .github/scripts/docs-generate.py` — a field moved
      between two kinds, so every generated block and `cr-reference.md` is
      stale.

### The ADOPTER SITE

- [ ] 10.5 `docs/guides/pipeline.md` — the Pipeline now decides what executes
      its conversations and under whose identity. This is the guide the change
      most affects and the one most likely to be forgotten.
- [ ] 10.6 `docs/guides/agent-profile.md` — a profile no longer selects a
      runtime.
- [ ] 10.7 `docs/introduction.md`, `docs/getting-started.md`, `docs/index.md` —
      does any of them state or imply the old placement, or promise a
      `cluster-admin`-shaped capability the chart no longer grants?
- [ ] 10.8 Read every changed page against `docs/CLAUDE.md` BY HAND, and run
      the prose lint. There is no lint script in this repo.
- [ ] 10.9 Build the site and LOOK at the changed pages. A squeezed column and a
      wrapped key are invisible until rendered.

### Rules

- [ ] 10.10 `.claude/rules/` — `wiring.md`, `invariants.md`, `terminology.md`.
      Listed in the Rules section and re-stated here because this task is the gate: the
      change is not done until every one of them is true.
