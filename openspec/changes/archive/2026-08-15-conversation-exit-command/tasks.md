## 1. One definition of busy

- [x] 1.1 Export `NeedsWorker(c *Conversation) bool` from `internal/dispatch`
  (`len(PendingInputs(c)) > 0 || c.Status.Inflight != nil`)
- [x] 1.2 Replace the controller's private `needsWorker` with it at every call
  site (`Reconcile`, `admit`, `evictableCount`, `createRuntimePod`), changing no
  behavior
- [x] 1.3 Confirm the existing controller and integration tests still pass
  unchanged — this step is a move, not a semantic change

## 2. Command recognition

- [x] 2.1 Add `ExitCommand = "exit"` and `isExitCommand` beside `CloseCommand` /
  `isCloseCommand` in `internal/chat/router.go`, with the same strictness: bare
  command, bot-suffixed form accepted, any trailing text disqualifies
- [x] 2.2 Intercept it in `HandleMessage` immediately after the `/close` check,
  BEFORE `appendInput`, so the command never becomes an input
- [x] 2.3 Add the general-surface answer in `HandleCommand`: usage text, nothing
  created, nothing released — mirroring the `/close` branch
- [x] 2.4 Extend the router's recognition test table (the one covering `/close`,
  `/close@AgentOpsBot`, and the trailing-text case) with the `/exit` equivalents

## 3. Releasing the runtime

- [x] 3.1 Wire `Runtime runtimepod.Config` into `chat.Router` and set it in
  `cmd/manager/main.go` from the same bootstrap value `httpapi.Server` receives
- [x] 3.2 Implement `Router.ReleaseRuntime(ctx, conv)`: refuse when
  `dispatch.NeedsWorker(conv)`, otherwise delete
  `runtimepod.PodName(conv.Name)` with `IgnoreNotFound`
- [x] 3.3 Inflight refusal: name the run id and offer `/close` for abandoning it;
  no pod is deleted and no inflight state is touched
- [x] 3.4 Queued-input refusal: say the conversation still has work, so the pod
  would be recreated immediately
- [x] 3.5 No-pod case: report that there was nothing to release; not an error
- [x] 3.6 Success replies via `ResolveFor(...).ContinuityPossible()` — one
  wording that says the conversation keeps its context, one that warns the next
  message starts fresh; a resolution error degrades to neutral wording rather
  than failing the command
- [x] 3.7 All four replies as `chat.Notice` / `chat.Warn` in the named markdown
  subset, fanned out through the ops queue; every success reply states that the
  conversation and its thread remain open

## 4. Discoverability

- [x] 4.1 Add one line to the `/agents` reply naming `/exit` and `/close` and the
  difference between them
- [x] 4.2 Test that the `/agents` output mentions both

## 5. Tests

- [x] 5.1 Router unit tests: idle conversation → pod deleted, conversation and
  threads intact, no input appended
- [x] 5.2 Router unit tests: inflight → refused, pod still present, reply names
  the run and `/close`
- [x] 5.3 Router unit tests: queued input → refused, pod still present
- [x] 5.4 Router unit tests: no pod → "nothing to release", no error
- [x] 5.5 Router unit tests: general surface → usage reply, no Conversation
  created, no pod deleted
- [x] 5.6 Continuity wording: one case with a home PVC configured and one
  without, asserting the two different replies
- [x] 5.7 Integration (envtest): at the cap with one `Pending` conversation,
  `/exit` on an idle one admits the pending conversation without waiting out an
  idle TTL
- [x] 5.8 Integration: after `/exit`, a new input recreates a pod and the
  conversation resumes with its context handle unchanged

## 6. Documentation

- [x] 6.1 `docs/concepts.md`: document `/exit` in the capacity section beside
  eviction — what it releases, what it keeps, when it refuses — and note that a
  Pipeline named after a manager command (`exit`, `close`, `agents`, `help`,
  `start`) is not reachable by that command
- [x] 6.2 `docs/contracts.md`: add `/exit` wherever the chat command surface is
  listed
- [x] 6.3 `CLAUDE.md`: record the `/exit` vs `/close` distinction in
  terminology — release the runtime versus end the conversation — and that
  `/exit` refuses mid-run because a mid-run pod delete stalls for a TTL and then
  re-runs the work
- [x] 6.4 `README.md`: unchanged (no pitch, kind list, demo or install change);
  verify it stays ≤150 lines

## 7. Verify

- [x] 7.1 `go build ./... && go vet ./... && go test ./...` in the golang:1.23
  container
- [x] 7.2 Full envtest run with `KUBEBUILDER_ASSETS`
- [x] 7.3 Live check on the cluster: send `/exit` in an idle conversation's
  thread, confirm `kubectl get pods` loses `agentops-conv-<name>` and the
  Conversation survives, then send a message and confirm it resumes
- [x] 7.4 Live check: send `/exit` during a run and confirm the refusal names the
  run and leaves the pod alone
