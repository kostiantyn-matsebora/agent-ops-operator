# Tasks — truthful specs

385 requirements across 59 capabilities. The ledger in §5 is one line per
capability rather than per requirement, because 385 checkboxes are ticked in
batches and 59 are not — see `design.md` D1.

**Every requirement gets one of three verdicts against the code: keep, correct,
delete.** The code is authoritative. The one exception is the finding that
matters most, and §2.3 is where it goes.

## 1. Pre-pass — candidates, not verdicts

- [ ] 1.1 Build the retired-vocabulary list from this repository's own context
      rules: removed CRD fields, withdrawn rules, superseded commands, renamed
      bundles and moved module paths. These are public names, so the list is
      written out in full — see `design.md` D4
- [ ] 1.2 Run it over `openspec/specs/` and rank capabilities by hit count.
      This ORDERS the work; it decides nothing. The same phrase is correct in a
      spec that records a removal and wrong in one that asserts it
- [ ] 1.3 List the capabilities whose `Purpose` is the scaffolding placeholder,
      and mark them in the ledger

## 2. Method, fixed before the first capability

- [ ] 2.1 For each requirement, establish what the code does — from the API
      types, the reconciler, the contract handlers or the chart templates, never
      from another spec. A spec verified against a spec propagates the drift it
      was meant to catch
- [ ] 2.2 Record the verdict per requirement in the working notes for that
      capability. Corrections are made in place in `openspec/specs/`; the change
      carries the durable RULE as its delta, and the corrections land as they
      are found
- [ ] 2.3 When the code does not satisfy a requirement that is true and
      desirable, STOP: record it as a defect and raise it as its own change. Do
      not weaken the requirement to match the behaviour. That converts a bug
      into a decision nobody made, and it is the failure mode a spec audit is
      most prone to
- [ ] 2.4 A true requirement is left exactly as written. The test is "is this
      true", not "is this well put" — rewriting for style inside an audit makes
      the diff unreviewable and hides the corrections in it

## 3. The contradiction, first

- [ ] 3.1 Resolve the two specs that disagree about how a bare chat message
      finds its profile. One records the tiebreak's removal and is correct; the
      other asserts the removed behaviour and carries a live scenario for it
- [ ] 3.2 Correct the `Channel` CRD surface against the API type — the spec
      asserts three fields the type does not carry
- [ ] 3.3 The losing claim says what REPLACED it. A silently dropped assertion
      leaves the next reader to rediscover why it went

## 4. Placeholder purposes

- [ ] 4.1 Write the `Purpose` for each capability the pre-pass marked, or merge
      the capability into the one that absorbed it. A merge is a success here,
      not a failure to write one — see `design.md` D5
- [ ] 4.2 Settle the open question in `design.md` on the first capability that
      turns out to have no true requirements left, and apply that answer to
      every later one

## 5. The audit ledger — one line per capability

Ordered by consequence, per `design.md` D6: the contradiction first, then
the CRD surface, then the contracts, then everything else by size. `*` marks a
placeholder `Purpose`, which is written or the capability is merged (D5).

- [ ] 5.1 `channel-type-model` — 3 requirements
- [ ] 5.2 `pipeline-model` — 6 requirements
- [ ] 5.3 `adapter-config-schema` — 3 requirements
- [ ] 5.4 `signal-source-model` — 3 requirements
- [ ] 5.5 `console-application` — 8 requirements *
- [ ] 5.6 `telegram-channel-adapter` — 15 requirements
- [ ] 5.7 `chat-signal-origination` — 4 requirements
- [ ] 5.8 `prometheus-bundle` — 9 requirements
- [ ] 5.9 `docs-site` — 24 requirements *
- [ ] 5.10 `channel-adapter-contract` — 14 requirements
- [ ] 5.11 `conversation-housekeeping` — 14 requirements
- [ ] 5.12 `conversation-context-continuity` — 12 requirements *
- [ ] 5.13 `console-bulk-close` — 11 requirements *
- [ ] 5.14 `console-topology` — 11 requirements *
- [ ] 5.15 `k8s-event-suppression` — 11 requirements *
- [ ] 5.16 `conversation-close` — 10 requirements *
- [ ] 5.17 `conversation-capacity` — 9 requirements *
- [ ] 5.18 `k8s-bundle` — 8 requirements
- [ ] 5.19 `activity-telemetry` — 7 requirements *
- [ ] 5.20 `architecture-diagrams` — 7 requirements
- [ ] 5.21 `chat-command-vocabulary` — 7 requirements
- [ ] 5.22 `console-deployment` — 7 requirements *
- [ ] 5.23 `console-ingress` — 7 requirements *
- [ ] 5.24 `console-unread` — 7 requirements
- [ ] 5.25 `ha-bundle` — 7 requirements *
- [ ] 5.26 `manager-introspection` — 7 requirements *
- [ ] 5.27 `runtime-egress-mediation` — 7 requirements
- [ ] 5.28 `adapter-rendered-messages` — 6 requirements
- [ ] 5.29 `channel-adapter-lifecycle` — 6 requirements
- [ ] 5.30 `chart-managed-secrets` — 6 requirements *
- [ ] 5.31 `chat-addressing-discovery` — 6 requirements
- [ ] 5.32 `console-live-runs` — 6 requirements *
- [ ] 5.33 `console-live-updates` — 6 requirements
- [ ] 5.34 `conversation-exit-command` — 6 requirements
- [ ] 5.35 `conversation-read-state` — 6 requirements
- [ ] 5.36 `documentation-structure` — 6 requirements *
- [ ] 5.37 `runtime-context-sync` — 6 requirements *
- [ ] 5.38 `alertmanager-signal-adapter` — 5 requirements
- [ ] 5.39 `component-network-isolation` — 5 requirements
- [ ] 5.40 `console-adapter` — 5 requirements *
- [ ] 5.41 `ha-signal-adapter` — 5 requirements *
- [ ] 5.42 `k8s-mcp-tooling` — 5 requirements
- [ ] 5.43 `mcp-toolset-model` — 5 requirements
- [ ] 5.44 `state-durability` — 5 requirements *
- [ ] 5.45 `telegram-ingest-router` — 5 requirements
- [ ] 5.46 `agent-runtime-ownership` — 4 requirements
- [ ] 5.47 `console-origination` — 4 requirements *
- [ ] 5.48 `conversation-message-timeline` — 4 requirements
- [ ] 5.49 `conversation-opens-with-its-input` — 4 requirements
- [ ] 5.50 `k8s-events-signal-adapter` — 4 requirements
- [ ] 5.51 `multi-channel-conversations` — 4 requirements
- [ ] 5.52 `runtime-workspace-persistence` — 4 requirements *
- [ ] 5.53 `signal-adapter-contract` — 4 requirements
- [ ] 5.54 `agent-definition-tools` — 3 requirements
- [ ] 5.55 `cron-signal-adapter` — 3 requirements
- [ ] 5.56 `signal-adapter-lifecycle` — 3 requirements
- [ ] 5.57 `builtin-toolset-catalog` — 2 requirements
- [ ] 5.58 `profile-is-identity` — 2 requirements
- [ ] 5.59 `signal-self-exclusion` — 2 requirements *

## 6. Keep it true

- [ ] 6.1 Add the retired-vocabulary guard to CI, over `openspec/specs/` and the
      published pages. A DENYLIST is correct here and the list is the value: it
      is the record of what this project stopped doing, in the one place that
      fails a build when someone brings it back
- [ ] 6.2 Prove it fails: a scratch pull request reintroducing one retired field
      name as a current claim
- [ ] 6.3 `openspec validate --specs --strict` passes, and every capability in
      the ledger is ticked. An unticked line is an unaudited capability, which
      is the state this change exists to end
