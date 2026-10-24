# Adapter and runtime spec compliance

## Why

An audit of both channel adapters (`channels/telegram`, `platform/console`)
and all three agent runtimes against the published specs found roughly
thirty-five concrete non-compliances, three spec-internal contradictions,
and contract pages whose claims are no longer true.

The worst are live defects:

- Every runtime reports `continuity: continued` on failed resumes, which
  records a false recovery on the storage breaker.
- The copilot runtime never extracts `<brief>`, so `status.brief` never
  updates and the block renders in chat as a named section.
- The ollama runtime drops auth headers on HTTP MCP servers, so any
  authenticated server (including aops) fails.
- The telegram adapter's in-process retry budget is per call, not per
  operation, so it can exceed the reclaim window and double-post.
- The console never displays a send that carries no thread id, while
  completing the operation as success.

## What Changes

- Fix each agent runtime where it violates `runtime-contract`,
  `conversation-context-continuity`, `builtin-toolset-catalog`,
  `structured-agent-output` or its own vendor spec — above all the false
  `continued` continuity claims (all three), the missing `<brief>`
  extraction (copilot), the dropped MCP auth headers (ollama) and the
  silently tolerated clone failures (all three).
- Fix both channel adapters where they violate `channel-adapter-contract`,
  `channel-adapter-lifecycle`, `adapter-rendered-messages` or their own
  specs — above all the telegram retry/pacing budget being per call instead
  of per operation, and the console's invisible thread-less sends and
  silent no-op delete/close completions.
- Resolve the spec-internal conflicts so there is one rule to comply with:
  the telegram close-topic no-retry rule vs the contract's backpressure
  rule, the stale "manager bounds above-the-fold content" claim, and the
  console's dependence on an unpublished operation-id shape.
- Make the runtime contract page's shape claims mechanically true: a
  numbered conformance checklist, work-unit/report schema tables generated
  from the Go structs, and spec-version claims that reflect reality.
- Extend the adapter conformance suite to pin the fixed behaviours it can
  reach (thread-less sends, delete-conversation, retry budget vs reclaim).
  Behaviours it cannot reach are pinned by unit tests in each component.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `telegram-channel-adapter`: scope the close-topic no-retry rule so it no
  longer conflicts with the contract's backpressure rule, drop the stale
  "bounded by the manager" above-the-fold claim, and name the chart gate
  the chart actually renders.
- `channel-adapter-contract`: publish the input operation-id shape the
  console already depends on (or forbid that dependence — the delta decides
  and the console follows).
- `runtime-contract`: align the contract-page shape requirements with the
  deliberately published front matter, and state the generated-table and
  numbered-checklist obligations the page will now meet.
- `contract-conformance-suite`: add the suite assertions this change
  introduces (thread-less send visibility, delete-conversation, retry
  budget inside the reclaim window).

## Impact

Code:

- `runtimes/claude/`, `runtimes/ollama/`, `runtimes/copilot/` — continuity
  reporting, brief extraction, MCP headers, clone/exit-code handling, tool
  reporting.
- `channels/telegram/` — retry/pacing budgets, SendDocument relay handling,
  choices, chunking, dedupe, escaping.
- `platform/console/` (adapter and UI) — thread-less sends, no-thread
  delete/close errors, choices on earlier messages, thread-id stability,
  vocabulary header.
- `chart/charts/telegram/` — configSchema `additionalProperties`.
- `platform/manager/test/conformance/` — new assertions.
- `.github/scripts/docs-generate.py` — a work-unit/report table generator.

Reference docs made untrue and updated here:

- `docs/contracts/runtime.md` — numbered checklist, generated schema
  tables, ADA/API wording the deltas change.
- `docs/contracts/channel-adapter.md` — ADA-11.1's `outputFormat` citation,
  the published operation-id shapes, retry-budget wording.
- `docs/runtimes/ollama.md`, `docs/runtimes/copilot.md` and the claude
  runtime page — spec-version claims.
- `docs/CHANGELOG.md` — behaviour fixes adopters can observe.

Adopter site: no landing-page, getting-started, installation or guide
content describes the defective behaviours, so no adopter page changes —
checked, not skipped.
