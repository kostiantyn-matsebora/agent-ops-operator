## Why

A conversation has three identifiers and none of them is used as the identity.

| Field | Is | Problem |
|---|---|---|
| `Name` | a readable word chain, `<kind>-<words>[-N]` | the key everywhere (130 uses), yet unique only among live records. `AllocateName` picks the highest live suffix plus one, so deleting `-3` and creating again reissues `-3`. |
| `ID` | opaque, minted by the store at create, new for every life of a name | the right identity, used 6 times. |
| `ConversationID` | the signal's `initiationId`, adopted at creation | a correlation key that many conversations share over time. Its name reads as the identity. |

Reusing a name is not hypothetical. The workspace directory is `subPath = conv.Name` and nothing reclaims it, so a recreated name mounts the previous conversation's tree. Op ids, pod ownership and cross-references carry the same hazard.

The system needs one identifier that is unique for the life of one conversation and used wherever the system refers to it. The friendly name belongs to the people reading a channel or the console.

## What Changes

- **`ID` is the system identity.** It replaces `Name` wherever the system, not a person, refers to a conversation:
  - the workspace `subPath`
  - `CONVO_ID` in the runtime environment
  - op ids and the thread id of a receipt-only channel
  - stored cross-references such as `causedBy`
  - the identifier on the adapter wire contract, beside the display name
- **`Name` is display.** It stays the store key and the pod name (`agentops-conv-<name>`), and it is what a person sees and types.
- **The pod carries its identity.** A label `agentops.dev/conversation-id` holds the `ID`, so the manager can go from a pod to its conversation without trusting the name.
- **A typed name resolves to the most recent live match.** A name that matches no live conversation is not found.
- **`Create` returns the stored record.** Every `Collection[T].Create` hands back the record as stored, with its backend-minted `ID` and `CreatedAt`. `AllocateName` passes it to its three callers, which today return a record whose `ID` is empty.
- **`ByID` index** on conversations.
- **BREAKING: `conversationId` becomes `correlationId`.** The `Conversation` field, the record field, the `ByConversationID` index, the `Op` JSON field and the adapter contract text (OPS-9) are renamed. It keeps its meaning: an opaque, sender-minted equality key.
- **No migration.** Operators delete conversations before upgrading.

Not in scope:
- Keying the store by `ID`. The store key stays the name, so `AllocateName` keeps its create-conflict protection and pod names stay unique.
- Changing how names are allocated or shown.
- The multi-repo workspace layout, which is `multi-repo-workspace`. That change keeps using the workspace `subPath` and takes the `ID` form once this lands.

## Capabilities

### New Capabilities
- `conversation-identity`: the system identity, the display name, name resolution, the pod label and where each is used.

### Modified Capabilities
- `state-store`: `Create` returns the stored record, and a `ByID` index exists for conversations.
- `state-store-nats`: the NATS backend returns the minted `ID` and `CreatedAt` from `Create`.
- `conversation-naming`: the name is display, and the allocator hands the created record back.
- `channel-adapter-contract`: the op's `conversationId` carries the `ID` beside the display name, and the correlation key moves to `correlationId`.
- `signal-adapter-contract`: `conversationId` is renamed to `correlationId`.
- `chat-signal-origination`, `pipeline-model`, `conversation-provenance`, `conversation-capacity`, `conversation-housekeeping`, `console-origination`, `coordination-loop`, `signal-persistence`: the rename wherever each states the field.
- `runtime-workspace-persistence`: the workspace subdirectory is named by the `ID`.
- `runtime-contract`: `CONVO_ID` carries the `ID`.

## Impact

- **Manager**: `internal/state` (the `Create` contract, the kube and NATS backends, the conformance suite), `internal/namewords`, `internal/chat` (op ids, router, coordinate), `internal/signalroute`, `internal/runtimepod` (`subPath`, `CONVO_ID`, the pod label), `internal/controller`, `internal/httpapi`, `api/v1alpha1` (the field rename, CRDs and deepcopy regenerated). About 130 uses of `conv.Name` are reviewed, and the 34 non-test `Create` callers adapt to the new return.
- **Console and channels**: `platform/console` and `channels/telegram` read `conversationId` today and move to `correlationId`, and take the `ID` from the wire where they key on a conversation.
- **Runtimes**: read `CONVO_ID` as an opaque string. No runtime code is expected to change. Verified by running each suite.
- **Chart**: manager and console image tags and the chart version, since a CRD field and a wire field are renamed.
- **Reference docs**: `docs/concepts.md`, `docs/contracts.md`, `docs/contracts/channel-adapter.md`, `docs/contracts/signal-adapter.md`, `docs/contracts/runtime.md`, `docs/crds/conversation.md`, `docs/CHANGELOG.md`, `docs/gotchas.md`.
- **Adopter site**: `docs/guides/signal-adapter.md`, `docs/guides/channel-adapter.md`, `docs/installation.md` for the delete-before-upgrade step, and any page showing `conversationId`.
- **Rules**: `.claude/rules/terminology.md` and `.claude/rules/invariants.md`, plus a `retired-vocabulary` entry scoped to `conversationId` as the signal correlation key, since the same name now carries the identity on the wire.
