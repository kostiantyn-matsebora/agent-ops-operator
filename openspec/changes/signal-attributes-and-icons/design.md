## Context

See proposal.md for why. What the tree holds today, and what constrains the shape:

- A normalized signal is `{fingerprint, labels, title?, payload?, kind?, reader?}`. `kind` is already a field with a closed set. `severity` is a label key two adapters fill from their own scale.
- The agent's prompt gets the payload alone (`SIGNAL_JSON`). Labels never reach it, so k8s-events and alertmanager copy severity into the payload.
- A conversation snapshots its Pipeline's `channelRefs` as `[]ObjectRef` at creation. `deliverRunReplies` sends every run to every bound thread, and threads exist for every snapshotted channel.
- `chat.PipelinesForSource` returns every Ready Pipeline listing a source, and ingest opens one conversation per Pipeline.
- Only Pipeline carries `spec.icon`, four forms, resolved by each surface. The console holds nine built-in `aops:` icons, Telegram holds their emoji.
- The manager reads chart configuration through environment variables and a JSON blob (`RUNTIME_COMMAND_JSON`), never through the API.
- The manager parses no markdown and no block grammar. Whatever the agent concludes stays in its text.

## Goals / Non-Goals

**Goals:**

- One severity vocabulary enforced at the one door every signal passes.
- Matchers that are wiring on the Pipeline, evaluated once at creation, changing no delivery code.
- One icon resolution used by every surface, so a kind, a severity and a type are drawn the same way a Pipeline is today.
- Nothing breaking. A manifest and an adapter written before this change behave as they did.

**Non-Goals:**

- An agent stating a severity on its answer. The manager parses no output, and escalation is `coordinated-agents`' verb.
- Re-binding a running conversation when a later signal is rated differently.
- Icons the adopter overrides for severity values, or per-kind defaults from values. The built-in set is the default and the field is the override.
- A new CRD for type values.

## Decisions

### Severity is a field, type is a label

- **A field, because the set is closed and validated.** A label is open by construction, and refusing one label's value while accepting every other key would make the label map two things. `kind` is the precedent: a closed set beside the open map.
- **A label, because the set is the adopter's.** `type` values are not ours to enumerate, and a label is what reaches routing from a Prometheus rule with no adapter change. Reserving the key is what k8s-events already does for `severity` and `workload`.
- **Rejected:** severity as a label with validation on that key. It would be the only validated label, invisible in the shape, and every adapter would still write the label AND read the rule.
- **Rejected:** a wider vocabulary (`fatal`, `debug`, `unknown`). Four values cover what a person pages on, `info` absorbs the floor, and absent already means unknown.

### Matchers reuse `metav1.LabelSelectorRequirement`

- The type exists, every Kubernetes reader knows it, and CEL restricts the operator to `In` and `NotIn` in the CRD. Nothing is invented and nothing is documented twice.
- **Evaluated over one projected map.** The signal's labels plus `severity` and `kind` as keys, field winning over label. Alertmanager's precedent: everything a route reads is a label. A person writing `key: severity` does not need to know it is a field.
- **Rejected:** `matchLabels` beside `matchExpressions`. It is sugar for `In` with one value, and two ways to spell one thing is the first thing the next reader asks about.
- **Rejected:** a separate `severity` shorthand on the ref. Same objection.

### The Pipeline ref type gains the matcher, the Conversation snapshot does not

- `signalSourceRefs[]` and `channelRefs[]` become a new element type, `MatchedRef {name, match?}`. The YAML `- name: x` is unchanged, so every existing manifest parses.
- `Conversation.spec.channelRefs` stays `[]ObjectRef`. The matcher is consumed at creation, and a snapshot that carried it would invite re-evaluation later, which the terminology rule forbids.
- Ingest filters twice, in `routeSignals`: the claiming Pipelines by their source matcher, then each surviving Pipeline's channels by their binding matcher, before the snapshot is written. `chat.PipelinesForSource` is untouched, since `Wired` and `/pipelines` count claims and not matches.
- The chat-command path (`/<pipeline> <task>`) evaluates channel matchers against a chat signal's map, and folds the originating surface in afterwards as it does today.

### Severity rides on provenance and on each input

- `SignalProvenance.severity` is written once with the labels. `ConversationInputSpec.severity` is written per input beside the labels it already carries.
- The agent's block reads from the input, so a recurrence rated `error` on a `warning` conversation tells the agent `error`.

### The prompt gets a metadata block, not a second payload

- Both templates gain a fenced block above the payload: kind, severity, then the labels as `key: value` lines. Rendered from three new variables (`SIGNAL_KIND`, `SIGNAL_SEVERITY`, `SIGNAL_LABELS`), which a profile's own prompt file receives too.
- **Rejected:** merging the labels into the payload JSON. The payload is the adapter's document, and the manager rewriting it would be the one place it composes an adapter's content.

### Icons: one field type, one ladder, resolved by the manager

- `IconRef` becomes a named string type in `common_types.go` carrying the four-form doc once, and each of the nine kinds embeds `Icon IconRef` in its spec. Pipeline's existing comment moves there.
- The manager resolves the ladder wherever it already publishes an icon: the vocabulary entry, the `signal` message. Resolution is `instance.Spec.Icon`, else `aops:<kind>`. Surfaces then do exactly what they do today with a Pipeline's string.
- **Rejected:** each surface applying the fallback. Two surfaces, two ladders, and the console reads kinds the message does not carry.
- The built-in set gains nine kind entries and four severity entries in `builtin.ts` and in Telegram's emoji table. The two tables are the same list, and a test on each side pins the names against one shared list in the manager (`chat.BuiltinIconNames`).

### Type icons come from values, through the manager's environment

- `global.agentops.signalTypes[]{name, icon}` renders to `SIGNAL_TYPES_JSON` on the manager Deployment, read once at start like `RUNTIME_COMMAND_JSON`. The manager stamps `icons.type` on the card from it.
- **Rejected:** a `SignalType` CRD. A new kind for a lookup table with no reconciler, no status and no wiring.
- **Rejected:** the console reading values itself. The console reads CRs, and a value is not one.

### The chart declares, a render test enforces

- Every agentops object the chart renders sets `spec.icon: aops:<something>`, bundles included. A test in `charttemplate_test.go` renders with every bundle on and fails naming any agentops object whose icon is missing or not `aops:`.
- **Why `aops:` and not any form:** the chart's objects must draw in an air-gapped install, and a URL or `mdi:` reaches for a network.

### Adapter mapping is fixed per adapter, configurable only where the sender's scale is open

- k8s-events and ha map by a constant table. Their upstream scales are closed.
- alertmanager maps through `config.severityMap` because a Prometheus rule's `severity` is whatever the author typed. Identity for the three-value convention when absent. An unmapped value is reported on the source's Ready message and the signal goes unrated, since a refused alert is worse than an unrated one.

## Risks / Trade-offs

- [A source with every claim matched away drops signals silently] → the drop reason names matching, distinct from unwired, and the response carries it. The console's queue view lists dropped-by-match beside dropped-unwired.
- [Two Pipelines with overlapping matchers double every conversation] → stated on the Pipeline page as the shareable-source rule, not refused. It is the existing fan-out behaviour with a filter in front.
- [A conversation with no matched channel is invisible to people] → it still opens and runs, its `DeliveryPending` condition says no thread is bound, and the console's own channel is the usual catch-all binding with no matcher.
- [An adapter outside this repository still sends `Warning`] → the 400 names the vocabulary and the adapter's log shows it. The contract page carries the mapping tables as the worked example.
- [Two built-in icon tables drift] → both are pinned to the manager's shared name list by a test on each side.
- [The Go element type of two Pipeline lists changes] → every in-repo reader is found by the compiler. The console reads JSON and `name` is still `name`.
- [The screenshots and demo recording go stale] → both are re-run in the documentation section, as the tasks rule requires.

## Migration Plan

1. Apply the new CRDs before the manager upgrade, as every CRD change here requires (`kubectl apply -f chart/crds/`). A manager started against old CRDs sees every new field pruned and behaves as before.
2. Upgrade the chart. Every shipped object gains an icon. No value is renamed and no default changes.
3. Upgrade adapters in any order. An old adapter sends no severity and its signals are unrated. A new adapter against an old manager has its `severity` field pruned by the JSON decoder, since unknown fields are ignored, and is otherwise unchanged.
4. Rollback is the reverse. A matcher on a Pipeline is ignored by an old manager, which is the pre-change behaviour of binding everything.

## Open Questions

- Which emoji stand for the four severities and the nine kinds on Telegram. Settled at implementation against what renders legibly, and pinned by the shared name list either way.
