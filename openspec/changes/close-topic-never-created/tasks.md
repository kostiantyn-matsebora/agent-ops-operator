## 1. Implementation

- [x] 1.1 In `reconcileClosed`, skip `EnqueueCloseTopic` for a thread
      whose `ThreadID` is empty, without marking it `ThreadsArchived`.
      Verified by reading the diff: the channel lookup and the enqueue
      call both moved inside a non-empty-`ThreadID` branch.

## 2. Unit tests

- [x] 2.1 Added an envtest case (`TestCloseTopicSkipsAThreadWithNoTopicYet`,
      `internal/integration/conversation_verbs_test.go`): a `Closed`
      conversation with one bound thread carrying an empty `ThreadID`
      produces no `close-topic` op and no `ThreadsArchived` entry for
      that channel, across two reconcile passes. Verified with
      `KUBEBUILDER_ASSETS=... go test ./internal/integration/... -run
      TestCloseTopicSkipsAThreadWithNoTopicYet` — PASS.
- [x] 2.2 Added `TestCloseTopicResumesOnceATopicArrives` to the same
      file: once the thread's `ThreadID` becomes non-empty (simulating a
      later `ensure-topic` success), the next reconcile pass enqueues
      `close-topic` for it normally, carrying that thread id. Verified
      with the same run — PASS.
- [x] 2.3 Ran the full manager module's existing suite and confirmed no
      regression: `cd platform/manager && go build ./... && go vet
      ./... && go test ./...` with `KUBEBUILDER_ASSETS` set — all
      packages PASS, including `internal/integration` (60.4s).

## 3. System tests

- [x] 3.1 Nothing here is decided by a cluster. The behavior is pure
      reconciler logic over CR status fields, already covered by the
      envtest cases in section 2 — ticked as not applicable.

## 4. Documentation

- [x] 4.1 No reference or adopter-site page describes this
      reconciliation detail. Ticked as not applicable — see proposal.md
      - Impact.
