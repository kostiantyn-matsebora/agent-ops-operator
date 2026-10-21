## 1. CRD types and validation — api-architect

- [ ] 1.1 Add `AttributeFilterOperator` (`In`/`NotIn`/`Exists`/`DoesNotExist`,
      `+kubebuilder:validation:Enum` on the field) and
      `AttributeFilterRequirement{Key, Operator, Values}` to
      `platform/manager/api/v1alpha1/signalsource_types.go`. Add
      `SignalSourceSpec.AttributeFilters []AttributeFilterRequirement` and
      `SignalSourceSpec.LogFiltered *bool` (nullable, documented default
      `true`). Verify by regenerating deepcopy and CRDs from this change's
      worktree:
      `go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 object paths=./api/...`
      then
      `go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 crd paths=./api/... output:crd:artifacts:config=../../chart/crds`,
      and confirm `chart/crds/agentops.dev_signalsources.yaml` shows the
      `operator` enum and both new fields.

## 2. Ingest filtering — backend-developer

- [ ] 2.1 In `platform/manager/internal/httpapi/signals.go`'s
      `handleSignalInbound`, right after the unconditional `signal.received`
      emission and before the `routeSignals`/`routeChatSignals` lane split,
      partition `in.Signals` into `admitted`/`filteredOut` against
      `source.Spec.AttributeFilters`: convert each `AttributeFilterRequirement`
      into a `metav1.LabelSelectorRequirement`, assemble a `metav1.LabelSelector`,
      and match via `metav1.LabelSelectorAsSelector` + `Selector.Matches` on the
      signal's `Labels`. A requirement that fails to construct (bad value
      arity) fails closed — the gated signal is treated as filtered out.
      Route only `admitted`. Skip routing entirely when `admitted` is empty.
      Verify with unit tests covering the spec's four scenarios: all-match
      routes normally, one failing requirement drops before cooldown, empty
      or absent `attributeFilters` admits everything unchanged, and a
      value-arity-malformed requirement fails closed.
- [ ] 2.2 Add `activity.CodeFiltered = "filtered"` beside
      `CodeUnclaimed`/`CodeAtCapacity`/`CodeAmbiguous` in
      `platform/manager/internal/activity/event.go`. Call
      `s.emitDropped(source, filteredOut, activity.CodeFiltered, reason)` for
      the filtered-out set, gated by
      `source.Spec.LogFiltered == nil || *source.Spec.LogFiltered`. Verify
      with a unit test that `logFiltered: false` suppresses only this event,
      while an `Unclaimed`/`AtCapacity`/`Ambiguous` drop on the same source
      still emits unconditionally.
- [ ] 2.3 Add a `ReasonFiltered` constant beside `ReasonAtCapacity` and set the
      batch response `reason` whenever `filteredOut` is non-empty, matching
      how the existing three reasons are reported through `out["reason"]`.
      Verify with an httpapi test asserting `reason` is set on a
      partially-filtered batch and unset on a fully-admitted one.
- [ ] 2.4 Verify, with a unit test, that a fully-filtered batch never calls
      `s.cooldown`/`cd.Fresh` and never changes `status.receivedTotal` —
      the insertion point in 2.1 keeps a filtered signal out of both lanes
      entirely, so this test is a regression guard on that property rather
      than new production code.

## 3. Unit tests

- [ ] 3.1 Run `go build ./... && go vet ./... && go test ./...` in
      `platform/manager` (this change's worktree), covering 1.1's generated
      CRD/deepcopy, 2.1-2.4's filter/logging/reason/bookkeeping behavior, and
      the manager's envtest suite (`KUBEBUILDER_ASSETS=... go test ./...`,
      `build-test.md`) asserting the API server rejects an unknown
      `attributeFilters[].operator` value at admission.

## 4. System tests

- [ ] 4.1 Not applicable — filtering is pure CRD-validated config evaluated
      in-process against already-normalized Go structs. No kubelet, RBAC,
      informer or pod-lifecycle decision is involved. Admission-time enum
      enforcement is exercised by the envtest suite in section 3, not the
      k3d-backed `test/system` tier.

## 5. Documentation

### Reference docs

- [ ] 5.1 Update the `SignalSource` field reference in `docs/concepts.md`
      with `attributeFilters` and `logFiltered`.
- [ ] 5.2 Run `python3 .github/scripts/docs-generate.py` (and `--check`) from
      this change's worktree to regenerate `docs/cr-reference.md`'s
      `SignalSource` fields table with both new fields.

### Adopter site

- [ ] 5.3 Add a hand-written section to `docs/guides/signal-adapter.md`
      explaining how to filter a source (`attributeFilters`'
      `key`/`operator`/`values`, and `logFiltered`), extend that guide's
      generated `SignalSource` example block's
      `fields=config,cooldownHours` marker to include
      `attributeFilters,logFiltered`, and re-run `docs-generate.py`. Verify
      the rendered example block shows both fields.
