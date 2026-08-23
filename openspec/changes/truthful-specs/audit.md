# Audit notes — verdicts per requirement

One section per capability, in ledger order. Each requirement carries **keep**,
**correct** or **delete** (`design.md` D2), and a correction says what the code
does instead.

**The code is authoritative.** A verdict cites the API type, the reconciler, the
contract handler or the chart template it was established from — never another
spec (`tasks.md` §2.1).

## Pre-pass (§1)

### 1.1 The retired-vocabulary list

Written out in full in `.github/spec-vocabulary-denylist.json`, which is also
the CI guard's data (§6.1). Public names by construction — `design.md` D4.

### 1.2 Ranking

Hits over `openspec/specs/`, highest suspicion first:

| Term | Hits | Where the hits are |
|---|---|---|
| `spec.type` | 6 | `channel-type-model` ×3, `signal-source-model` ×2, `adapter-config-schema` ×1 |
| `MAX_RUNTIMES` | 8 | `conversation-capacity` ×2 (records the deprecation), `console-application` ×3, `manager-introspection` ×2, `signal-self-exclusion` ×1 |
| `/agents` | 6 | `chat-signal-origination` ×2, `conversation-exit-command` ×2, `telegram-channel-adapter` ×1, `agent-definition-tools` ×1 (a path, not the command) |
| `spec.delivery` | 1 | `channel-type-model` |
| `defaultProfileRef` | 1 | `channel-type-model` |
| oldest-claimant tiebreak | 12 | `pipeline-model` ×2 record the removal; `channel-type-model` ×1 asserts it; the other 9 are ordinary uses of the word |
| `docs/vm-bundle.md` | 1 | `documentation-structure` |
| moved module paths | 3 | `api/v1alpha1`, `config/samples/`, `internal/` — all now under `platform/manager/` |
| `agentops-channel-<name>` | 1 | `channel-adapter-lifecycle` — records that it does not exist |
| `POST /task` | 1 | `console-origination` — records that it is not used |
| `sessionId` | 1 | `conversation-context-continuity` — records the rename |
| `vmalertmanager` | 7 | all name a VictoriaMetrics API OBJECT, which `structure.md` keeps |

**The ranking decides nothing.** Same phrase, opposite verdicts: `pipeline-model`
records the tiebreak's removal (keep) and `channel-type-model` asserts it
(correct) — `design.md` D3.

### 1.3 Placeholder purposes

23 capabilities open with `Purpose: TBD - created by archiving change <name>`.
Marked `*` in the ledger, and the count matches the proposal's scan.

### 1.4 Three capabilities were added after the proposal was written

`landing-presentation`, `repository-layout` and `structured-agent-output` exist
in `openspec/specs/` and were not in the ledger. Added as 5.60–5.62: an
unaudited capability is exactly the state this change ends.

## §3 / §5.1 `channel-type-model` — 3 requirements

| Requirement | Verdict | Established from |
|---|---|---|
| Channel CRD splits shared metadata from type-specific config | **correct** | `api/v1alpha1/channel_types.go` carries `Adapter`, `CredentialsSecretRef`, `Config` and nothing else |
| Conversation thread ids are strings | **correct** | `ConversationStatus.Threads []ThreadBinding{Channel,ThreadID}` — there is no `status.threadId` |
| Delivery is the operator's, and prompts stay transport-blind | **keep** | `internal/dispatch/templates/{task,reply}.md` carry `{{DELIVERY_INSTRUCTIONS}}`; the substituted section is pinned by `dispatch_test.go` on "printed answer IS the deliverable" and "Do not attempt to send chat messages yourself" |

**Corrections made:**

- `spec.type`, `spec.delivery` moved from ASSERTED to RECORDED-AS-REMOVED, beside
  `defaultProfileRef` and `spec.telegram` which were already recorded that way.
- The `defaultProfileRef` sentence said the default "comes from its oldest Ready
  `Pipeline`" — the tiebreak `pipeline-model` records as REMOVED. It now says
  what replaced it: nothing does. A bare chat message resolves through the
  Pipelines claiming the chat `SignalSource`
  (`internal/httpapi/signals.go:routeChatSignals`), and several claimants are
  answered with the choices rather than tie-broken.
- Two scenarios were the type-worded originals of scenarios rewritten for
  `spec.adapter` and had survived beside them: "Arbitrary config accepted for any
  type" folded into its adapter-worded twin (it carried the only statement of the
  no-schema case), "Type is required and immutable" deleted as a duplicate of
  "Adapter reference is required and immutable".
- The migration example produced `type: telegram`, a field the CRD does not have.

## §3 / §5.2 `pipeline-model` — 7 requirements

*(the ledger said 6; the file carries 7)*

| Requirement | Verdict | Established from |
|---|---|---|
| Pipeline CRD declares the wiring | **correct** (one clause) | `api/v1alpha1/pipeline_types.go` — every field named exists |
| Pipeline-only resolution | **keep** | `internal/chat/pipelines.go:PipelinesForSource`, `internal/controller/pipeline_controller.go` (no conflict guard, no creation-order fallback) |
| Sources are shareable and signals fan out | **keep** | `internal/httpapi/signals.go:routeSignals` — cooldown and grouping evaluated ONCE above the per-server loop, exactly as written |
| A conversation records the pipeline that originated it | **keep** | `Conversation.spec.pipelineRef`, `chat.PipelineForConversation` |
| Chart-managed wiring is declared once, at the top | **keep** | `chart/templates/pipelines.yaml` fails the render on a profile-less entry; `k8s-bundle`/`prometheus-bundle` gate on their own default-off wiring flag with values-supplied foreign names |
| Pipelines are named for their purpose, not their transport | **keep** | naming rule, no code surface to contradict |
| Runtime selection is wiring, never identity | **keep** | `internal/runtimepod/snapshot.go`, `internal/httpapi/server.go` resolve conversation → pipeline → profile (deprecated) → `default` |

**Corrections made:**

- "conversations originated from any referenced channel SHALL be bound to all
  referenced channels" asserted channel origination, which no longer exists. The
  case it was describing is a `/<pipeline>` command, and `router.go:boundChannels`
  folds the ORIGINATING channel in on top of the addressed Pipeline's — which the
  old wording could not express.
- The matching scenario said "a user starts a conversation on a channel".
- "both appear in the surface's listing of available agents" → the `/pipelines`
  listing. `/agents` still answers and is never printed (`terminology.md`).
- "a user asks for the Pipeline listing" → names the command.

**DEFECT FOUND — §2.3. Not fixed here, and the requirement is NOT weakened.**

The requirement says `spec.runtimeRef` IS validated by `Ready`, and a scenario
turns on it. `pipeline_controller.go` validates `signalSourceRefs`, `channelRefs`,
`profileRef`, `toolsets.refs` and `mcpConfigs.refs` — and not `runtimeRef`. A
Pipeline naming an `AgentRuntime` that does not exist reports `Ready=True`, and
the failure surfaces only when a pod is built.

The requirement is true and desirable: the manager already reads `AgentRuntime`,
so the reason `serviceAccountName` is exempt (no RBAC for the read) does not
apply. Raised as its own change — see §6.4.

## §3 / §5.3 `adapter-config-schema` — 3 requirements

| Requirement | Verdict | Established from |
|---|---|---|
| Adapter CRs may declare a config spec for their type | **keep** | `ChannelAdapterSpec.ConfigSchema` / `.CredentialKeys`, same on `SignalAdapterSpec` |
| Declared schemas are compile-checked on the adapter CR | **keep** | `internal/controller/adapterworkload.go` — `SchemaValid` / `InvalidSchema` |
| Manager validates config against a declared schema | **correct** (one phrase) | same file — `ConfigValid` / `SchemaValidated` / `SchemaViolation`, applied advisorily in both source and channel reconcilers |

**Corrections made:** "the CR named by `spec.type`" → `spec.adapter`; the worked
example named the metrics adapter by its pre-rename name.

## §3 / §5.4 `signal-source-model` — 3 requirements

| Requirement | Verdict | Established from |
|---|---|---|
| SignalSource CRD splits shared metadata from type-specific config | **correct** | `api/v1alpha1/signalsource_types.go` carries `Adapter`, `Grouping`, `CredentialsSecretRef`, `Config` |
| Unwired sources are visible and drop signals loudly | **keep** | `internal/controller/signalsource_controller.go` — `Wired` names every serving Pipeline with its count |
| Grouping policy stays manager-side for every source type | **keep** | `internal/httpapi/signals.go` + `SignalSourceStatus.Cooldown[]` for the durable record |

**Corrections made:** `spec.type` → `spec.adapter` in the requirement and in two
scenarios (one of which was the type-worded original of an adapter-worded twin,
folded); the Purpose claimed "unchanged in-process compatibility for the built-in
`alertmanagerWebhook` type", and there are no built-in signal types at all — the
manager hosts no signal transport (`adapters.md`, and no such constant exists in
`api/v1alpha1`).

## §5.5 `console-application` — 9 requirements *

*(the ledger said 8; the file carries 9)*

| Requirement | Verdict | Established from |
|---|---|---|
| One service fans in every source the browser must not touch | **keep** | `platform/console/` — `kube.go` list/watch, `activity.go`, `manager.go`; the only writes are `originate.go` and the reply path |
| Snapshots are authoritative and the stream carries cursors | **keep** | `api.go`, `cache.go`, `rehydrate.go` |
| The overview page reports the installation and what is wrong with it | **correct** | `overview.go`; the ceiling is `MAX_ACTIVE_CONVERSATIONS` (`ui/src/pages/Queues.tsx`) |
| Queue state is a first-class view that separates queued from stalled | **correct** | `queues.go`, same ceiling name |
| A metrics backend extends history and is optional | **keep** | `metrics.go` — optional URL, aggregates only, nothing persisted |
| The configuration browser renders and cross-checks every CR | **correct** | `configapi.go:findings()` |
| The transcript renders agent output as structure, not as raw text | **keep** | `ui/src/api/blocks.ts`, `transcript.go` |
| Reads are authenticated; writes require identity and can be disabled | **keep** | `auth.go` |
| The set of trusted identity headers is a published interface | **keep** | `auth.go:forwardAuthHeaders` — six headers, in the same order as `docs/console-guide.md` |

**Corrections made:** `MAX_RUNTIMES` ×3 → `MAX_ACTIVE_CONVERSATIONS`; the
cross-check listed "Pipelines whose profile has no runtime", and `findings()`
checks AGENTPROFILES whose `runtimeRef` resolves to nothing — including the case
where it names none and no `default` runtime exists, which the old wording could
not describe. Purpose written.

## §5.26 `manager-introspection` — 7 requirements *

| Requirement | Verdict | Established from |
|---|---|---|
| The manager exposes only state that exists nowhere else | **keep** | `internal/httpapi/status.go` header comment and payload |
| Manager runtime state is served by GET /status | **correct** | `status.go:181` reports `s.MaxActiveConversations` |
| Operational state is exposed as standard Prometheus metrics | **keep** | `internal/metrics/metrics.go` — `agentops_` prefix, `_total`, `_seconds` |
| Metric labels are bounded by CR count | **keep** | same file — no conversation/run/op label |
| Instrumentation has one emission point | **keep** | metrics are fed from the activity emit sites |
| Scrape configuration and alert rules ship with the chart, disabled by default | **keep** | `chart/templates/metrics.yaml` |
| Capability resolution is served, never recomputed by clients | **keep** | `status.go:240` — `GET /pipelines/{name}/resolved` |

**Corrections made:** `MAX_RUNTIMES` ×2 → `MAX_ACTIVE_CONVERSATIONS`. Purpose written.

## §5.59 `signal-self-exclusion` — 2 requirements *

| Requirement | Verdict | Established from |
|---|---|---|
| agent-ops never ingests its own machinery as a signal | **correct** | `signals/k8s-events/selfexclude.go` |
| Self-exclusion has three independent mechanisms | **keep** | same file — `ownedNamePrefixes`, `ownedAppLabels` + `ownLabelPrefix`, `ownNamespace` |

**Corrections made:** `MAX_RUNTIMES` → the conversation cap plus
`MAX_QUEUED_CONVERSATIONS`, which is also what the source comment says; one
banned "wakes an agent" (`terminology.md` — a signal STARTS a conversation).
Purpose written.

## §5.6 `telegram-channel-adapter` — 16 requirements

*(the ledger said 15)*

All sixteen verified against `channels/telegram/` — `limiter.go` (two budgets),
`telegram.go` (`retry_after`, `setMyCommands`, `reply_markup` per message),
`vocabulary.go`, `blocks.go`, `render.go`, `main.go` (`GET/PUT /offset`, config
parsing). **keep**, with four corrections of wording that named things the code
does not have:

- the Purpose and three scenarios spoke of `type=telegram` / `type: telegram`;
  a Channel names its adapter.
- the config list included "polling enablement". `main.go:52` says it plainly:
  there is no `pollingEnabled` any more, because this adapter never polls.
- routing-visible behaviour listed "default profile" among what the shared router
  implements. Nothing supplies one.
- the end-to-end scenario had a user typing `/agents`. It still answers, and this
  is the last place that should teach it — `/pipelines` is the command.

## §5.7 `chat-signal-origination` — 4 requirements

| Requirement | Verdict | Established from |
|---|---|---|
| Conversations originate only from served signal sources | **correct** | `internal/httpapi/signals.go:routeChatSignals` |
| Chat signals carry their originating surface | **keep** | same file — `LabelChatSender`, the `agentops.dev/channel` refusal |
| Commands are answered without creating a conversation | **correct** | `internal/chat/router.go:HandleCommand`, `internal/chat/vocabulary.go` |
| Chat grouping defaults preserve human behavior | **keep** | the lane split in `signals.go` |

**Corrections made:** the commands requirement said `/agents` was the listing and
that `/<profile> <task>` creates a conversation "for the named profile". Both are
wrong in the same way: `vocabulary.go` publishes `ListCommand` and holds
`RetiredListCommand` deliberately OUT of the published set, and `addressing`
parses ONE segment naming a PIPELINE — which is what carries the capabilities. A
scenario addressed `k8s-engineer`, which is the k8s-bundle's PROFILE name, not a
route. One banned "neither Pipeline is woken".

## §5.8 `prometheus-bundle` — 9 requirements

| Requirement | Verdict | Established from |
|---|---|---|
| Self-gated subchart named for the protocol | **correct** | `chart/templates/rbac.yaml` holds the `vm-bundle:` fail-guard; every bundle template gates on `prometheus-bundle.enabled` |
| Ingest component packages the adapter with its webhook Service | **keep** | `templates/alertmanager.yaml` |
| Sender self-registration is VictoriaMetrics-only | **keep** | `signals/alertmanager/register.go` writes `VMAlertmanagerConfig` — the vendor name here NAMES A VICTORIAMETRICS OBJECT and is kept per `structure.md` |
| Vanilla Alertmanager served by printed receiver configuration | **keep** | `chart/templates/NOTES.txt` |
| One metrics MCP component, keyed for the query API | **keep** | `templates/mcp.yaml` — fixed `prometheus` server key |
| Metrics MCP server workload deployable under its own identity | **keep** | `templates/mcp-server.yaml` |
| The bundle ships an alert-investigator profile | **correct** | `templates/profile.yaml` |
| The wiring component ships one claiming Pipeline, off by default | **correct** | `templates/pipelines.yaml` + `pipeline-identity.yaml` |
| Each component is individually toggleable | **keep** | `values.yaml` |

**Corrections made:** the substrate sentence said the bundle renders no runtime
ServiceAccount. `templates/pipeline-identity.yaml` renders ONE — the account its
own route executes under, holding no Kubernetes RBAC — and suppresses it when the
install names its own. That reversal is the current rule (`invariants.md`: a
bundle renders the accounts its own routes need), and the spec still carried the
rule it replaced. One banned "wake a personality-free agent".

## §5.9 `docs-site` — 26 requirements *

*(the ledger said 24)*

Verified against the built site itself — `docs/_config.yml`, `_data/nav.yml`,
`_layouts/`, `_includes/`, `assets/`, and the pages. **keep** throughout except
the four below. The generated-content and screenshot requirements are pinned by
`.github/workflows/ci.yml:234` (`docs-generate.py --check`) and by the committed
`docs/assets/img/console/`, `console-demo-{light,dark}.mp4`, their posters and
`console-demo.vtt`; the third-party marks requirement by
`docs/assets/img/logos/README.md`.

**Corrections made:**

- **The deliverables list named five pages.** Twelve carry front matter: those
  five plus the seven guides under `docs/guides/`, every one of them in
  `_data/nav.yml`. The rule is restated as the test that actually decides it —
  a page is a deliverable exactly when it carries front matter.
- **"In this change the file SHALL list only the landing page"** — a
  change-scoped sentence that outlived its change and became a false claim about
  `nav.yml`.
- **The Introduction's opening clause** said an agent's reach "comes from the
  routing that wakes it". `docs/introduction.md` already says "the route that
  started it"; the spec was the half that had not been swept.
- **A concept card was said to link to "the reference that owns its detail".**
  Every card on the page links to the GUIDE that teaches that kind, which is a
  different document.

**Purpose** written.

## §5.10 `channel-adapter-contract` — 14 requirements

Verified route by route against `internal/httpapi/server.go`'s mux and the
handlers behind it. **keep** except four corrections:

- **The op-kind enumeration named three kinds.** `internal/chat/ops.go` declares
  four — `delete-conversation` is the fourth, and it has its own requirement in
  this same file, so the enumeration contradicted its own capability.
- **`status.threadId`** in the completion scenario. Thread ids live per binding.
- **"relayed to the conversation's sibling channels".** `chat.DeliverInputs`
  decides PER DESTINATION, not per message: every bound channel that did not
  already display it, with `ChannelAdapter.spec.echoesOwnMessages` declaring
  whether the originating one did. "Siblings" is the framing the origin-kind rule
  used, and that rule is deleted.
- **"The manager SHALL expose TWO operations".** There are three:
  `POST /channel/conversations/{name}/reset-context` exists in `contextreset.go`
  and is reached by the same binding rule. Its BEHAVIOUR is specified — by
  `conversation-context-continuity`, "A conversation whose context is gone can be
  reset explicitly" — but the contract capability that enumerates what an adapter
  may call did not list it, so the two disagreed about the size of the contract.
  Added here as the third verb, with the details that belong to the contract
  rather than to continuity: both spellings of the handle cleared, the
  `ContextContinuity=False` condition, the announcement on every bound thread,
  and success on a conversation that carries no handle.

**Noted, not corrected (code comment, not a spec):** `server.go:17` still
documents `GET /channel/channels?type=` in its header comment, while the handler
at line 776 refuses that parameter with 400. The spec is right and the comment is
stale.

## §5.11 `conversation-housekeeping` — 14 requirements

**keep** throughout, verified against `internal/controller/conversation_controller.go`
(`AutoCloseEnabled` / `AutoCloseIdleAge` / `AutoDeleteEnabled` /
`AutoDeleteClosedAge`, both defaulting off, the two clocks measured from
different origins) and `platform/housekeeping/reclaim.go` (`ReclaimWorkspaces`
scan-then-list, `ReclaimSessions` reference-plus-grace, phase-blind listing,
`maxDeletions`, `dryRun`).

**Correction made:** one banned "waking the closed one" and the scenario heading
built on it. A signal LANDS on a conversation or opens a new one; nothing is
asleep.

## §5.12 `conversation-context-continuity` — 12 requirements *

**keep** throughout, verified against `ConversationStatus.RuntimeContextID` (with
`SessionID` dual-read), `internal/storagebreaker/breaker.go` fed by both edges,
`ResolveFor(...).ContinuityPossible()` at `internal/httpapi/server.go:366`, the
`ContextContinuity` condition, and `internal/httpapi/contextreset.go`.

**Purpose** written. No corrections — this capability had been kept current.

## §4 Placeholder purposes — all 23 written

Every capability whose `Purpose` was the scaffolding placeholder now carries a
written one. `grep -r 'TBD - created by archiving' openspec/specs/` returns
nothing.

**No capability was MERGED.** D5 offered that outcome and it did not arise: the
console capabilities that looked finest-grained turned out to answer genuinely
different questions — `console-application` is what it IS, `console-deployment`
how it is INSTALLED, `console-ingress` how it is EXPOSED, `console-adapter` its
half of the channel contract. Writing the purposes is what established that,
which is the test D5 asked for.

**§4.2 — the open question is settled by D7 and had no case.** No capability
turned out to have no true requirements left. The rule stands for the next
audit: a capability whose requirements are all false is DELETED, because the
archive holds why it existed and a note saying so is not a specification.

## §5.13 `console-bulk-close` — 11 requirements *

**keep**, verified against `platform/console/convapi.go`: `handleBulkClose`
refuses an empty list and anything over `conversationPageSize` (50) SERVER-side,
`closeOne` walks the whole batch returning `closed` / `skipped` / `failed` per
name, `IncludeWorking` is the opt-in, and the close itself goes out as `/close`
on the console's own thread — `handleBulkDelete` stands beside it. **Purpose**
written.

## §5.14 `console-topology` — 11 requirements *

**keep**, verified against `topology.go`, `activity.go` and `ui/src/graph/`.
**Purpose** written.

## §5.32 `console-live-runs` / §5.40 `console-adapter` / §5.47 `console-origination` — 15 requirements *

**keep**, verified against `convapi.go`, `adapter.go`, `originate.go`,
`rehydrate.go`. Thread ids are `console-<uid>`; origination posts a `kind: chat`
signal to `/signal/inbound` and names no pipeline. **Purposes** written.

**Corrections made:** two scenarios described a relay as arriving "from a sibling
channel" — the retired framing (see §5.10).

**Stale advice deleted on sight, outside `openspec/specs/`.** `chart/values.yaml`
and `platform/console/api.go` both said a SignalSource "is claimed by exactly ONE
Pipeline". Sources are shareable; `gotchas.md` names this exact sentence as
something deleted on sight, and it had come back in two places. Both now say what
happens instead.

## §5.15–§5.62 — the remaining capabilities

Read requirement by requirement and verified against the implementation each
names. **keep** throughout except the corrections listed below; every `Purpose`
that was a placeholder is now written.

| Capability | Verified against | Verdict |
|---|---|---|
| `k8s-event-suppression` | `signals/k8s-events/` rules, dwell, inhibition, mute | keep |
| `conversation-close` | `internal/chat/router.go`, the close-topics finalizer, `status.threadsArchived[]` | keep |
| `conversation-capacity` | `conversation_controller.go`, `runtimestart.go`, `cmd/manager/main.go` | **correct** |
| `k8s-bundle` | `chart/charts/k8s-bundle/templates/` | **correct** |
| `activity-telemetry` | `internal/activity/`, `internal/httpapi/contextreport.go` | keep |
| `architecture-diagrams` | `docs/diagrams/agent-ops.drawio` + `export.py` | **correct** |
| `chat-command-vocabulary` | `internal/chat/vocabulary.go` | keep |
| `console-deployment` | `chart/templates/console.yaml` incl. the identity guard | keep |
| `console-ingress` | same file — the non-root `path` guard fails the render | keep |
| `console-unread` | `platform/console/unread_test.go`, `convapi.go` | keep |
| `ha-bundle` | `chart/charts/ha-bundle/templates/` | **correct** |
| `runtime-egress-mediation` | `platform/egress-proxy/` | keep |
| `adapter-rendered-messages` | `internal/chat/ops.go`, `channels/telegram/render.go` | keep |
| `channel-adapter-lifecycle` | `internal/controller/adapterworkload.go` | keep |
| `chart-managed-secrets` | `agentops.generatedSecretGuard` in `_helpers.tpl` | keep |
| `chat-addressing-discovery` | `internal/chat/vocabulary.go`, `channels/telegram/vocabulary.go` | keep |
| `console-live-updates` | `platform/console/api.go`, `ui/src/` | keep |
| `conversation-exit-command` | `dispatch.NeedsWorker`, `internal/chat/exit_test.go` | **correct** |
| `conversation-read-state` | `ThreadBinding.ReadAt` / `.ReadTracked`, `channelread.go` | keep |
| `documentation-structure` | `README.md`, `CLAUDE.md` + `.claude/rules/`, `_data/nav.yml` | **correct** |
| `runtime-context-sync` | `platform/context-sync/` | keep |
| `alertmanager-signal-adapter` | `signals/alertmanager/` incl. `register.go` | keep |
| `component-network-isolation` | the five `networkpolicy` templates | keep |
| `ha-signal-adapter` | `signals/ha/` | **correct** |
| `k8s-mcp-tooling` | `chart/charts/k8s-bundle/templates/mcp*.yaml` | keep |
| `mcp-toolset-model` | `internal/mcpcompile/`, `runtimepod/snapshot.go` | keep |
| `state-durability` | the three-homes rule against `conversation_types.go` | keep |
| `telegram-ingest-router` | `gateways/telegram/`, `chart/charts/telegram-bundle/` | keep |
| `agent-runtime-ownership` | `chart/templates/runtime.yaml`, `runtime-rbac.yaml` | keep |
| `console-origination` | `platform/console/originate.go` | keep |
| `conversation-message-timeline` | `status.runs[].inputs[]` in `conversation_types.go` | keep |
| `conversation-opens-with-its-input` | `chat.DeliverInputs`, `InputItem.OriginSurface` | **correct** |
| `k8s-events-signal-adapter` | `signals/k8s-events/` | keep |
| `multi-channel-conversations` | `chat/delivery.go` | **correct** |
| `runtime-workspace-persistence` | `runtimepod/podspec.go` | keep |
| `signal-adapter-contract` | `internal/httpapi/signals.go` + the mux | keep |
| `agent-definition-tools` | `runtimes/claude/runtime.js` | keep |
| `cron-signal-adapter` | `signals/cron/` | **correct** |
| `signal-adapter-lifecycle` | `internal/controller/signaladapter_controller.go` | **correct** |
| `builtin-toolset-catalog` | `chart/templates/builtin-toolsets.yaml` | keep |
| `profile-is-identity` | `api/v1alpha1/agentprofile_types.go` | **correct** |
| `landing-presentation` | `docs/index.md`, `docs/assets/` | keep |
| `repository-layout` | `.github/components.sh images` | **correct** |
| `structured-agent-output` | `channels/telegram/blocks.go`, `ui/src/api/blocks.ts` | keep |

### The corrections, by what was wrong

**A rule stated in its REVERSED form.** Three bundle specs said a bundle renders
no runtime ServiceAccount. `agent-runtime-ownership` already carries the
reversal — a bundle renders the accounts its OWN routes need, because it is the
only scope that knows what they do — and `k8s-bundle`, `ha-bundle` and
`prometheus-bundle` each ship a `pipeline-identity.yaml` that does exactly that.
Four specs, one rule, and three of them describing the version it replaced.

**A SECOND live contradiction, of the same class as §3's.**
`conversation-opens-with-its-input` said "No `pipelineRef` SHALL be introduced on
the Conversation", while `pipeline-model` requires the conversation to record its
origin and `conversation_types.go:238` carries the field. Resolved toward the
code, and the losing claim now says what replaced it and why the field exists at
all (shareable sources produce same-signature conversations, so without it the
second Pipeline's signal lands on the first's conversation).

**Evidence claimed from a source the manager cannot read.**
`conversation-capacity` said a blocked runtime start's message carries "the most
recent related event". `runtimestart.go:106` is explicit that it must not:
the manager holds `create` and `patch` on events and NO read verb, and
`PodReadyToStartContainers` is the discriminator precisely so no event read is
needed. Corrected to what the code does, with the reason kept.

**A budget that moved.** `documentation-structure` capped the README at 150
lines. It is 203, the budget is 215, and the number changed because the DOCUMENT
changed — 150 bounded a README that was three documents wearing one filename.
The section list is now stated as the bound, with the number following it. Its
routing requirement also named `CLAUDE.md` as holding the routing table, which
now lives in the `.claude/rules/` topic files that `CLAUDE.md` indexes.

**Banned vocabulary, eleven occurrences.** "Wake" for what a signal does, across
eight capabilities including the one describing the DRAWN diagram — whose SVG
already says "what starts". A signal STARTS a conversation.

**The retired listing command, presented as the interface.** `/agents` in
`chat-signal-origination` (twice, including one that called `/<profile> <task>`
the addressed form), `conversation-exit-command`, `telegram-channel-adapter` and
`docs/ha-bundle.md`.

**"Sibling channels", four occurrences.** Delivery is decided PER DESTINATION,
which `multi-channel-conversations` states in its own requirement and then
contradicted two scenarios later.

**`type:` where a CR names its adapter**, in `cron-signal-adapter` and
`signal-adapter-lifecycle`.

## §6 Keeping it true

### 6.1 The guard

`.github/scripts/retired-vocabulary-guard.py` over
`.github/retired-vocabulary.json` — 18 terms, each with what to write instead and
the words that mark a sentence as RECORDING the removal rather than asserting it.
Wired into `ci.yml` as its own job beside `publication`, and the two are
deliberately opposite shapes for reasons stated in both.

**The window is a line either side of the match, bounded to 240 characters.**
Both halves were paid for while building it:

- LINE-scoped was the first version and it failed on correct text — the prose is
  hard-wrapped, so "retired" lands on the line above `/agents` about half the
  time. A guard that fails for a reason invisible from its own message is a guard
  somebody turns off.
- PARAGRAPH-scoped was the second and it went too far: one "removed" anywhere in
  a paragraph passed an assertion at its other end. That is not hypothetical —
  the paragraph listing every removed `Channel` field is exactly where a
  reintroduced `spec.type` would be written, and the test below confirmed it
  passed.

### 6.2 It fails

Two reintroductions, each applied to the tree, run, and reverted:

| Reintroduction | Result |
|---|---|
| `spec.type` added back to the `Channel` CRD-surface requirement — the hardest case, inside a long sentence that legitimately says "removed" | FAILS, naming file, line and term |
| the oldest-claimant tiebreak added as a fresh scenario in `pipeline-model` | FAILS, naming file, line and term |

Tree restored, guard clean over 85 files. **The verdict is recorded, not the
text** (`publication.md`).

### 6.3 Green

- `openspec validate --specs --strict` — 62 passed, 0 failed.
- `retired-vocabulary-guard.py` — clean.
- `publication-guard.py` — clean.
- `docs-generate.py --check` — 23 generated files up to date.

## Findings raised elsewhere

1. **`Pipeline.spec.runtimeRef` is not validated** — the §2.3 defect. Raised as
   `openspec/changes/validate-pipeline-runtime-ref`.
2. **`.claude/rules/gotchas.md` and the ha-bundle chart disagree.** The rule says
   ha-bundle's acting route "claims the log source and NO chat source, so
   reaching it is `/ha-ops <task>` and never an accident".
   `chart/charts/ha-bundle/templates/pipelines.yaml` gives BOTH routes the
   install's chat sources, and the `ha-bundle` spec argues for that explicitly.
   Two of three agree, but which is right is a DESIGN decision rather than an
   audit correction, so it is reported and not resolved here.

## Drift corrected outside `openspec/specs/`

Found while establishing what the code does, and each one a sentence that would
have taught the next reader something untrue:

| Where | Was |
|---|---|
| `chart/values.yaml`, `platform/console/api.go` | "a source is claimed by exactly ONE Pipeline" — the sentence `gotchas.md` says is deleted on sight |
| `platform/manager/internal/httpapi/server.go` | the package's endpoint list: `?type=` twice, "worker", and three routes missing |
| `platform/manager/internal/controller/signaladapter_controller.go` | `SOURCE_TYPE` env and `spec.type` |
| `api/v1alpha1/agentruntime_types.go` | "the worker image", `sessionId` in the work-contract summary, "every worker", "an idle worker" |
| `api/v1alpha1/{channel,signal}adapter_types.go` | `spec.type` as the routing key, ×4 |
| `api/v1alpha1/conversation_types.go` | "dispatched to the worker" |
| `internal/dispatch/dispatch.go` | "worker renders PromptVars" |
| `docs/contracts.md` | taught runtimes to read `resumeSessionId`, which is DEPRECATED — `runtimeContextId` is the field |
| `docs/ha-bundle.md` | `/agents` offered as the listing command |

**The API doc comments are the CRD descriptions**, so `chart/crds/` and
`docs/cr-reference.md` were regenerated — which is what makes those nine rows one
change rather than nine.

## §7 Documentation

**7.1 — the reference docs.** No behaviour changed, so the question was which
published pages the audit found UNTRUE while establishing what the code does.
Three, all fixed where they live:

- `docs/contracts.md` taught runtimes to read `resumeSessionId` when continuing.
  `dispatch.go:86` marks that field DEPRECATED and says runtimes must read
  `runtimeContextId`; `runtimes/claude/runtime.js:357` prefers the new one and
  falls back. The contract page was teaching the fallback as the field.
- `docs/ha-bundle.md` offered `/agents` as the listing command.
- `docs/cr-reference.md` carried `spec.type` ×4, `sessionId`, and "worker" ×4 —
  all of it GENERATED, so it was fixed in the API doc comments, `chart/crds/`
  regenerated with `controller-gen`, and `docs-generate.py` re-run.
  `docs-generate.py --check` reports 23 generated files up to date.

**7.2 — the adopter site: CHECKED, and nothing needed changing.** Recording that
as a result rather than skipping it. `index.md`, `introduction.md`,
`getting-started.md`, `installation.md`, `console-guide.md` and the seven guides
were read against every class the audit corrected — the removed `Channel` fields,
the withdrawn tiebreak, the retired listing command, the cap's env name, the
bundle route accounts, `pipelineRef`. The only near-hits were
`guides/pipeline.md:66`, which already says "`Channel` has no default profile",
and the `.claude/agents/` PATH in four guides, which is the agent definition file
and not the command.

That the site was clean while `openspec/specs/` was not is itself the finding:
the site gets swept when a behaviour changes, and the specs did not.

**7.3 — `CONTRIBUTING.md`** gained a section for the new guard beside the
publication one, stating why the two are opposite shapes and that recording a
removal still passes.

**7.4 — the repository's context.** `.claude/rules/retired-vocabulary.md` is a
NEW FILE rather than a section in `publication.md`: one topic per file, and these
two guards are opposite shapes whose reasoning must not be read as one rule.
`documentation.md`'s routing table gained the row, so the next writer retiring
something lands on it — and the rule states that the term is added IN THE SAME
CHANGE that removes the thing, because afterwards nobody remembers the name.

## Verification

| Check | Result |
|---|---|
| `openspec validate --specs --strict` | 62 passed, 0 failed |
| `retired-vocabulary-guard.py` | clean, 85 files |
| `publication-guard.py` | clean |
| `docs-generate.py --check` | 23 generated files up to date |
| `go build ./... && go vet ./...`, every module | clean |
| `go test` — controller, chat, httpapi, api | ok |

**The working tree carries another session's in-progress change**
(`context-volume-and-existing-storage`). Running `controller-gen` regenerated
`chart/crds/` from the CURRENT api types, so those files now carry that change's
`context:` volume field as well as this change's comment corrections. That is
what regenerating does and it is consistent with the tree, but it is worth
knowing before reading the CRD diff.
