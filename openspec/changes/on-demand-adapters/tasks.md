## 1. Demand computation (reporting only)

- [ ] 1.1 Add a demand helper for channel adapters: count `Channel`s whose
      `spec.adapter` equals the CR name
- [ ] 1.2 Add a demand helper for signal adapters: `SignalSource`s naming the
      CR, intersected with sources claimed by a Ready `Pipeline` — reuse the
      existing `PipelineForSource` semantics rather than reimplementing the claim
- [ ] 1.3 Add the `Active` condition to both reconcilers: `True/HasDemand`,
      `False/NoServedChannels`, `False/NoWiredSources` — the last naming the
      unclaimed sources in its message
- [ ] 1.4 Add watches: `Channel` → ChannelAdapter; `SignalSource` + `Pipeline` →
      SignalAdapter, mapping back the same way the `Served` mapping already does
- [ ] 1.5 Keep `replicas: 1` hardcoded for now — this step only proves the
      demand signal is correct on a real cluster

## 2. Scale to zero

- [ ] 2.1 Add a desired-replicas input to `adapterWorkload` and stop hardcoding
      `replicas := int32(1)` in `ensureAdapterWorkload`
- [ ] 2.2 Wire both reconcilers to pass 1 on demand, 0 without
- [ ] 2.3 Confirm scaling to zero leaves the Deployment, ServiceAccount, owned
      Service, ownerRefs, and credential projection untouched — only the replica
      count moves
- [ ] 2.4 Confirm deleting the adapter CR still removes workload and Service

## 3. Hysteresis

- [ ] 3.1 Add the idle grace period as a manager env knob with a conservative
      default, documented alongside `MAX_RUNTIMES`
- [ ] 3.2 Scale up immediately on the reconcile that observes demand
- [ ] 3.3 Scale down only after demand has been absent for the grace period,
      driving the countdown from the `Active` condition's `lastTransitionTime`
      and a requeue-after — no in-process timer
- [ ] 3.4 Verify the countdown survives a manager restart: it resumes rather
      than restarting or scaling down immediately

## 4. Shared front door (telegram trio)

- [ ] 4.1 Define the router's demand as the union of the demands it fronts —
      active if its own source is claimed **or** a `Channel` names the channel
      adapter it forwards to
- [ ] 4.2 Verify the channel adapter cannot go deaf: with a Channel present and
      the router's source unclaimed, the router still runs
- [ ] 4.3 Coordinate with `chat-signal-origination` — if that change has not
      landed, land this rule with it rather than leaving a dangling special case

## 5. Tests

- [ ] 5.1 envtest: ChannelAdapter alone → Deployment at 0; add a Channel →
      scales to 1
- [ ] 5.2 envtest: Channel with no Pipeline keeps the adapter at 1 (the
      deafness guard — pin this explicitly)
- [ ] 5.3 envtest: SignalAdapter with an unclaimed source → 0 replicas,
      `Active=False/NoWiredSources` naming the source; claim it → scales to 1
- [ ] 5.4 envtest: port-declaring SignalAdapter at zero keeps its Service, with
      no endpoints
- [ ] 5.5 envtest: delete-and-recreate a Channel within the grace period never
      scales the Deployment down
- [ ] 5.6 envtest: sleeping then waking preserves projected credential envFrom
      entries
- [ ] 5.7 envtest: router stays awake when only the channel side has demand
- [ ] 5.8 Update `internal/integration/channeladapter_test.go` and
      `signaladapter_test.go` — their current "adapter CR ⇒ Deployment exists"
      assertions change meaning; change them deliberately, not incidentally
- [ ] 5.9 Full suite: `go build ./... && go vet ./...`,
      `KUBEBUILDER_ASSETS=… go test ./...`

## 6. Docs

- [ ] 6.1 `CLAUDE.md`: adapter lifecycle now demand-gated, the two demand rules
      and why they differ, and the `Active` vs `Served` distinction
- [ ] 6.2 README / `docs/`: enabling a bundle flag no longer costs a pod; how to
      read `Active=False` and what wakes an adapter
- [ ] 6.3 Note the vm-bundle case explicitly — `alertmanager.enabled=true` with
      `defaultSource.enabled=false` now renders CRs and a Service but no pod
- [ ] 6.4 Bump the chart version; no values or template change is required
