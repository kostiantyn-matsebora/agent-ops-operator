## 1. Namespace pair plumbing

- [ ] 1.1 Add a `Namespaces{Control, Conversations string}` type (new small package or `internal/controller`), with a constructor that defaults `Conversations` to `Control` when unset
- [ ] 1.2 Resolve both in `cmd/manager/main.go` from `NAMESPACE` and `CONVERSATION_NAMESPACE`; log the pair at startup
- [ ] 1.3 Configure the controller-runtime cache with both namespaces (`DefaultNamespaces`), collapsing to one entry when equal; keep `LeaderElectionNamespace` on the control namespace
- [ ] 1.4 Replace the `Namespace string` field with the pair on `ConversationReconciler`, `httpapi.Server`, `chat.Router`, `chat.OpQueue`

## 2. Reclassify every namespace lookup

- [ ] 2.1 `internal/controller/conversation_controller.go` — conversations, inputs, pods and MCP ConfigMaps use the conversations namespace; profile, runtime, toolset, MCPConfig and channel lookups use the control namespace
- [ ] 2.2 `internal/httpapi/signals.go` — SignalSource/Pipeline/Channel reads control; Conversation and ConversationInput creation and listing use conversations
- [ ] 2.3 `internal/httpapi/server.go` — `/work` dispatch reads the conversation from conversations and its profile/toolsets/channels from control; `/task` creates in conversations
- [ ] 2.4 `internal/chat/router.go` — conversation lookup and creation in conversations; Channel and Pipeline reads in control
- [ ] 2.5 `internal/chat/ops.go` and `internal/chat/pipelines.go` — channel/pipeline reads in control, conversation status patches in conversations
- [ ] 2.6 `internal/controller/*_controller.go` for channels, signal sources, adapters and pipelines — confirm each stays entirely control-plane
- [ ] 2.7 `internal/runtimepod/podspec.go` — build pods for the conversations namespace; verify `CONTROL_URL` still resolves the manager FQDN in the control namespace

## 3. Reference preflight

- [ ] 3.1 Collect the Secret/ConfigMap names a conversation's pod will resolve from `AgentProfile` (repo auth, env valueFrom) and its bound `MCPConfig`s, without reading any of them
- [ ] 3.2 Report the collected names on a Conversation condition stating they are expected in the conversations namespace
- [ ] 3.3 Assert no Secret read path is introduced (manager RBAC still grants no `secrets` verbs)

## 4. Chart: namespaces, RBAC, placement

- [ ] 4.1 Add `conversationNamespace: agent-ops-conversations` and `createConversationNamespace: true` to `chart/values.yaml`, with a helper template resolving the effective namespace
- [ ] 4.2 New template creating and labelling the conversations namespace with `helm.sh/resource-policy: keep`
- [ ] 4.3 Split `chart/templates/rbac.yaml` into a control-namespace Role/RoleBinding and a conversations-namespace Role/RoleBinding, each carrying only what it uses
- [ ] 4.4 Move the runtime ServiceAccount to the conversations namespace; update `chart/templates/runtime-rbac.yaml` binding subjects
- [ ] 4.5 Place the home PVC in the conversations namespace (`chart/templates/pvc.yaml`), keeping the resource policy
- [ ] 4.6 Pass `CONVERSATION_NAMESPACE` to the manager in `chart/templates/deployment.yaml`
- [ ] 4.7 Update `chart/charts/k8s-bundle/` — runtime SA subject follows the split, identity/wiring CRs stay in the release namespace
- [ ] 4.8 Bump `chart/Chart.yaml` and add upgrade notes to `chart/templates/NOTES.txt` (drain step, secret placement, both namespaces printed)

## 5. NetworkPolicies

- [ ] 5.1 Add `networkPolicies.enabled: true` and `networkPolicies.extraEgress: []` to `chart/values.yaml`
- [ ] 5.2 Default-deny ingress policy for the conversations namespace
- [ ] 5.3 Egress policy allowing DNS and the manager Service in the control namespace, plus rendered `extraEgress` entries
- [ ] 5.4 k8s-bundle contributes an egress allowance for its MCP server Service
- [ ] 5.5 NOTES warning that a CNI without policy support ignores these objects

## 6. Console

- [ ] 6.1 Read `CONTROL_NAMESPACE` and `CONVERSATION_NAMESPACE` in `console/main.go`, both defaulting to `POD_NAMESPACE`
- [ ] 6.2 Route the k8s reads in `console/kube.go` per kind — conversations and inputs from the conversations namespace, everything else from control
- [ ] 6.3 Add a read-only Role/RoleBinding for the console SA in the conversations namespace in `chart/templates/console.yaml`, and pass the two namespace env vars
- [ ] 6.4 Bump the console image tag alongside the manager

## 7. Tests

- [ ] 7.1 Extend the envtest suite to create both namespaces and run the reconcilers in split mode
- [ ] 7.2 Integration test: a signal-originated conversation lands in the conversations namespace with its pod, MCP ConfigMap and inputs, while its pipeline/profile/channel are read from control
- [ ] 7.3 Integration test: deleting the conversation garbage-collects its pod and ConfigMap (ownerRef intact across the split)
- [ ] 7.4 Integration test: single-namespace mode (both values equal) reproduces current placement exactly
- [ ] 7.5 Test the preflight condition names the expected Secret references without any Secret read
- [ ] 7.6 Chart render tests: two Roles and no manager ClusterRole; runtime SA and PVC in the conversations namespace; NetworkPolicies present by default and absent when disabled
- [ ] 7.7 Run the full build/vet/test matrix for the root module and every adapter module in the golang container

## 8. Docs

- [ ] 8.1 Document the two-namespace layout, what lives where, and the rule that generates the split in `docs/concepts.md`
- [ ] 8.2 State explicitly in `docs/contracts.md` that adapter and runtime contracts carry no namespace
- [ ] 8.3 Update `README.md` install/upgrade sections with both namespaces, the secret-placement requirement, and the drain step
- [ ] 8.4 Update `CLAUDE.md` — namespace invariants, `Namespaces` pair, where runtime references must exist
- [ ] 8.5 Add an upgrade guide entry covering secret re-creation and the abandoned-conversation cutover
