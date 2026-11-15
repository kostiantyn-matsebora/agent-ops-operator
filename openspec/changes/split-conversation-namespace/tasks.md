## 1. Namespace pair in the manager — backend-developer

- [x] 1.1 Add the `Namespaces{Control, Conversations}` value type, `Conversations` defaulting to `Control`
- [x] 1.2 Read `CONVERSATION_NAMESPACE` in `cmd/manager/main.go`, defaulting to `NAMESPACE`, and log the pair at startup
- [x] 1.3 Configure the cache with both namespaces (one entry when equal), keeping leader election in control
- [x] 1.4 Replace the single `Namespace` field with the pair on every struct `main.go` wires (router, reconcilers, HTTP server, orphan sweeper, `signalroute`, control-plane)

## 2. Classify every namespace use — backend-developer

- [x] 2.1 `internal/controller/conversation_controller.go`: pods, MCP ConfigMaps and `liveRuntimePods` use Conversations, every wiring read uses Control
- [x] 2.2 `internal/controller/pipeline_controller.go`: `ensureClaim` creates claims in Conversations
- [x] 2.3 `internal/controller/orphansweep.go`: list and delete in Conversations
- [x] 2.4 `internal/chat`, `internal/httpapi` (`status.go` pod reads included), `internal/signalroute`: pod reads in Conversations, wiring reads in Control
- [x] 2.5 Confirm the Channel, ChannelAdapter, SignalAdapter, SignalSource and Coordinator reconcilers stay entirely in Control
- [x] 2.6 `internal/runtimepod`: pods built for Conversations, `CONTROL_URL` still the manager's FQDN in Control

## 3. The kube store spans two namespaces — backend-developer

- [x] 3.1 `kube.OpenStores` takes the pair: conversations, runs and input blobs open in Conversations, signals, cooldowns, receipts and adapter state in Control
- [x] 3.2 The one-time kube-to-nats import reads each kind from its own namespace
- [x] 3.3 Under kube, the synthetic ownerRef names the `Conversation` CR in Conversations. Under nats, `NoOwnerReference` is unchanged

## 4. Expected-references condition — backend-developer

- [x] 4.1 Collect the Secret and ConfigMap names a conversation's pod will resolve (profile repo auth and env, runtime credential, MCP config `valueFrom` and raw refs, image pull secrets) from the CRs alone
- [x] 4.2 Set a condition on the conversation naming them as expected in the conversations namespace
- [x] 4.3 Confirm no Secret read path exists and the manager Roles carry no `secrets` verb

## 5. Signal adapters and self-exclusion — backend-developer

- [x] 5.1 The SignalAdapter reconciler injects `CONVERSATION_NAMESPACE` as a literal value beside `POD_NAMESPACE`. Channel adapters are unchanged
- [x] 5.2 `signals/k8s-events/selfexclude.go`: mechanism 4 drops every event in `CONVERSATION_NAMESPACE` when it differs from `POD_NAMESPACE`, unconfigurably, with no object read
- [x] 5.3 Add `agentops-mcp-conv-` to the name-prefix rule, so an MCP ConfigMap is excluded under nats, where it carries no owner reference
- [x] 5.4 Reconcile the mechanism numbering in `selfexclude.go`'s comments with the spec

## 6. Console — backend-developer

- [x] 6.1 `platform/console/config.go`: read `CONVERSATION_NAMESPACE`, defaulting to `POD_NAMESPACE`
- [x] 6.2 `platform/console/kube.go`: watch pods in both namespaces. Conversations and Signals stay on `/state/*`

## 7. Chart: namespace, placement, RBAC — deployment-engineer

- [x] 7.1 `values.yaml`: `conversationNamespace: agent-ops-conversations`, `createConversationNamespace: true`, plus one helper resolving the effective namespace
- [x] 7.2 New template creating the namespace with the release labels and `helm.sh/resource-policy: keep`
- [x] 7.3 `rbac.yaml`: split the manager Role into a control Role and a conversations Role (pods, configmaps, conversations, conversationinputs and status, events, PVC `get/list/watch/create`). One Role when the namespaces are equal
- [x] 7.4 `rbac.yaml` and `runtime-rbac.yaml`: every `rbac.runtime.serviceAccounts` entry renders in Conversations with every binding subject naming it there, and the floor account renders in both namespaces, bound to nothing
- [x] 7.5 `pvc.yaml`: every chart claim in Conversations, `resource-policy: keep` kept
- [x] 7.6 `housekeeping.yaml`, `context-probe.yaml`: both CronJobs in Conversations
- [x] 7.7 `context-probe.yaml`: give the CronJob and its pods an agentops `app.kubernetes.io/name`, since `agentops-context-probe` matches no self-exclusion prefix
- [x] 7.8 `state-read-auth.yaml`: render the generated token into Conversations from the same lookup
- [x] 7.9 `deployment.yaml`: pass `CONVERSATION_NAMESPACE` to the manager. `console.yaml`: pass it to the console and add its read-only pods Role in Conversations
- [x] 7.10 `chart/charts/kubernetes`: route accounts and their binding subjects in Conversations, every other object in the release namespace
- [x] 7.11 `Chart.yaml` stays at 19.0.1 here, the bump to 20.0.0 is deferred to release time with the image pins (7.12), when `require-release-docs.sh` checks that Chart.yaml, changelog and docs agree
- [x] 7.12 Image-tag pin deferred to release time, stated reason: this change's own CI has not yet published new manager, console and `signal-k8s-events` tags (still 0.62.0, 0.41.0, 0.6.0). `require-release-docs.sh` refuses the chart tag push unless `Chart.yaml`, changelog and docs agree, so the release re-checks the pin
- [x] 7.13 `NOTES.txt`: print both namespaces and point at the 20.0.0 Upgrade section

## 8. Chart: NetworkPolicy peers — deployment-engineer

- [x] 8.1 `networkpolicy.yaml`: add a `namespaceSelector` to every peer naming runtime pods or the manager across the split, and render the runtime ingress-deny policy in Conversations
- [x] 8.2 `coordination.yaml` (`mcp-aops`), `charts/kubernetes` MCP server policy, `charts/home-assistant` MCP admin policy: admit runtime pods from Conversations
- [x] 8.3 Check the `nats`, `commandbus`, `prometheus`, `pulse` and `telegram` policies, and record which need no change
  - `nats`, `commandbus`, `telegram`: no change (control-plane-to-control-plane peers only)
  - `prometheus`, `pulse`: their own MCP-server policies referenced `agentops-runtime`, so each gained the same `namespaceSelector` as `kubernetes` and `home-assistant`
  - this fix went beyond the task's original premise and is recorded here and in `docs/CHANGELOG.md`

## 9. Unit tests

- [x] 9.1 Envtest runs the reconcilers split, with both namespaces created: a signal-opened conversation's pod and MCP ConfigMap land in Conversations while its wiring is read from Control
- [x] 9.2 Envtest: closing releases the pod and ConfigMap in Conversations, and the orphan sweeper deletes an unrecorded pod there
- [x] 9.3 Envtest: equal namespaces reproduce today's placement exactly
- [x] 9.4 The kube store's conformance suite passes with the pair, Signals in Control and Conversations and ConversationInputs in Conversations
- [x] 9.5 The expected-references condition names every reference kind with no Secret read
- [x] 9.6 The SignalAdapter reconciler injects `CONVERSATION_NAMESPACE` in both layouts
- [x] 9.7 `signals/k8s-events`: mechanism 4 drops an unprefixed claim's event split, is not overridable, and does not apply when the namespaces are equal, and the name-prefix rule drops an `agentops-mcp-conv-` ConfigMap
- [x] 9.8 Console: pods from both namespaces reach the Infrastructure view
- [x] 9.9 Chart render tests: two manager Roles and no new ClusterRole, runtime accounts, claims and both CronJobs in Conversations, the context-probe label, the floor account in both namespaces bound to nothing, namespace `keep`, the single-namespace render matching today's
- [x] 9.10 `TestEveryWiredFlowIsAllowed` and `TestNetworkPolicyIsOptIn` pass in both layouts
- [x] 9.11 Envtest: the manager's cache syncs under the chart's own rendered Roles, impersonating its account, in both layouts (`TestManagerCacheSyncsUnderTheChartsRoles*`). The leader proxy finds the leader's pod in Control under a split (`TestLeaderProxyFindsTheLeaderInControlUnderASplit`)
- [x] 9.12 `go build`, `go vet` and `go test ./...` in every module touched, run against this working copy's tree in the Go container

## 10. System tests

- [ ] 10.1 A system lane on k3d, built from this working copy: `TestSplitConversationPodRunsInConversationsNamespace` (`test/system/namespace_split_test.go`)
  - a split release opens a conversation from an ordinary signal
  - its pod runs in Conversations under a Conversations account, mounts its claim and reaches the manager
  - WRITTEN AND COMPILED ONLY (`go vet -tags system`, `go test -tags system -c`): no docker/k3d here, so it has never run on a cluster
  - dispatch `system-smoke.yml` to confirm pass/fail
- [ ] 10.2 The same lane with NetworkPolicies enforced: every wired flow passes and runtime pods accept no ingress — `TestSplitNetworkPolicyEnforcedRuntimePodAcceptsNoIngress` (same file), FULL TIER ONLY. The no-ingress probe targets the egress-proxy sidecar's own `:15001` listener (the one socket a runtime pod binds wider than loopback) from a control-namespace pod — see the test's own doc comment. WRITTEN AND COMPILED ONLY, not run
- [ ] 10.3 The authorizer denies the manager pods in a third namespace and PVC `delete` in Conversations — `TestSplitRBACDeniesThirdNamespaceAndPVCDelete` (same file), via `SubjectAccessReview` (`Kube.Can`, pre-existing). WRITTEN AND COMPILED ONLY, not run
- [ ] 10.4 A missing Secret in Conversations fails the pod and the condition names it — `TestSplitMissingSecretFailsPodAndConditionNamesIt` (same file). WRITTEN AND COMPILED ONLY, not run
- [ ] 10.5 The migration on a 19.x release: close, place Secrets, PV rebind, upgrade, and a reopened conversation continues its context — ADAPTED: this harness (`install.go`) only ever renders the WORKING TREE's own chart, with no mechanism to pin a published 19.x version, so `TestSplitMigrationFromSingleNamespaceCarriesClaimAndReopens` (`test/system/namespace_migration_test.go`) pins `global.agentops.conversationNamespace` to the release namespace as the pre-split stand-in, then performs the real PV rebind (Retain, delete, clear `claimRef`, pre-create with `volumeName`, `persistence.context.existingClaim`) against a real PV before upgrading onto the split layout. No Secret placement step: this lane's profile resolves none. WRITTEN AND COMPILED ONLY — the PV-rebind mechanics are especially unverified without a real cluster. Dispatch the full tier to confirm
- [ ] 10.6 Run the system pack. NOT PERFORMED: no docker/k3d on this machine
  - `go test -tags system -list` cannot discover tests here: `TestMain` (`harness_test.go`) provisions a k3d cluster before `m.Run()`
  - discovery was confirmed instead by `go vet -tags system ./...` (zero findings) and `go test -tags system -c` (the suite links, including the five new functions)
  - the `conveyor` session driving this PR dispatches `gh workflow run system-smoke.yml --ref change/split-conversation-namespace`
  - the full-tier equivalent for 10.2/10.5 is dispatched the same way

## 11. Documentation

Left unticked until section 10 has run, so the documentation gate follows the system tests.

### Reference docs

- [ ] 11.1 `docs/concepts.md`: the two namespaces, the placement rule and its table
- [ ] 11.2 `docs/contracts.md`: no manager surface carries a namespace
- [ ] 11.3 `docs/contracts/signal-adapter.md` to 2.5.0: `POD_NAMESPACE` and `CONVERSATION_NAMESPACE` in `#environment`, `ADP-10`, `ADP-7` reworded, changelog and conformance rows
- [ ] 11.4 `docs/configuration.md`: `conversationNamespace`, `createConversationNamespace`
- [ ] 11.5 `docs/security.md`: the moved trust boundary, the state-read token beside agent code, re-run `threat-model.py`
- [ ] 11.6 `docs/integrations/kubernetes.md` and `docs/console.md`: route accounts and runtime pods in the conversations namespace
- [ ] 11.7 `docs/CHANGELOG.md` `[20.0.0]`, with the Upgrade section as the migration of record: close, Secrets, PV rebind, upgrade, kube cleanup, verify, rollback. The heading names 20.0.0 ahead of `Chart.yaml`, which stays at 19.0.1 until the release-time bump (7.11)
- [ ] 11.8 `.claude/rules/`: `terminology.md` (control and conversations namespace, the namespace pair), `invariants.md` (the placement rule), `structure.md`, `gotchas.md` — four files edited, with three new `gotchas.md` entries including the egress-proxy-listener probe, taken from the no-ingress test's doc comment (the lane has not run)
- [ ] 11.9 Re-run `python3 .github/scripts/docs-generate.py` and pass `--check` (`docs/installation.md`'s install commands pin `--version 19.0.1`, matching `Chart.yaml` until the release-time bump in 7.11)

### Adopter site

- [ ] 11.10 `docs/installation.md`: both namespaces, Secret placement, the 19 to 20 upgrade mirroring the changelog
- [ ] 11.11 `docs/getting-started.md`: where the first conversation's pod appears
- [ ] 11.12 `docs/index.md` and `README.md`: the isolation claim
- [ ] 11.13 `docs/guides/agent-profile.md`, `agent-runtime.md`, `toolsets.md`: referenced Secrets belong in the conversations namespace
- [ ] 11.14 `docs/guides/pipeline.md`: claims are created in the conversations namespace
