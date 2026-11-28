## Context

See `proposal.md` for the motivation. Verified on master `45615bb0`:

- `ConversationRecord` carries `Name`, `ID`, `CreatedAt`, `NameBase` and `ConversationID` (`internal/state/record.go`). `ID` is minted by the backend at `Create`: `metadata.uid` on `kube`, `nuid.Next()` on `nats`.
- The store key, the pod name `agentops-conv-<name>` and `AllocateName`'s create-conflict protection all rest on the name. `nextSuffix` reissues a freed highest suffix.
- `Collection[T].Create` returns only an error and a revision, so `AllocateName` hands its three callers (`chat/router.go`, `chat/coordinate.go`, `signalroute/conversation.go`) a record with an empty `ID`.
- `ops.go` `enqueueEnsureTopic` already refuses an empty `ID` on a receipt channel, and `ReceiptThreadID` already uses `ID`.

## Goals / Non-Goals

**Goals:**
- One identifier, the `ID`, used wherever the system refers to a conversation.
- A rename that cannot be confused with the identity: the signal key becomes `correlationId`.
- An adapter wire field for the `ID` that cannot be mistaken for the name or the correlation key.

**Non-Goals:**
- Keying the store by `ID`, changing allocation, or migrating records. All three are in `proposal.md`.
- The workspace path. `multi-repo-workspace` (PR #507) owns it, see Decision 8.

## Decisions

### 1. The identity is the existing `ID`

`ID` already has the right life: minted at create, new for each life of a name, never settable by a caller (`A record carries a backend-minted identity`). No new field and no second minting site. The work is making the system use it.

Alternative considered: mint the identity in the manager before `Create`. Rejected, since it makes the manager a second minting authority beside the backend and `ID` would then be settable, contradicting the existing requirement.

### 2. `Create` returns the stored record

The signature becomes `(T, Revision, error)`. A backend returns what it stored: `kube` the object returned by the API server, `nats` the value it wrote, with the minted `ID` and `CreatedAt`. A conflict returns the zero `T`.

- It applies to every `Collection[T]`, in `kube/store.go`, `kube/part.go`, `kube/adapterstate.go` and `nats/collection.go`.
- The conformance suite asserts it once, walked over every field of `Stores`, so no kind is skipped.
- `AllocateName` returns the record, and its three callers hold one with `ID` populated.
- The 34 non-test `Create` callers adapt. A caller with no use for the record discards it.

Alternative: leave `Create` alone and have the three callers `Get` after. Rejected. It is a second round trip on the hot creation path and lets a caller forget to.

### 3. The wire field is `conversationId`

Three words now sit near each other on an op:

| Field | Is |
|---|---|
| `conversation` | display name, as today |
| `conversationId` | the system identity, the `ID` |
| `correlationId` | the signal's adopted key, renamed from the old `conversationId` |

`conversationId` is the natural name for the conversation's identifier, and the contract has no vendor word in it. The field changes meaning, so the contract version moves (Decision 4) and an adapter on the old version is refused. It is never served a `conversationId` that quietly means something else.

Rejected:

- `conversationUid`: embeds a Kubernetes word in a public contract.
- `id`: ambiguous beside the op's own id.
- `conversationRef`: reads as a reference to something else.

The value stays opaque to the adapter. The same `ID` is `CONVO_ID` and the pod label.

Adapters point back by `threadId` on `inbound` and `read`, so those requests need no new field. An adapter uses `conversationId` only to key its own per-conversation state.

### 4. The wire `contract` moves from `2` to `3`

Moving the correlation key to `correlationId` changes what a field adapters read means. Compatibility rules call that MAJOR. The rename moves its meaning to `correlationId` and the same key now holds the identity. Carrying on at `2` would hand an old adapter a `conversationId` that quietly means something else. At `3` it is refused with the existing `400` naming the version to build against, an explicit error. `channels/telegram` and `platform/console` move together with the manager.

The document moves MAJOR for the same reason. The signal contract's wire is unchanged, since `initiationId` is untouched, and only its prose is renamed.

### 5. Name resolution and "live"

A typed name resolves to the live record with the newest `CreatedAt`. LIVE means present in the store with no deletion marker. A Closed conversation is live.

Phase is judged by the verb after resolution, not during it, so the answer to `/exit <name>` on a Closed conversation is "closed", never "not found". `conversation-identity` lists the class of each verb.

Since the store key is the name, at most one record carries a name at a time. A record held by a finalizer keeps its key until it is released. In practice the "newest" clause is the tie-break for a backend that ever admits two records of one name, and resolution today is a `Get` by name followed by the deletion check. The spec states the general rule so a backend change cannot make the answer ambiguous.

### 6. Cross-references hold the `ID`

`causedBy.parent` and any other stored reference to a conversation hold its `ID`. A reference resolves by `ByID`, so a deleted parent stays deleted when a new conversation takes its name. A display name is resolved at read time and is never stored as the reference. Reuse scoping (`conversation-provenance`) compares `causedBy` for equality, which holds unchanged for `ID`s.

### 7. The pod label

`agentops.dev/conversation-id=<ID>`. Pod events map to a conversation through the label and `ByID`, not through trimming the pod name. A value that is not label-safe would fail pod creation, so label-safety (at most 63 characters, `[A-Za-z0-9._-]`, alphanumeric ends) is a requirement on every backend, asserted by the conformance suite. NUID is 22 characters and a UUID is 36, both inside it.

### 8. Independence from `multi-repo-workspace`

That change (PR #507, read-only reference) moves the mount to `/workspace` and modifies `runtime-contract`'s `The specification states both parties` and `The specification is exhaustive`, and `runtime-workspace-persistence`'s `Agent session persistence is the shipped default` and `The workspace volume is declared on the wiring, not the runtime`.

This change touches none of those headers:

- `runtime-workspace-persistence`: one ADDED requirement, `A workspace subdirectory is named by the conversation's ID`. It names the subdirectory and says nothing of the mount path.
- `runtime-contract`: one ADDED requirement, `CONVO_ID carries the conversation's identity`. The `always present` table row naming `CONVO_ID` lives in `The specification is exhaustive` and is left alone, since that requirement is theirs. The row stays accurate because it lists the name of the variable only.

The two changes are independent. Both may land in either order, and neither edits the other's requirements. Each states a different fact, the subdirectory's name here and the mount path there. The `docs/contracts/runtime.md` prose is the one file both touch, and the later change to land reconciles it.

### 9. Workspace reclamation follows the `ID`

Housekeeping's orphan rule (`conversation-housekeeping`) matches a directory against a conversation by `ID`. Matching by name would let a new conversation under a reused name shield a deleted conversation's directory from reclamation, and the directory is `ID`-named now.

### 10. The `ByConversationID` index becomes `ByCorrelationID`, and `ByID` is added

The wire name of the index moves from `conversationId` to `correlationId`, and a new index named `id` is added on conversations in both backends. `Collection[T]` has no free-form selector, so the index is a contract change with a conformance scenario (`state-store`).

## Risks / Trade-offs

- **[A missed `conv.Name` use keeps the hazard]** The proposal counts 130 uses → each is reviewed and classified as system reference or display. A test creates, deletes and recreates a name and asserts workspace, op ids, `causedBy` and pod label all differ.
- **[A caller reads `ID` from a record it did not get from `Create`]** → the `Create` contract returns the record, and `enqueueEnsureTopic`'s empty-`ID` refusal stays as the backstop.
- **[An adapter declaring `contract` 2 is refused after upgrade]** → deliberate, see Decision 4. The adapters in this repository ship in the same release, and the changelog names the bump.
- **[Existing conversations are unusable after upgrade]** → no migration, per `proposal.md`. The record without an `ID` is reported with the instruction to delete it, and the chart major version and upgrade notes say so.
- **[A pod of a deleted conversation outlives it]** → its `convo` poll is refused by `ID` and it is not adopted by a same-named conversation.
- **[Conflict with `multi-repo-workspace`]** → none of its requirement headers is modified here. If an unforeseen overlap arises in `docs/contracts/runtime.md`, the later landing resolves it there. The spec files do not collide.
- **[An adapter reading `conversationId` after upgrade gets a different value]** → deliberate and guarded by Decision 4: the contract version refuses an adapter that has not moved.

## Migration Plan

1. Delete conversations on the installed release.
2. Upgrade the chart. It is a major version, since a CRD field and a wire field are renamed. The CRDs are applied first (Helm never upgrades an existing CRD).
3. Rollback is the previous chart. A conversation created on the new release carries `spec.correlationId`, which the old CRD would prune, so delete conversations again before rolling back.

## Open Questions

- None that change the specs. The retired-vocabulary entries for `conversationId` (as the signal key) and for "the conversation name is its identity" are written in the documentation task.
