# Proposal: make-channel-type-architecture-extendable

## Why

Channel types are currently compiled into the operator: `ChannelSpec` hardcodes a `telegram` sub-struct, the poller/factory/token-reader are Telegram-specific, and adding any new channel (Slack, Teams, Discord, …) means forking the operator. Adopters should be able to bring their own channel implementations without touching operator code — the same way `AgentRuntime` already lets them bring their own runtime against the `/work` contract.

## What Changes

- **BREAKING** — Restructure the `Channel` CRD into two parts:
  - **channel metadata** (one schema for all types): `spec.type` (string identifier), `defaultProfileRef`, and other type-agnostic fields;
  - **channel config** (`spec.config`): an opaque, schema-less object (`x-kubernetes-preserve-unknown-fields`) holding whatever the specific channel type needs. The operator never interprets it.
  Existing `spec.telegram` CRs must be migrated (API group is provisional pre-1.0; no conversion webhook).
- **BREAKING** — Widen `ConversationStatus.threadId` from `*int64` to string so channel types with non-numeric thread ids (Slack `ts`, Teams message ids) fit; flows through dispatch `WorkUnit` and the runtime env.
- Define a **channel adapter contract**: a pull-based HTTP API on the manager (mirroring the runtime `/work` contract) through which an out-of-process adapter receives outbound operations (ensure-topic, send) via long-poll, completes them asynchronously, and pushes inbound messages into the shared router. Adapters own their transport, credentials, and polling — the manager stops reading channel secrets entirely (strengthens the existing invariant).
- **Extract Telegram from the operator** into a reference channel adapter (own binary/image, `channel-telegram/`, precedent: `runtime-claude/`): getUpdates polling, offset persistence, bot token, approver filtering all move there.
- Manager keeps a small **provider registry**: channel type → in-process provider (reserved for built-ins such as the planned web channel) with fallback to the generic remote-adapter path for unknown types.
- Helm chart: adapter-facing values (Telegram adapter deployment template as reference), updated Channel CRD, migration notes.
- Related pending change: `add-web-chat-channel` currently plans a `spec.web` sub-struct — after this lands it must be rebased to a built-in `type: web` provider with its settings under `spec.config` (tracked there, not here).

## Capabilities

### New Capabilities

- `channel-type-model`: the split Channel CRD — common metadata (`type`, `defaultProfileRef`, delivery hints), opaque per-type `config`, string thread ids, validation, migration from the telegram sub-struct shape.
- `channel-adapter-contract`: the manager↔adapter HTTP contract — outbound operation long-poll + async completion, inbound message push into the shared router, adapter authentication.
- `telegram-channel-adapter`: the reference adapter extracted from the manager — Telegram behavior preserved (polling, offsets, approvers, single-consumer invariant), packaged as its own image with chart support.

### Modified Capabilities

<!-- none in openspec/specs/ yet; the overlap with the un-archived add-web-chat-channel change is handled by rebasing that change, noted above -->

## Impact

- `api/v1alpha1/channel_types.go` rewritten (metadata + `runtime.RawExtension`-style config); `conversation_types.go` thread-id type change; regenerated deepcopy + CRDs.
- `internal/chat/`: `Telegram` impl and poller deleted from the manager; `Provider` becomes registry-resolved; router (transport-neutral inbound) becomes the single entry point for adapters and built-ins.
- `internal/httpapi/`: new `/channel/*` adapter endpoints beside `/work`; auth for adapters.
- `cmd/manager/main.go`: hardcoded `ChatFactory`/`TokenReader` Telegram branches removed; registry wiring.
- `internal/dispatch/`: `WorkUnit.ThreadID` type change; delivery instructions driven by channel metadata instead of hardcoded Telegram wording; fixtures updated deliberately.
- New `channel-telegram/` module (reference adapter) + Dockerfile; chart gains adapter deployment/values; RBAC shrinks (manager loses its secret `get` for bot tokens; adapters get their own).
- Invariants updated: "manager never reads agent secrets" tightens to "manager reads no secrets"; chat-poller leader-onlyness becomes an adapter responsibility (single-replica or leader-elected adapter).
- Migration: every existing Channel CR and any tooling that reads `status.threadId` as a number.
