## 1. Containment: reap runtime pods that never start

- [x] 1.1 Add `RUNTIME_START_DEADLINE` to the manager's env config in
  `cmd/manager/main.go` with a default generous enough for a large image pull,
  and thread it onto `ConversationReconciler`
- [x] 1.2 In `Reconcile`, beside the existing `PodSucceeded || PodFailed` reap at
  `conversation_controller.go:171`, detect a pod past its deadline that has not
  reached `Running` and delete it
- [x] 1.3 Leave `liveRuntimePods` UNCHANGED — `Pending` still counts. Add a test
  pinning that a stuck pod inside its deadline does not free a slot
- [x] 1.4 Record consecutive start failures per conversation and back off before
  re-admitting, so a permanently broken volume does not hot-loop pod creation
- [x] 1.5 Extract the pod's own failure evidence (unmet pod condition plus the
  most recent related event) into a reusable helper, and classify it as
  storage-attributable or not
- [x] 1.6 Add the `RuntimeStarted` condition to `Conversation.status`, message
  carrying that evidence verbatim; regenerate deepcopy and CRDs
- [x] 1.7 Emit a Kubernetes event on the Conversation for the same transition
- [x] 1.8 Integration test: a pod that never starts is reaped, the slot frees,
  the FIFO-first waiter is promoted, and the condition names the attach failure

## 2. Containment: the breaker gains the provisioning edge

- [x] 2.1 Generalise `internal/httpapi/continuity.go` to accept a
  storage-attributable provisioning failure as a report, keeping the existing
  window and threshold semantics
- [x] 2.2 Ensure non-storage failures (image pull, unschedulable) never count —
  test both directions explicitly
- [x] 2.3 While the breaker is open, `admit` provisions nothing and conversations
  hold in `Pending` with a reason distinguishing storage from capacity
- [x] 2.4 Add the single-canary probe that closes the breaker, rather than every
  waiting conversation retrying
- [x] 2.6 Confirm no signal is emitted anywhere on this path — pin it with a test,
  since this is the loop the self-exclusion rules exist to stop

## 3. Console: a stalled queue explains itself

- [x] 3.1 Render `RuntimeStarted` and the storage-held Pending reason on the
  conversation list and detail views
- [x] 3.2 Surface the install-wide "storage is out" fact where an operator looks
  for "why is nothing running". NOT `AgentRuntime` readiness — nothing writes
  that status, as 2.5 and 5.4 both found. It comes from the breaker via
  section 8's metric, so this task depends on 8.6
- [x] 3.3 Re-run `npm run screenshots` in `console/ui` — the site's screenshots
  are build output and the change is not done until they match

## 4. Drain-aware release

- [x] 4.1 Watch `Node` for `spec.unschedulable` and no-schedule taints; map to
  the conversations whose runtime pods are on that node. Gated behind
  `rbac.drainAware` (default OFF) — nodes are cluster-scoped, and this is the
  manager's ONLY cluster-scoped grant
- [x] 4.2 ~~Exclude cordoned nodes from admission~~ — NOT NEEDED. Kubernetes
  already refuses to schedule new pods onto a cordoned node, so writing this
  would duplicate the scheduler
- [x] 4.3 Release idle pods there via the same path as `/exit`, reusing
  `dispatch.NeedsWorker` — not restated
- [x] 4.4 Leave inflight runs alone, for the reason `/exit` already refuses them
- [ ] 4.5 ~~Make `housekeeping/` respect a cordoned node~~ — DEFERRED, and
  probably empty. It is a CronJob, so the scheduler already keeps it off a
  cordoned node, and a run already in flight cannot learn that a drain started.
  Revisit only if it is observed holding a volume through a reboot
- [x] 4.6 Document the limitation honestly: this shrinks the window, it does not
  close it (in `chart/values.yaml` and `internal/controller/drain.go`)
- [x] 4.7 CONDITION TAINTS ARE NOT DRAINS. `node.kubernetes.io/not-ready`,
  `unreachable` and the pressure taints are applied from node CONDITIONS.
  Reading them as a drain releases pods during a transient NotReady — across
  many nodes at once during a partition. Excluded explicitly and pinned by test;
  found because envtest nodes carry `not-ready` and the first implementation
  released a pod on a healthy node
- [x] 4.8 Chart render tests pinning both shapes, and that the behaviour and its
  ClusterRole ship together — enabling one without the other yields a manager
  that only fills the log with forbidden errors

## 5. API: contextSync declaration

- [x] 5.1 Add `AgentRuntime.spec.contextSync` — `paths`, `exclude`, `interval`,
  `retain` — with `paths` an include list relative to `HOME`
- [x] 5.2 Add `Conversation.status.contextCheckpoint` — time, generation,
  quiesced
- [x] 5.3 Regenerate deepcopy and CRDs into `chart/files/crds`
- [x] 5.4 Validate `contextSync`: present means `paths` required and
  well-formed, absent means today's behaviour. NOT on the `AgentRuntime` Ready
  condition as originally written — nothing writes `AgentRuntimeStatus` at all,
  so that would have meant inventing a reconciler to hold one condition, which
  is the same wall task 2.5 hit. Enforced instead where machinery exists: the
  CRD SCHEMA rejects structural errors at admission (stronger than reporting
  them afterwards), and `ResolveFor` rejects what a schema cannot express — an
  absolute path, or one escaping the runtime's home, which would have the sync
  process copy files the agent container is deliberately denied. That error
  surfaces on the Conversation's `RuntimeStarted` condition
- [x] 5.5 Document in `docs/concepts.md` beside `contextStorage`, stating why the
  runtime declares it and the chart cannot infer it

## 6. The context-sync module

- [x] 6.1 New module `context-sync/` with its own `go.mod` and no dependencies
  outside itself, matching the adapter modules
- [x] 6.2 Work-contract proxy: forward `GET /work` (25s long-poll, correct
  pass-through and cancellation) and `POST /work/done` to the real control URL
- [x] 6.3 Restore on start, before answering the first `/work`
- [x] 6.4 Checkpoint on `/work/done` BEFORE forwarding, so a recorded handle
  always has its bytes
- [x] 6.5 Stat-walk manifest of `(path, size, mtime)` over the local store; skip
  the checkpoint when unchanged
- [x] 6.6 Incremental copy against the previous generation — unchanged files
  become HARDLINKS, so a checkpoint after one edited transcript writes exactly
  one file. Implemented in Go rather than by shelling out to
  `rsync --link-dest` as originally written: it is about fifty lines, and it
  keeps the module dependency-free and its image `distroless/static`, matching
  every other component here. Shelling out would have put a tool that must be
  present and correct between the context and the volume
- [x] 6.7 Atomic generation directories plus a `current` symlink swapped by
  rename; retain N
- [x] 6.8 Tag each generation `quiesced` or `bestEffort` from proxy state (work
  handed out, completion not yet seen)
- [x] 6.9 Periodic timer on `CONTEXT_SYNC_INTERVAL`, debounced against the last
  checkpoint; `0` means work boundaries only
- [x] 6.10 Final checkpoint on SIGTERM, within the termination grace period
- [x] 6.11 Report each operation to the manager over its reporting endpoint
- [x] 6.12 Dockerfile and image build, added to the image list in `CLAUDE.md`

## 7. Pod shape

- [x] 7.1 Give the durable claim a per-conversation `SubPath`, as workspace
  already has — but ONLY on the sidecar's mount, not on every install's home.
  Scoping it that way means an install that has NOT opted in keeps its context
  exactly where it is. CORRECTED AFTER A LIVE RUN: this does not remove the
  migration, it moves it to the moment of opting in. The layout changes from
  claim-root to per-conversation, so every conversation holding a context handle
  FAILS its next run rather than answering without memory. That is the
  continuity rule working, and the reset verb (9.5) is the recovery. Documented
  in CHANGELOG and concepts.md
- [x] 7.2 When `contextSync` is set: agent container's `/data/home` becomes an
  emptyDir with a `sizeLimit`, and the PVC moves onto the sidecar only
- [x] 7.3 Build the sidecar as a native sidecar (init container with
  `restartPolicy: Always`) so it terminates with the pod
- [x] 7.4 Point the agent container's `CONTROL_URL` at the sidecar on localhost
- [x] 7.5 Raise `terminationGracePeriodSeconds` enough for a final checkpoint
- [x] 7.6 When `contextSync` is absent, build exactly today's pod — pin with a
  test

## 8. Telemetry

- [x] 8.1 Add `context.restore`, `context.checkpoint`, `context.skip` and
  `context.failed` event kinds to `internal/activity`, carrying duration and
  bytes
- [x] 8.2 Add the manager endpoint the sidecar reports to, authenticated like the
  rest of the work contract
- [x] 8.3 Confirm metrics follow with NO second instrumentation pass — the
  registry is already an `Observer`
- [x] 8.4 Write `status.contextCheckpoint` only when data was actually
  transferred; pin that a skip performs no patch
- [x] 8.5 Render the events on the console Sequence tab
  (`console/ui/src/pages/Conversation.tsx`, eventKey 3)
- [x] 8.6 Surface the OPEN BREAKER as install-level telemetry: a metric for
  "context storage is being treated as unavailable", with the time it opened.
  Moved here from section 2 — it was specified as `AgentRuntime` `Ready=False`
  and there IS no AgentRuntime reconciler, so honouring it literally meant
  inventing a controller whose only job was to hold one condition. A metric is
  where an install-wide fact belongs, and the registry already observes it
- [x] 8.7 Show it in the console where an operator asks "why is nothing
  running" — the per-conversation `StorageUnavailable` reason already exists,
  this is the one place that says the SUBSTRATE is down rather than making the
  reader infer it from many conversations saying the same thing

## 9. Resilience: detection and the loss verb

- [x] 9.1 A periodic MOUNT PROBE, not a read-only filesystem check. The open
  question resolves against the original idea, because the original idea cannot
  work: this corruption class is only detectable AT MOUNT TIME. A consumer that
  can read the filesystem is a consumer whose mount already succeeded, which
  means fsck already passed — so no scan from inside a mounted tree can ever
  find it. What CAN detect it is attempting to mount at all. Shipped as a
  chart-only CronJob (`agentops-context-probe`), default off, that mounts the
  claim and reads it: a volume that will not attach leaves the job unable to
  start, and the deadline fails it. No new image, no new module
- [x] 9.2 Report its result as status, never as a signal. Named `agentops-*` so
  `signal-k8s-events`' NAME-PREFIX self-exclusion catches its failures — the same
  mechanism, and the same reason, that `agentops-housekeeping` relies on. A
  failed probe is visible as a failed Job and in metrics, and reaches no agent
- [x] 9.3 When storage is established unusable, create the runtime pod WITHOUT
  the home mount and mark the conversation context-lost
- [x] 9.4 Make the conversation state the loss on its bound threads, as a typed
  message — no transport dialect in `internal/`
- [x] 9.5 Add the explicit operator-initiated reset that clears the context
  handle and keeps the conversation and threads
- [x] 9.6 Pin that reset NEVER happens automatically on a failed continuation

## 10. Chart

- [x] 10.1 Add `runtime.contextSync.*` values, defaulting to OFF
- [x] 10.2 Ship the reference runtime's `paths` once confirmed empirically
- [x] 10.3 Add the sidecar image reference and bump the chart version
- [x] 10.4 Render `contextSync` onto the `default` AgentRuntime in
  `chart/templates/runtime.yaml`
- [x] 10.5 Chart render tests pinning both the off and on shapes

## 11. Migration

- [x] 11.1 ~~Write the home-layout migration~~ — NOT NEEDED. Scoping the
  per-conversation subPath to the sidecar's mount (7.1) means no existing
  install's layout changes at all. Opting in starts a fresh durable copy; the
  old root content is left untouched for an operator to remove or ignore
- [x] 11.2 ~~Dry-run the migration~~ — moot with 11.1 dropped
- [x] 11.3 CHANGELOG entry, newest first, with the exact steps and the ordering
  that matters
- [x] 11.4 Verify rollback: clearing `contextSync` restores the direct mount and
  the durable copy stays readable as a plain tree

## 12. Documentation

- [x] 12.1 `docs/concepts.md` — `contextSync`, the checkpoint status field, the
  context-lost state and the reset verb
- [x] 12.2 `docs/contracts.md` — the proxied work contract and the sidecar
  reporting endpoint
- [x] 12.3 `docs/installation.md` — the parent chart's new values, grouped by the
  decision they serve
- [x] 12.4 `CLAUDE.md` — `context-sync` terminology (and that it is NOT a
  manager), plus the invariants: reap-never-exempt, checkpoint-before-report,
  write-only-on-actual-dump
- [x] 12.5 Check the adopter pages for anything this makes untrue
- [x] 12.6 Run the adopter-prose lint over `docs/*.md`

## 13. Verification

- [x] 13.1 `go build ./... && go vet ./...` at the root and in `context-sync/`
- [x] 13.2 Full test suite with `KUBEBUILDER_ASSETS`
- [x] 13.3 Smoke the sidecar in a real pod — a rendered pod is not a running one,
  and this change adds a container whose env var names must be right
- [x] 13.4 Fault-injection: make the volume unattachable and confirm the breaker
  opens, work holds, nothing fails, and recovery drains in FIFO order
