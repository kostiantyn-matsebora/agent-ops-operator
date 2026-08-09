## 1. API and configuration

- [x] 1.1 Add `ConversationPending ConversationPhase = "Pending"` to `api/v1alpha1/conversation_types.go`, documenting it as "created, awaiting a capacity slot; nothing provisioned"
- [x] 1.2 Regenerate deepcopy and CRDs in the golang container (`controller-gen object` + `controller-gen crd` into `chart/files/crds`)
- [x] 1.3 Read `MAX_ACTIVE_CONVERSATIONS` (default 5) in `cmd/manager/main.go` with `MAX_RUNTIMES` as a deprecated fallback that logs a deprecation line; rename the reconciler field to `MaxActiveConversations`
- [x] 1.4 Read `MAX_QUEUED_CONVERSATIONS` (default 50) in `cmd/manager/main.go` and pass it to the httpapi `Server`
- [x] 1.5 Change the `RUNTIME_IDLE_TTL_M` default in `cmd/manager/main.go` from 10 to 1

## 2. Admission and the Pending phase

- [x] 2.1 Add an admission helper to `internal/controller/conversation_controller.go`: count live runtime pods, return whether this conversation may be admitted (slot free AND no older `Pending` conversation needing a worker)
- [x] 2.2 Move the capacity decision to the top of `Reconcile`, before `ensureTopics`: a conversation needing a worker but not admitted gets phase `Pending` and returns early — no topic op, no MCP ConfigMap, no pod, no dispatch
- [x] 2.3 Keep `Queued` for admitted-but-waiting and keep the existing idle-pod eviction in `createRuntimePod`, re-checking the cap against a fresh pod list before creating
- [x] 2.4 Add a pod watch in `SetupWithManager` mapping runtime pod deletion to the oldest `Pending` conversations, so a freed slot is filled without waiting for the requeue backstop
- [x] 2.5 On transition into `Pending`, enqueue one general-surface send op on the conversation's first bound channel ("queued for capacity"); emit it only on the phase transition
- [x] 2.6 Suppress the queued notice and all provisioning for conversations that need no worker (no pending inputs, nothing inflight)

## 3. Backlog bound at ingest

- [x] 3.1 In `internal/httpapi/signals.go`, count `Pending` conversations before creating a new one in `routeSignalGroup` and refuse creation at `MAX_QUEUED_CONVERSATIONS`
- [x] 3.2 Return the refusal through the existing batch drop-reason path so chat origins are told via `tellOriginatingSurfaces` and alert/job origins are logged and counted
- [x] 3.3 Verify window reuse still appends to an existing `Pending` conversation even when the backlog is full (the bound gates creation only)

## 4. close-topic operation

- [x] 4.1 Add `OpCloseTopic OpKind = "close-topic"` and `EnqueueCloseTopic(ctx, channel, threadID, conversation)` to `internal/chat/ops.go`
- [x] 4.2 Accept an empty completion body for `close-topic` in the ops-done handler; log failures instead of writing a Conversation condition, and do not regenerate
- [x] 4.3 Handle the new kind in `channel-telegram/main.go` — call `closeForumTopic` and complete with an empty result; tolerate an already-closed topic
- [x] 4.4 Handle the new kind in `console/adapter.go` — mark the thread archived in the console UI and keep its transcript for the session
- [x] 4.5 Update the op `Kind` comments in `channel-telegram/manager.go` and `console/manager.go`

## 5. Closing a conversation

- [x] 5.1 Add finalizer `agentops.dev/close-topics` to conversations at admission time (or first reconcile) in `internal/controller/conversation_controller.go`
- [x] 5.2 Handle a non-zero `deletionTimestamp`: enqueue one `close-topic` op per bound thread, wait for completion, and remove the finalizer once all complete or after a 2-minute grace measured from the deletion timestamp
- [x] 5.3 Intercept `/close` in `Router.HandleMessage` before the reply-input append — parse with `addressing.Parse`, match profile `close`
- [x] 5.4 Fan out the farewell message to every bound thread, naming abandoned work when `status.inflight` is set, then delete the `Conversation`
- [x] 5.5 Answer `/close` on a general surface in `Router.HandleCommand` with usage instead of "unknown agent", creating nothing

## 6. Chart

- [x] 6.1 Add `maxActiveConversations: 5` and `maxQueuedConversations: 50` to `chart/values.yaml`; keep `maxRuntimes` documented as deprecated
- [x] 6.2 Change `runtimeIdleTtlMinutes` to `1` in `chart/values.yaml` with a comment on the throughput/latency trade
- [x] 6.3 Wire `MAX_ACTIVE_CONVERSATIONS` and `MAX_QUEUED_CONVERSATIONS` env into `chart/templates/deployment.yaml`, keeping `MAX_RUNTIMES` emitted only when `maxRuntimes` is explicitly set
- [x] 6.4 Bump `chart/Chart.yaml` version and the manager, `channel-telegram`, and `console` image tags so the contract change ships as one unit

## 7. Tests

- [x] 7.1 Unit-test `/close` interception in `internal/chat` — command intercepted, no reply input appended, farewell fanned out, general-surface usage answer
- [x] 7.2 Unit-test the `close-topic` op in `internal/chat/ops_test.go` — enqueue, delivery by adapter name, empty-body completion
- [x] 7.3 Integration test (`internal/integration/`): with the cap at 1, a second conversation reports `Pending` with no pod, no topic op, and no MCP ConfigMap
- [x] 7.4 Integration test: deleting/closing the active conversation admits the pending one in FIFO order and it then gets its topic and pod
- [x] 7.5 Integration test: close deletes the conversation, enqueues `close-topic` for each bound thread, and the finalizer is released after the grace period when no adapter completes
- [x] 7.6 Integration test: backlog bound refuses creation and reports the drop reason
- [x] 7.7 Assert `RUNTIME_IDLE_TTL_M=1` by default and that an `AgentRuntime` override still wins (extend the existing env assertion in `suite_test.go`)
- [x] 7.8 Run the full build/vet/test matrix for the root module and every adapter module in the golang container

## 8. Docs

- [x] 8.1 Document the `close-topic` kind, its empty completion, and its terminal-failure semantics in `docs/contracts.md`
- [x] 8.2 Document the capacity model (active = pod-backed, `Pending`, FIFO admission, backlog bound) and `/close` in `README.md` and `docs/concepts.md`
- [x] 8.3 Update the `CLAUDE.md` map/terminology notes: runtime-pool cap becomes the conversation cap, `Pending` phase, `/close` and topic archiving
- [x] 8.4 Note the default changes (8 → 5 active, 10 → 1 minute idle) as upgrade-visible behavior in the chart values comments
