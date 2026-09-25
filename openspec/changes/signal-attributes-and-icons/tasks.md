Every build and test below runs against THIS working copy's tree — the worktree at `../agent-ops-worktrees/signal-attributes-and-icons/` on a workstation (`docker exec -i -w "$PWD" agentops-go …` from inside it), or the session's clone in a remote session — never the default branch's checkout.

## 1. API types and CRDs

- [ ] 1.1 Add `IconRef` to `api/v1alpha1/common_types.go` carrying Pipeline's four-form doc once, and `Icon IconRef` to the specs of SignalSource, Channel, AgentProfile, AgentRuntime, SignalAdapter, ChannelAdapter, MCPToolset and MCPConfig, moving Pipeline onto the shared type. Verify `go build ./...` in `platform/manager` and that Conversation and ConversationInput gained no icon field.
- [ ] 1.2 Add `MatchedRef {Name, Match []metav1.LabelSelectorRequirement}` and switch `PipelineSpec.SignalSourceRefs` and `.ChannelRefs` to it, with a CEL rule restricting `operator` to `In`/`NotIn`. Verify the compiler finds every reader and an envtest applying `operator: Exists` is refused naming the two operators.
- [ ] 1.3 Add the `Severity` enum type (`critical`, `error`, `warning`, `info`) to `common_types.go`, `Severity` to `SignalProvenance` and to `ConversationInputSpec`. Verify the CRD schema carries the enum on both.
- [ ] 1.4 Regenerate deepcopy and CRDs with controller-gen into `chart/crds/`. Verify `git diff --stat chart/crds` touches nine CRDs and `go vet ./...` is clean.

## 2. Ingest: vocabulary, matchers, snapshot

- [ ] 2.1 Add `Severity` to `NormalizedSignal` in `internal/httpapi/signals.go` and validate every signal's value against the vocabulary before any routing, answering 400 naming the four values and creating nothing. Verify a handler test posts `Warning` and gets 400 with the vocabulary in the body.
- [ ] 2.2 Add `internal/ingest/match.go`: build the projected map (labels, then `severity` and `kind` overriding) and evaluate a `[]LabelSelectorRequirement` with Kubernetes `In`/`NotIn` semantics, including the empty-`values` cases. Verify table tests cover field-over-label, unrated with `NotIn`, empty `In`, and several expressions all holding.
- [ ] 2.3 In `routeSignals`, filter the claiming Pipelines by their source matcher after cooldown and grouping, and report a signal no matcher takes as dropped with a `not-matched` reason distinct from `not-wired`. Verify an envtest with two disjoint Pipelines opens one conversation, and one with no matching Pipeline reports the new reason with queued 0.
- [ ] 2.4 Snapshot `Conversation.spec.channelRefs` as the names of the Pipeline's channel bindings whose matcher holds, on both the signal path and the `/<pipeline>` chat-command path (surface folded in after). Verify an envtest binds `console` only for a `warning` signal under a `pager` binding matched to `critical`, and a chat command still binds the surface it was typed on.
- [ ] 2.5 Write `severity` on the provenance at creation and on every ConversationInput. Verify an envtest where a `warning` conversation receives an `error` recurrence shows provenance `warning` and the new input `error`.

## 3. The agent's prompt

- [ ] 3.1 Add `SIGNAL_KIND`, `SIGNAL_SEVERITY` and `SIGNAL_LABELS` to the alert and task work-unit variables in `internal/dispatch/dispatch.go`, read from the input's ConversationInput, and a fenced metadata block above the payload in `investigate.md` and `task.md`. Verify the dispatch fixture tests show the block before the payload and a profile with its own prompt file receives the three variables.

## 4. Adapters map their scale

- [ ] 4.1 `signals/k8s-events`: send `severity` mapped from the Event type, drop the `severity` key from the payload, add `type` to `reservedLabels`. Verify `go test ./...` there, with a normalize test for `Warning`→`warning`, `Normal`→`info` and a pod label `type` that does not reach the signal.
- [ ] 4.2 `signals/ha`: send `severity` mapped by the level table for log records and shaped surface records. Verify a test per row of the table and one for a config-entry failure shaped as `ERROR`.
- [ ] 4.3 `signals/alertmanager`: parse `config.severityMap`, map the alert's `severity` label with the identity default, send the field, and report an unmapped value on the source's Ready message once per value while keeping Ready true. Verify tests for the default, a house map, and the unmapped report.
- [ ] 4.4 Extend the conformance suite's signal-adapter lane so every shipped adapter's emitted signal carries a severity inside the vocabulary or none. Verify `go test -tags conformance -count=1 ./test/conformance/` in `platform/manager` passes for all four adapters.

## 5. The manager publishes rating, type and icons

- [ ] 5.1 Add `chat.BuiltinIconNames` (nine kinds, four severities, plus today's nine) and `chat.ResolveIcon(kind, instanceIcon)` implementing the ladder, and use it for the vocabulary entry's icon. Verify a unit test resolves a bare Pipeline to `aops:pipeline` and a declared one to its own.
- [ ] 5.2 Add `Severity`, `Type` and `Icons` to the `signal` message in `internal/chat/message.go` and fill them in `SignalMessage`'s caller from the provenance, the input, the resolved source and pipeline icons, the severity icon and the type table. Verify a unit test on a rated typed signal and one on an unrated task signal, whose message carries only the pipeline and source icons.
- [ ] 5.3 Read `SIGNAL_TYPES_JSON` in `cmd/manager/main.go` into the type table, tolerant of absence. Verify a unit test parses the chart's shape and an empty env yields no type icons.
- [ ] 5.4 Add the `not-matched` drop to the manager's introspection and the console's queue view beside the unwired drop. Verify the endpoint test lists both counters.

## 6. Chart

- [ ] 6.1 Set `spec.icon: aops:<name>` on every agentops object the parent chart and every bundle render (pipelines, sources, channels, profiles, runtimes, adapters, toolsets, MCP configs). Verify `helm template` with every bundle on shows an icon on each.
- [ ] 6.2 Add `global.agentops.signalTypes` (default empty, commented with the shape) and render it as `SIGNAL_TYPES_JSON` on the manager Deployment. Verify a render test with two types shows the JSON on the container and the default renders no env.
- [ ] 6.3 Add `TestEveryShippedObjectDeclaresABuiltinIcon` to `charttemplate_test.go`, rendering with every bundle on and failing on any agentops object whose icon is missing or not `aops:`. Verify it passes, and that removing one icon from a template makes it fail naming the object.

## 7. Console

- [ ] 7.1 Add the nine kind icons and four severity icons to `ui/src/icons/builtin.ts`, and a test pinning the key set to `chat.BuiltinIconNames` through a fixture the manager writes. Verify `npm test` in `platform/console/ui`.
- [ ] 7.2 Draw the resolved icon on topology nodes and inventory rows, mark `feeds`/`posts` edges whose ref carries a matcher and show the expressions on open. Verify a component test with a matched binding and a Playwright screenshot of the topology read against `visual-check.md`.
- [ ] 7.3 Render `severity`, `type` and `icons` on the transcript's signal card with theme tokens per severity, unchanged for a message without them. Verify a transcript test for a rated typed card and an unrated one, and a screenshot of a conversation against the dev server.

## 8. Telegram

- [ ] 8.1 Add the kind and severity emoji to `channels/telegram/vocabulary.go`'s table with a test pinning its keys to the shared name list, lead a rated card with the severity emoji and value in `render.go`, and draw type, source and pipeline emoji where the reference is one. Verify `go test ./...` in `channels/telegram` with a rated card, an `mdi:` type omitted, and an untyped card rendering as before.

## 9. Unit tests

- [ ] 9.1 `platform/manager`: `go test ./...` with `KUBEBUILDER_ASSETS` set, covering 1.x through 5.x and the chart render tests of 6.x. Verify green from this working copy.
- [ ] 9.2 `signals/k8s-events`, `signals/ha`, `signals/alertmanager`, `channels/telegram`: `go build ./... && go vet ./... && go test ./...` in each. Verify green.
- [ ] 9.3 `platform/console/ui`: `npm test` and `npm run test:coverage`. Verify green.
- [ ] 9.4 `platform/manager`: `go test -tags conformance -count=1 ./test/conformance/`. Verify green.
- [ ] 9.5 `python3 .github/scripts/publication-guard.py` and `python3 .github/scripts/retired-vocabulary-guard.py`. Verify both pass with no new term needed.

## 10. E2E tests

- [ ] 10.1 Not applicable: nothing here is decided by a cluster. Vocabulary validation, matching and the channel snapshot are ingest logic under envtest, mapping is adapter code under `go test` and the conformance tier, icons are rendering, and the chart obligation is a render test. No kubelet, authorizer, informer or pod lifecycle changed.

## 11. Documentation

### 11.1 Reference docs

- [ ] 11.1.1 `docs/concepts.md`: severity as a field with its vocabulary, `type` in the shared label table, matchers on both Pipeline ref lists with the one-map rule, and the icon ladder on every kind. Verify each is a section a reader lands on from the nav.
- [ ] 11.1.2 `docs/contracts.md`: `severity` in the inbound shape with the refusal, and `severity`, `type`, `icons` on the `signal` message. Verify the shapes match the Go types.
- [ ] 11.1.3 `docs/integrations/kubernetes.md`, `home-assistant.md`, `prometheus.md`: each adapter's mapping table, and `config.severityMap` with the unmapped report on prometheus. Verify each page carries its table.
- [ ] 11.1.4 `docs/configuration.md`: `global.agentops.signalTypes` with its YAML. Verify it names the default.
- [ ] 11.1.5 `docs/console.md`: what the card, the topology and the inventory draw. Verify it names the ladder once and points at concepts for it.
- [ ] 11.1.6 `docs/CHANGELOG.md`: the new fields, the CRD apply step, no breaking change. Verify it is the newest entry.
- [ ] 11.1.7 `docs/guides/pipeline.md` and `docs/guides/signal-adapter.md`: a matcher in the worked Pipeline and severity in the worked emission, through their generated markers. Verify the prose explains the matcher line.
- [ ] 11.1.8 Run `python3 .github/scripts/docs-generate.py` and then `--check`. Verify every generated block and `docs/cr-reference.md` are regenerated and the check passes.
- [ ] 11.1.9 `.claude/rules/wiring.md`: matchers are wiring on the Pipeline and evaluated at creation only. Verify `python3 .claude/scripts/rules_compliance.py` on the file is clean.

### 11.2 Adopter site

- [ ] 11.2.1 `docs/index.md`: a `match` line in the "What you write" Pipeline, and the console strip's icons. Verify the page builds.
- [ ] 11.2.2 `docs/introduction.md`: one sentence that routing by severity and type is part of the model. Verify it links to concepts.
- [ ] 11.2.3 `docs/getting-started.md`: where the first Pipeline's icon comes from and that it is optional. Verify the worked Pipeline still applies unchanged.
- [ ] 11.2.4 `docs/console-guide.md`: severity marks and icons in the views. Verify the screenshots it embeds show them.
- [ ] 11.2.5 `README.md`: a `match` line in the annotated Pipeline. Verify `wc -l README.md` stays within 215.
- [ ] 11.2.6 Re-run BOTH `npm run screenshots` and `npm run demo` in `platform/console/ui`. Verify the site's screenshots and the landing recording show icons and a severity mark.
