Phases follow design D-H. Every build and test below runs INSIDE the worktree
(`docker exec -w "$PWD"` from `../agent-ops-worktrees/coordinated-agents`), and
every deploy uses `--state-values-set chartPath=` naming this worktree's
`chart/` — the defaults resolve master and report success against it.

## 1. Phase 1 — `AgentCapability` and the shared capability (design D-A)

- [x] 1.1 `api/v1alpha1/agentcapability_types.go`: `AgentCapabilitySpec` with the six capability
      fields moved out of `PipelineSpec`; `AgentCapability` kind + list; `AgentStatus`
      with `Ready`. Doc comments say it is the CAPABILITY and wires nothing.
- [x] 1.2 `PipelineSpec` embeds `AgentCapabilitySpec` inline (JSON names unchanged) and
      gains `AgentRef *ObjectRef`; CEL rule for exclusivity per D-A;
      `profileRef` becomes optional.
- [x] 1.3 `dispatch.ResolveCapability` — the one resolver. Every read of a
      Pipeline's capability fields moves to it (signals.go creation, router
      origination, pipeline reconciler validation, runtimepod resolution).
- [x] 1.4 AgentCapability reconciler: `Ready` validates the same refs the Pipeline
      reconciler validates today, through shared code — no second validator.
- [x] 1.5 Pipeline `Ready` False naming a dangling `capabilityRef`; neither
      `capabilityRef` nor `profileRef` → `Ready` False, not admission.
- [x] 1.6 Regenerate deepcopy and CRDs; `chart/crds/agentcapabilities.agentops.dev.yaml`.
- [x] 1.7 Tests: envtest — inline and referenced Pipelines produce identical
      conversation snapshots; CEL rejects both forms; every existing Pipeline
      fixture passes unchanged.

## 2. Phase 2 — `Coordinator`, provenance, the loop (design D-B, D-C, D-E)

- [x] 2.1 `api/v1alpha1/coordinator_types.go`: embedded `AgentCapabilitySpec` or
      `capabilityRef`, `signalSourceRefs`, `channelRefs`, `agents[]{name, capabilityRef,
      description (required, MinLength=1)}`, `limits{maxAgents, maxTurns,
      deadline}`; status with `Ready`. Also carries `coordinatorRef` per entry
      (mutually exclusive with `capabilityRef` by CEL) — nesting per D-B, which
      2.4's Ready check already needs to walk.
- [x] 2.2 `ConversationSpec.CausedBy *Provenance{parent, entry}` — PARENT, one
      hop, never the tree's ultimate root — `spec.coordinatorRef`;
      `ConversationStatus.budget{maxAgents, maxTurns, deadline, agentsInvoked,
      turns}`, `escalatedAt`, `closeReason`, `brief` (MaxLength=512, D-I).
- [x] 2.3 `PipelinesForSource` → claimants of both kinds (D-B); every call site
      iterates claimants; `Wired` counts both; bare-chat choice list names
      both.
- [x] 2.3b `internal/addressing` + `HandleCommand` resolve `/<name>` across
      Pipeline and Coordinator; `/pipelines` and the choice list carry both
      kinds' addressed forms; an addressed conversation binds the origin
      surface only.
- [x] 2.4 Coordinator reconciler: `Ready` per D-B, message lists failing entry
      names, and a name a Pipeline also holds; a not-Ready Coordinator claims
      nothing. The Ready check RESOLVES `coordinatorRef` nesting recursively
      (own capability, source/channel refs, every `agents[]` entry, and —
      through the same function — each nested Coordinator's own Ready), with
      a visited-set guard reporting a STATIC cycle by name rather than
      recursing into it again. `invoke`-time claiming and the live cycle
      guard (D-E2) are a later commit.
- [x] 2.5 Conversation creation from a Coordinator: no `channelRefs`; its OWN
      limits snapshotted into `status.budget`; the Coordinator's `channelRefs`
      snapshotted into `spec.escalationChannelRefs` on an UNCAUSED root only —
      a member never binds it.
- [x] 2.6 Manager `/coordinate/*` surface: `invoke`, `close`, `escalate`,
      `read`; caller token context `coordinator:<name>:<conversation>`;
      `agents[]` list and the CALLING CONVERSATION'S OWN SUBTREE scope
      enforced HERE (D-F). `invoke` returns created|attached and REFUSES a
      target whose Coordinator already appears among the caller's `causedBy`
      ancestors (D-E2, cycle guard). A `channel-reader:<channel>` token gets
      the `{name, title, brief, phase, pipeline}` projection of conversations
      bound to that channel and is refused every verb — decided here, from
      the context alone.
- [x] 2.7 Member creation: `causedBy` set to the invoking PARENT (one hop), no
      channels, capability from the listed AgentCapability, reuse scoped by
      `causedBy` in `reusableBy`. A member MAY itself carry `coordinatorRef`
      when the invoking Coordinator's `agents[]` entry names it by
      `coordinatorRef` rather than `capabilityRef` (nesting).
- [x] 2.8 `handleWorkDone` on a member appends the result input on its PARENT
      (`causedBy`, one hop) in the same status write (D-C); dedup key
      `member:<conv>:<runId>`; closed parent → skip. Reconciler backstop
      re-derives a missing append.
- [x] 2.9 `/channel/inbound` refuses an input whose origin surface is the
      target conversation.
- [x] 2.10 Budget edges per D-E, evaluated on EACH Coordinator's own
      conversation independently: `maxAgents` in the invoke handler,
      `maxTurns` in `handleWorkDone`, `deadline` via reconciler requeue.
      Closing a conversation closes its members with the same `closeReason`,
      kept verbatim, recursively.
- [x] 2.10b Cycle guard (D-E2): on `invoke`, collect the caller's own
      `coordinatorRef` first, then walk its `causedBy` chain to the uncaused
      root collecting each ancestor's `coordinatorRef`. Refuse naming the
      repeated Coordinator when the target matches one already in that list.
      No depth limit.
- [x] 2.11 `closeReason` stamped by the `close` verb (required there), absent
      from `/close`; a coordinator cannot close outside conversations it
      directly caused, except closing itself, always allowed.
- [x] 2.12 Regenerate deepcopy and CRDs; `chart/crds/agentops.dev_coordinators.yaml`.
- [x] 2.13 Tests: envtest — fan-out counts a Coordinator; member result lands
      on its parent exactly once across a simulated restart; self-input
      refused; each of the three limits closes with reason and members, per
      level; reuse scope; a channel-reader token sees only its channel's
      projection and no verb; a nested Coordinator's budget is independent of
      its ancestor's; a direct and an indirect cycle are both refused.
- [x] 2.14 `brief` (D-I): `/work/done` accepts `brief`; `handleWorkDone`
      records it latest-wins in the run's status write, leaving it alone when
      absent; `dispatch/templates/format.md` asks the agent for one sentence
      of what the conversation is about; `runtimes/claude` and
      `runtimes/ollama` extract and report it as they do the context handle.
      Tests: absent leaves stored; present replaces; never parsed from
      `result`.

## 3. Phase 2 — escalation (design D-D)

- [x] 3.1 `escalate` on an UNCAUSED conversation (no `causedBy`) sets
      `spec.channelRefs` from its `spec.escalationChannelRefs` snapshot —
      reads no Coordinator — stamps `escalatedAt`, enqueues `ensure-topic`
      with the digest as the opening message.
- [x] 3.1b `escalate` on a conversation carrying `causedBy` opens NO thread:
      closes it with the message as `closeReason` and result, landing on its
      PARENT as an ordinary member-result input (task 2.8's path) — bubbling
      one hop, exactly as an agent-initiated close does.
- [x] 3.2 `DeliverInputs` fences on `escalatedAt`: nothing earlier is
      delivered to the escalated channels.
- [x] 3.3 `budget-exceeded` closes the conversation, then calls `escalate`
      with a manager-written digest (limit, counts, member list) — bubbling
      per 3.1b on a nested conversation, opening a thread only on the
      uncaused root.
- [x] 3.4 Fake-chat integration test: escalate on the uncaused root opens
      threads with the digest only; a later member result reaches the
      thread; a person's reply is a root input; escalate on a nested member
      closes it and lands the message on its parent with no thread anywhere.

## 4. Phase 3 — `platform/mcp-aops` (design D-F)

- [x] 4.1 New module, standard library only, shared Dockerfile recipe;
      `.github/components.sh` derives `mcp-aops`; multi-arch (built through the
      shared `linux/amd64,linux/arm64` recipe — not hand-verified on hardware
      this session, per every other component built from it).
- [x] 4.2 MCP over streamable HTTP; tools `list_agents`, `list_conversations`,
      `get_conversation`, `get_tree`, `invoke`, `close`, `escalate`, `read`;
      each forwards the caller's `AOPS_MCP_TOKEN` to `/coordinate/*`.
      `list_conversations` carries `brief`; under a channel-reader token the
      server returns whatever projection the manager answered with and adds
      nothing.
- [x] 4.3 Runtime pod build injects `AOPS_MCP_TOKEN` for conversations whose
      capability binds the aops MCPConfig; derived, never stored.
- [x] 4.4 Chart: Deployment, Service, NetworkPolicy under the ADR 0001 wall,
      RBAC (none beyond the floor), `coordination.enabled` gate,
      `global.builtinToolsets.agentops-coordinate`, the rendered `MCPConfig`;
      NOTES.txt line.
- **4.5 (the live-cluster smoke this phase owes) moved to the trailing E2E
      tests section below**, per `docs/testing.md`'s tier split — a cluster
      decides it, envtest does not.

## 5. Phase 4 — console (design D-G)

- [x] 5.1 Adapter watches `agentcapabilities` and `coordinators`; chart Role grants
      list/watch on both.
- [x] 5.2 Inventory rows and topology nodes for both kinds; `capabilityRef` and
      `agents[]` edges; unwired AgentCapability rendered distinct from misconfigured.
- [x] 5.3 Incident view on a root: one timeline, members interleaved,
      expandable; member transcript links to its root; list groups by root
      with a flatten toggle; un-escalated closures marked with `closeReason`.
- [x] 5.4 Fixture gains one root with three members; screenshot the view per
      `visual-check.md` and READ the PNG before ticking.

## 6. Rules and vocabulary

- [x] 6.1 `.claude/rules/wiring.md`, three claims, each named: "no other CR
      carries wiring" → two wiring kinds; the `#### Capabilities are wiring,
      exclusively` header and its section → capabilities are declared on an
      AgentCapability OR inline on a Pipeline/Coordinator, and reached through wiring
      only; under `### MCPToolset`, "Bound from `Pipeline.spec.toolsets` ONLY"
      → bound from any capability's `toolsets`, never a profile's. Deleting
      a Coordinator cascades nothing, stated beside the Pipeline rule.
- [x] 6.2 `terminology.md`: the "Agent is TAKEN" entry names `AgentCapability`
      and why the CRD is not `Agent`; `Coordinator`, `root`, `member`, `escalate`;
      `structure.md`: `platform/mcp-aops`; `invariants.md`: the loop refusal,
      the budget, "a caused conversation binds no human channel".
- [x] 6.3 `retired-vocabulary.json`: no new term — nothing is retired.

## 7. Unit tests

- [x] 7.1 Every module builds, vets and tests in the container, from the
      worktree path. Verdict: green, all modules including the new
      `platform/mcp-aops`. `platform/context-sync` carries two pre-existing
      failures unrelated to this change (unchanged since #221, reproduced on
      `origin/master` alone).
- [x] 7.2 `KUBEBUILDER_ASSETS` envtest suite green.
- [x] 7.3 `python3 .github/scripts/publication-guard.py` and
      `retired-vocabulary-guard.py` pass; record the verdict only. Verdict:
      both clean.
- [x] 7.4 `helm template` with and without `coordination.enabled`;
      `serviceaccount-guard.py` passes. Verdict: clean, both network-policy
      shapes. CI's permutation matrix gained `coordination` and
      `coordination-netpol` rows (mcp-aops rendered in no other permutation,
      same gap `mcp-servers` closed) and `coordination.enabled=true` joined
      `everything`.

## 8. E2E tests

- [ ] 8.1 Smoke against a live install from the worktree chart (moved from
      4.5): a Coordinator with one AgentCapability, a signal, `invoke`
      observed creating a member, result landing on the root, `escalate`
      opening a Telegram thread. Record the verdict, not the transcript. NOT
      RUN in this remote session — no local cluster here
      (`remote-session.md`). `gh workflow run e2e-smoke.yml --ref
      change/coordinated-agents` was dispatched and PASSED (run 36637172129),
      which confirms the chart deploys with `coordination.enabled` and the
      existing smoke suite stays green — it does not exercise this
      scenario's specific invoke/escalate path, which the pack's fixed
      lanes do not cover yet. Left for a workstation session, or a follow-up
      change adding a coordination lane to `platform/manager/test/e2e/`.

## 9. Documentation — THE LAST TASK, and it is not optional

### 9a. Reference docs

- [x] 9a.1 `docs/concepts.md`: kind table (thirteen), `AgentCapability`, `Coordinator`,
      `causedBy`, the loop, escalation, budget, `status.brief`, the state
      matrix rows.
- [x] 9a.2 `docs/contracts.md`: `/coordinate/*`, `/work/done` root routing and
      `brief`, `/channel/inbound` refusal, the aops MCP tool contract with its
      two reach classes, the five token derivation contexts.
- [x] 9a.3 `docs/console.md` and `docs/console-guide.md`: the incident view.
- [x] 9a.4 `docs/security.md`: the agent-invokes-agents flow; re-run
      `python3 docs/diagrams/threat-model.py`.
- [x] 9a.5 `docs/installation.md`: `coordination.*` values, the component.
- [x] 9a.6 `docs/CHANGELOG.md`: two CRDs to apply by hand, the new component.
- [x] 9a.7 `docs/adr/0002-coordinated-agents.md`: status → Accepted, plus a
      "What implementation changed" section as 0001 carries.
- [x] 9a.8 Re-run `python3 .github/scripts/docs-generate.py`; commit every
      regenerated block and `docs/cr-reference.md`.

### 9b. Adopter site

- [x] 9b.1 `docs/index.md`: kind count, the "what you write" tab mentions a
      Coordinator.
- [x] 9b.2 `docs/introduction.md`: two wiring kinds over one capability.
- [x] 9b.3 `docs/installation.md`: component list.
- [x] 9b.4 `docs/guides/coordinate-agents.md`: new guide with generated CR
      blocks; `_data/nav.yml` line.
- [x] 9b.5 `README.md`: kind table, one line under the seams; stays ≤ 215
      lines.
- [ ] 9b.6 `platform/console/ui`: re-run BOTH `npm run screenshots` and
      `npm run demo`; commit the assets. NOT RUN in this remote session — no
      local cluster or port-forwarded console here (`visual-check.md`,
      `remote-session.md`). The fixture also needs extending first (design
      D-G: a root with three members, one itself a sub-coordinator) before
      either command would show anything coordination-related. Left for a
      workstation session.
