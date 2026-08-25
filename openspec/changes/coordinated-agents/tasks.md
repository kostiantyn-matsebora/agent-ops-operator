Phases follow design D-H. Every build and test below runs INSIDE the worktree
(`docker exec -w "$PWD"` from `../agent-ops-worktrees/coordinated-agents`), and
every deploy uses `--state-values-set chartPath=` naming this worktree's
`chart/` — the defaults resolve master and report success against it.

## 1. Phase 1 — `AgentCapability` and the shared capability (design D-A)

- [ ] 1.1 `api/v1alpha1/agentcapability_types.go`: `AgentCapabilitySpec` with the six capability
      fields moved out of `PipelineSpec`; `AgentCapability` kind + list; `AgentStatus`
      with `Ready`. Doc comments say it is the CAPABILITY and wires nothing.
- [ ] 1.2 `PipelineSpec` embeds `AgentCapabilitySpec` inline (JSON names unchanged) and
      gains `AgentRef *ObjectRef`; CEL rule for exclusivity per D-A;
      `profileRef` becomes optional.
- [ ] 1.3 `dispatch.ResolveCapability` — the one resolver. Every read of a
      Pipeline's capability fields moves to it (signals.go creation, router
      origination, pipeline reconciler validation, runtimepod resolution).
- [ ] 1.4 AgentCapability reconciler: `Ready` validates the same refs the Pipeline
      reconciler validates today, through shared code — no second validator.
- [ ] 1.5 Pipeline `Ready` False naming a dangling `capabilityRef`; neither
      `capabilityRef` nor `profileRef` → `Ready` False, not admission.
- [ ] 1.6 Regenerate deepcopy and CRDs; `chart/crds/agentcapabilities.agentops.dev.yaml`.
- [ ] 1.7 Tests: envtest — inline and referenced Pipelines produce identical
      conversation snapshots; CEL rejects both forms; every existing Pipeline
      fixture passes unchanged.

## 2. Phase 2 — `Coordinator`, provenance, the loop (design D-B, D-C, D-E)

- [ ] 2.1 `api/v1alpha1/coordinator_types.go`: embedded `AgentCapabilitySpec` or
      `capabilityRef`, `signalSourceRefs`, `channelRefs`, `agents[]{name, capabilityRef,
      description (required, MinLength=1)}`, `limits{maxAgents, maxTurns,
      deadline}`; status with `Ready`.
- [ ] 2.2 `ConversationSpec.CausedBy *Provenance{root, entry}`,
      `spec.coordinatorRef`; `ConversationStatus.budget{maxAgents, maxTurns,
      deadline, agentsInvoked, turns}`, `escalatedAt`, `closeReason`.
- [ ] 2.3 `PipelinesForSource` → claimants of both kinds (D-B); every call site
      iterates claimants; `Wired` counts both; bare-chat choice list names
      both.
- [ ] 2.3b `internal/addressing` + `HandleCommand` resolve `/<name>` across
      Pipeline and Coordinator; `/pipelines` and the choice list carry both
      kinds' addressed forms; an addressed root binds the origin surface only.
- [ ] 2.4 Coordinator reconciler: `Ready` per D-B, message lists failing entry
      names, and a name a Pipeline also holds; a not-Ready Coordinator claims
      nothing.
- [ ] 2.5 Root creation from a Coordinator: no `channelRefs`, limits
      snapshotted into `status.budget`.
- [ ] 2.6 Manager `/coordinate/*` surface: `invoke`, `close`, `escalate`,
      `read`; caller token context `coordinator:<name>:<conversation>`;
      `agents[]` list and root scope enforced HERE (D-F). `invoke` returns
      created|attached.
- [ ] 2.7 Member creation: `causedBy` set, no channels, capability from the
      listed AgentCapability, reuse scoped by `causedBy` in `reusableBy`.
- [ ] 2.8 `handleWorkDone` on a member appends the result input on the root in
      the same status write (D-C); dedup key `member:<conv>:<runId>`; closed
      root → skip. Reconciler backstop re-derives a missing append.
- [ ] 2.9 `/channel/inbound` refuses an input whose origin surface is the
      target conversation.
- [ ] 2.10 Budget edges per D-E: `maxAgents` in the invoke handler, `maxTurns`
      in `handleWorkDone` on a root, `deadline` via reconciler requeue. Closing
      a root closes its members `root-closed`.
- [ ] 2.11 `closeReason` stamped by the `close` verb (required there), absent
      from `/close`; a coordinator cannot close outside its root.
- [ ] 2.12 Regenerate deepcopy and CRDs; `chart/crds/coordinators…yaml`.
- [ ] 2.13 Tests: envtest — fan-out counts a Coordinator; member result lands
      on root exactly once across a simulated restart; self-input refused;
      each of the three limits closes with reason and members; reuse scope.

## 3. Phase 2 — escalation (design D-D)

- [ ] 3.1 `escalate` sets `spec.channelRefs` from the Coordinator read at that
      moment, stamps `escalatedAt`, enqueues `ensure-topic` with the digest as
      the opening message.
- [ ] 3.2 `DeliverInputs` fences on `escalatedAt`: nothing earlier is
      delivered to the escalated channels.
- [ ] 3.3 `budget-exceeded` escalates first with a manager-written digest
      (limit, counts, member list), then closes.
- [ ] 3.4 Fake-chat integration test: escalate opens threads with the digest
      only; a later member result reaches the thread; a person's reply is a
      root input.

## 4. Phase 3 — `platform/mcp-aops` (design D-F)

- [ ] 4.1 New module, standard library only, shared Dockerfile recipe;
      `.github/components.sh` derives `mcp-aops`; multi-arch.
- [ ] 4.2 MCP over streamable HTTP; tools `list_agents`, `list_conversations`,
      `get_conversation`, `get_tree`, `invoke`, `close`, `escalate`, `read`;
      each forwards the caller's `AOPS_MCP_TOKEN` to `/coordinate/*`.
- [ ] 4.3 Runtime pod build injects `AOPS_MCP_TOKEN` for conversations whose
      capability binds the aops MCPConfig; derived, never stored.
- [ ] 4.4 Chart: Deployment, Service, NetworkPolicy under the ADR 0001 wall,
      RBAC (none beyond the floor), `coordination.enabled` gate,
      `global.builtinToolsets.agentops-coordinate`, the rendered `MCPConfig`;
      NOTES.txt line.
- [ ] 4.5 Smoke against a live install from the worktree chart: a Coordinator
      with one AgentCapability, a signal, `invoke` observed creating a member, result
      landing on the root, `escalate` opening a Telegram thread. Record the
      verdict, not the transcript.

## 5. Phase 4 — console (design D-G)

- [ ] 5.1 Adapter watches `agentcapabilities` and `coordinators`; chart Role grants
      list/watch on both.
- [ ] 5.2 Inventory rows and topology nodes for both kinds; `capabilityRef` and
      `agents[]` edges; unwired AgentCapability rendered distinct from misconfigured.
- [ ] 5.3 Incident view on a root: one timeline, members interleaved,
      expandable; member transcript links to its root; list groups by root
      with a flatten toggle; un-escalated closures marked with `closeReason`.
- [ ] 5.4 Fixture gains one root with three members; screenshot the view per
      `visual-check.md` and READ the PNG before ticking.

## 6. Rules and vocabulary

- [ ] 6.1 `.claude/rules/wiring.md`, three claims, each named: "no other CR
      carries wiring" → two wiring kinds; the `#### Capabilities are wiring,
      exclusively` header and its section → capabilities are declared on an
      AgentCapability OR inline on a Pipeline/Coordinator, and reached through wiring
      only; under `### MCPToolset`, "Bound from `Pipeline.spec.toolsets` ONLY"
      → bound from any capability's `toolsets`, never a profile's. The
      escalation-time Coordinator read stated as the one exception to
      "nothing reads wiring after creation".
- [ ] 6.2 `terminology.md`: the "Agent is TAKEN" entry names `AgentCapability`
      and why the CRD is not `Agent`; `Coordinator`, `root`, `member`, `escalate`;
      `structure.md`: `platform/mcp-aops`; `invariants.md`: the loop refusal,
      the budget, "a caused conversation binds no human channel".
- [ ] 6.3 `retired-vocabulary.json`: no new term — nothing is retired.

## 7. Verification

- [ ] 7.1 Every module builds, vets and tests in the container, from the
      worktree path.
- [ ] 7.2 `KUBEBUILDER_ASSETS` envtest suite green.
- [ ] 7.3 `python3 .github/scripts/publication-guard.py` and
      `retired-vocabulary-guard.py` pass; record the verdict only.
- [ ] 7.4 `helm template` with and without `coordination.enabled`;
      `serviceaccount-guard.py` passes.

## 8. Documentation — THE LAST TASK, and it is not optional

### 8a. Reference docs

- [ ] 8a.1 `docs/concepts.md`: kind table (thirteen), `AgentCapability`, `Coordinator`,
      `causedBy`, the loop, escalation, budget, the state matrix rows.
- [ ] 8a.2 `docs/contracts.md`: `/coordinate/*`, `/work/done` root routing,
      `/channel/inbound` refusal, the aops MCP tool contract, the four token
      derivation contexts.
- [ ] 8a.3 `docs/console.md` and `docs/console-guide.md`: the incident view.
- [ ] 8a.4 `docs/security.md`: the agent-invokes-agents flow; re-run
      `python3 docs/diagrams/threat-model.py`.
- [ ] 8a.5 `docs/installation.md`: `coordination.*` values, the component.
- [ ] 8a.6 `docs/CHANGELOG.md`: two CRDs to apply by hand, the new component.
- [ ] 8a.7 `docs/adr/0002-coordinated-agents.md`: status → Accepted, plus a
      "What implementation changed" section as 0001 carries.
- [ ] 8a.8 Re-run `python3 .github/scripts/docs-generate.py`; commit every
      regenerated block and `docs/cr-reference.md`.

### 8b. Adopter site

- [ ] 8b.1 `docs/index.md`: kind count, the "what you write" tab mentions a
      Coordinator.
- [ ] 8b.2 `docs/introduction.md`: two wiring kinds over one capability.
- [ ] 8b.3 `docs/installation.md`: component list.
- [ ] 8b.4 `docs/guides/coordinate-agents.md`: new guide with generated CR
      blocks; `_data/nav.yml` line.
- [ ] 8b.5 `README.md`: kind table, one line under the seams; stays ≤ 215
      lines.
- [ ] 8b.6 `platform/console/ui`: re-run BOTH `npm run screenshots` and
      `npm run demo`; commit the assets.
