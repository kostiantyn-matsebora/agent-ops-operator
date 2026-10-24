## Context

`platform/manager/test/conformance/` already builds real adapter binaries
and drives them against `FakeManager`, an in-package `httptest.Server`.
`channel_test.go` additionally starts `test/fakebotapi` as a separate built
binary, because `gateways/telegram` needs a real network endpoint to dial
and because the system pack deploys that same fake as a Service.

`runtimes/ollama` talks to its vendor over plain HTTP at `OLLAMA_URL`
(`runtimes/ollama/main.go`, `ollama.go`). No other test tier — the system pack
included — exercises `runtimes/ollama` today, so nothing else needs its fake
Ollama double to exist as a separate artifact.

See `proposal.md` for why this gap matters and why `runtimes/claude` and
`runtimes/copilot` are out of scope here.

## Goals / Non-Goals

**Goals:**
- Catch a regression in `runtimes/ollama` against `docs/contracts/runtime.md`
  before merge, with the same cost profile the adapter conformance set has.
- Reuse `FakeManager` rather than building a second fake manager.

**Non-Goals:**
- Covering `runtimes/claude` or `runtimes/copilot` (see Open Questions).
- Testing anything the manager decides server-side about a runtime pod —
  admission, eviction, context checkpointing. Those stay in envtest/system.
- Judging answer quality. The fake Ollama double returns scripted, fixed
  text — it is a protocol double, not a model.

## Decisions

### The fake Ollama double is an in-package `httptest.Server`, not a new binary
**Decision:** add it to `runtime_test.go` (or a sibling `fakeollama.go` in
the same package), built and started like `FakeManager` itself.

**Why:** `test/fakebotapi` is a separate binary because `gateways/telegram`
needs a real network endpoint and the system pack deploys it as a Service.
Neither applies here — nothing else dials a fake Ollama. Promoting it to
`test/fakeollama` would add a module with no second consumer.

**Alternative considered:** mirror `test/fakebotapi` exactly, for
consistency. Rejected — the consistency that matters is "a double, scripted
deterministically, reachable only through the vendor's own endpoint shape,"
not "every double is its own binary." `FakeManager` itself is already the
in-package precedent.

### Directive vocabulary, not a full clone of `test/stubruntime`'s
**Decision:** the fake Ollama double reads the first word of the prompt it
receives and scripts its response from a small, purpose-built vocabulary —
only what the asserted requirement IDs need:

| Directive | Double's response |
|---|---|
| (default) | a short, fixed completion |
| `slow` | holds the response past one poll cycle |
| `unparsable` | a response the runtime cannot parse |

Every requirement ID the suite asserts, and which path exercises it:

| Requirement ID(s) | Exercised by |
|---|---|
| `API-1`, `LC-1` | the `slow` directive. The double holds its response past one poll cycle, proving the runtime keeps polling and a second poll while Working gets nothing |
| `API-7` | the `unparsable` directive. A response the runtime cannot parse proves the undetermined fact is omitted, never reported as zero |
| `API-2`, `API-4`, `API-5`, `API-8`, `API-9`, `RT-1`, `RT-2`, `RT-3` and its sub-items, `SEC-1`–`SEC-3`, `COMPAT-1` | the default path. One ordinary successful run, against a served unit and a repository fixture the test controls directly |
| `LC-2` | a short `RUNTIME_IDLE_TTL_M`, no directive. The double is never reached once the TTL elapses |
| `LC-3` | an `/exit` call sent mid-run, no directive. The double is still mid-response when it arrives |

`API-3` is excluded for the same reason as `API-6` and every `MGR-n` row.
It obligates whoever ANSWERS `GET /work` — the manager — to have context
already restored before replying.

`FakeManager` trivially satisfies this by construction, so asserting it
here would prove nothing about a real manager's behavior. It stays owned
by envtest/system.

**Why:** `test/stubruntime`'s vocabulary (`echo`/`fail`/`stale-context`/
`no-context`/`die`/`stall`/`storage-outage`/`calls`) exists for manager-side
mechanisms. It scripts a stale handle, a storage outage, a stall past the
idle TTL, as the manager observes them.

This suite asserts the runtime's own request/response shape instead:
allowlist composition, bounds trimming, enum handling. Cloning the full
vocabulary would script conditions nothing here asserts.

**Alternative considered:** reuse `test/stubruntime`'s exact directive
words for consistency across suites. Rejected now. Revisit if a later
requirement ID needs a condition `test/stubruntime` already scripts — the
table above only grows when an assertion needs it to.

### `GET /work` / `POST /work/done` on `FakeManager` mirror the `/channel/ops` pair exactly
**Decision:** same request-recording, same completion-recording, same
rejection-toggle shape as `handleOps`/`handleDone`, just a different payload
shape (work unit in, completion report out).

**Why:** `channel_test.go`'s assertions (`assertContractHandshake`,
`completionsFor`) are the proven pattern for "did the binary poll correctly,
did it report correctly." Reusing the shape means the runtime assertions
read the same way to a reviewer already familiar with the adapter ones.

## Risks / Trade-offs

- **A scripted double cannot prove the runtime executes real work
  correctly.** Mitigated by scope: this tier never claimed to be the
  quality oracle — `docs/testing.md` already reserves that for the real
  agent runtime lane in the system pack's full tier.
- **The directive vocabulary could grow unscoped if treated as a dumping
  ground for every future runtime assertion.** Mitigated by requiring each
  new entry to name the requirement ID it exists for, in the table above
  and in the test's own comment, the same discipline `test/stubruntime`'s
  own doc comment already models.

## Open Questions

- **`runtimes/claude` / `runtimes/copilot` timing.** `claude-runtime-sdk`
  (in progress, 0/41 tasks) will replace `runtimes/claude`'s
  `CLAUDE_BIN`/spawn seam with the Claude Agent SDK driven in-process. A
  follow-on change should scope their conformance coverage once that lands
  and the SDK's own test-double shape is known, rather than building
  against a spawn-based fake that migration discards. This does not change
  this change's specs, approach, or tasks — it changes what a later change
  covers.
