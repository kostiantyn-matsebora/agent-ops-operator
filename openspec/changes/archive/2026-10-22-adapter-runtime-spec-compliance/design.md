# Design — adapter-runtime-spec-compliance

## Context

See proposal.md for motivation. The full audit gap list (finding ids C1–C6,
O1–O9, P1–P7, A1–A11, B1–B6, S1–S3) lives in the audit record this change was
planned from. Each finding is restated in the task that fixes it.

Facts that shape the approach:

- The manager's continuity handling is already correct: only `unavailable`
  feeds the storage breaker and holds the input, `continued` records a
  recovery (`internal/httpapi/server.go`). The defects are runtime-side
  claims, so the manager is not touched.
- `test/stubruntime` already reports `unavailable` on its storage-outage
  directive — the stub models the contract the shipped runtimes break.
- The conformance suite runs both adapters as built binaries, so new
  black-box assertions slot into the existing harness.
- The in-flight change `channel-adapter-specification` also carries a
  `channel-adapter-contract` delta, but it only ADDs page-shape
  requirements. No requirement overlaps this change's delta.

## Goals / Non-Goals

**Goals:**

- Every shipped runtime and adapter behaves as the published specs say,
  with each fixed behaviour pinned by a test.
- The specs are made internally consistent where they contradicted each
  other, so compliance is decidable.
- Contract pages claim only what is mechanically true.

**Non-Goals:**

- No runtime conformance tier. Runtimes stay pinned by their unit tests,
  and the suite spec stays adapter-only.
- No manager-side continuity or breaker changes.
- No new wire fields on the channel-ops or work contracts.
- Copilot `maxTurns` enforcement stays a documented gap — the spec carries
  no MUST for it.

## Decisions

### Contract decisions (settled by the delta specs)

| Decision | Resolution |
|---|---|
| Close-topic retry (S3, A9) | In-window backpressure is absorbed like any call. "Not retried" means a COMPLETED operation is never re-attempted. A failed close is re-derived by the manager, not terminal |
| Above-the-fold bound (S1, A7) | The "bounded by the manager" claim is deleted. The ADAPTER guarantees fold order — title plus named sections lead chunk one when they fit, folded content never precedes them |
| Chart gate naming (A8 drift) | The spec names `telegram.enabled`, the value an installer sets. `telegramAdapter.enabled` is recorded as retired |
| Input op-id shape (B4) | BOTH delivery id shapes are published as contract guarantees. Any unpublished id stays opaque and parsing it is forbidden. Rejected: a new `inputId` wire field duplicating what the stable id encodes |
| Retry budget (A2) | The budget is per OPERATION. Every transport call's wait, retry and timeout charges one total. Rejected: per-call budgets sized to call counts, which vary by chunking |
| Runtime page front matter | Allowed and declared presentation. A front-matter edit moves no version |
| Checklist and generated tables | No delta — the requirements stand, the page and `docs-generate.py` are brought into compliance |
| Startup exits (LC-2) | Missing required config before the first poll is a nonzero CRASH naming the gap. The idle TTL stays the only lifecycle exit. Honouring SIGTERM is not a lifecycle exit |
| `/work/done` rejection | A definitive `4xx` (not `408`/`429`) is never retried — logged with status and body, polling resumes. Transient failures keep bounded retry |
| Suite additions | Three channel-side assertions: thread-less send visible-or-error (B1), delete-conversation act-or-error (B3), per-operation retry time under `reclaimAfterSeconds` (A1, A2) |

### Implementation decisions

**Continuity reporting states what HAPPENED, never the intent.**

- `continued` only when the vendor actually resumed the context.
- An unreadable store is an outage (`unavailable`), never a loss. Only a
  confirmed-absent context fails as lost.
- A failed resume for any other reason reports its failure without a
  continuity claim of recovery.
- This matches the stub runtime, so no manager change follows.

**Telegram carries one budget object per operation.**

- The claim handler mints it from `reclaimAfterSeconds` and threads it
  through every Bot API call the operation makes — chunk sends, fallbacks
  and `SendDocument` included.
- Pacing moves from per-claim to per-call, charged where the call is issued.
- `SendDocument` is rewritten onto the shared relay request/response path,
  so 429, gateway rejections and the general-topic fallback behave
  identically everywhere.

**Thread-less console sends surface as the channel's general feed.**

- The adapter already stores them under the `channel:<name>` pseudo-thread.
- The transcript API starts serving that thread, and the SPA renders it as
  the channel's general surface, mirroring Telegram's general topic.
- Rejected: completing the op with an error, which would turn every manager
  notice into a failure the manager cannot act on.

**Choices degrade, Telegram-style, in both adapters.** A choice that
cannot be an interactive control renders as text (A5). The console shows
each message's choices with that message, with only the latest message's
set interactive (B2).

**Console thread ids come from the UID or not at all.** On a cache miss,
`ensure-topic` resolves the conversation's UID through the existing watch
client before answering, and errors if it cannot — redelivery retries.
The `console-name-<conversation>` fallback is deleted (B5).

**The runtime schema tables become build output.** `docs-generate.py`
gains a generator that renders the work-unit and report tables from the
manager's Go structs under a declared marker in `docs/contracts/runtime.md`,
the same marker discipline every other generated block uses. CI's existing
`--check` then catches drift.

## Risks / Trade-offs

- [Continuity reporting changes alter breaker input in production] →
  every reporting path gets a unit test, and the system pack's
  storage-outage behaviour is asserted through the real claude runtime
  before merge.
- [Per-operation budget could fail operations that used to ride out long
  429 chains] → the budget total is unchanged (strictly below reclaim).
  Only its accounting moves, and the conformance assertion pins the bound.
- [Archive-time collision with `channel-adapter-specification` on
  channel-adapter-contract] → verified disjoint today (ADDs only, no shared
  requirement). Re-check at whichever change archives second.
- [New console general-feed surface is user-visible UI] → the visual check
  (screenshot against the dev server) is part of the UI tasks, and
  `screenshots`/`demo` assets are regenerated in the documentation section.
- [Fold-order chunking changes long-message layout in live Telegram chats]
  → render tests pin the new order, and the change only moves content
  between chunks, never drops it.

## Migration Plan

No CRD, storage or wire-field changes. Runtimes and adapters roll with
their images under the existing release process. No operator action.

## Open Questions

None — the contract rulings above settle every ambiguity the audit raised.
