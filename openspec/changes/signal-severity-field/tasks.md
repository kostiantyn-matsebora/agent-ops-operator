## 1. CRD and API types — api-architect

- [x] 1.1 Add a `Severity` type (closed string enum: `critical`, `error`,
  `warning`, `info`) to `platform/manager/api/v1alpha1/`, and an optional
  `Severity` field to `SignalProvenance` and to `ConversationInputSpec`.
  Verify `cd platform/manager && go build ./...` succeeds.
- [x] 1.2 Regenerate deepcopy and CRDs (`controller-gen@v0.16.5`, `object`
  then `crd` targets per `.claude/rules/build-test.md`). Verify the two
  CRD YAMLs under `chart/crds/` gain the `severity` property with its four
  enum values, and `go vet ./...` is clean.

## 2. Inbound validation — backend-developer

- [x] 2.1 Add an optional `Severity` field to `NormalizedSignal` in
  `internal/httpapi/signals.go`. Verify with `go build ./...`.
- [x] 2.2 Validate `Severity` against the four-value vocabulary ahead of
  any routing, refusing the whole batch with a 400 naming the accepted
  values on an unmapped value. Verify with a unit test posting an unknown
  value and asserting the 400 body names all four values, and a second
  test posting no severity at all and asserting today's existing
  behavior is unchanged.
- [x] 2.3 Thread the validated severity onto `SignalProvenance` at
  conversation creation and onto each created `ConversationInputSpec`,
  never overwriting an existing provenance on a later input. Verify with
  a unit or envtest case: a conversation opened by a `warning` signal
  keeps `warning` on its provenance after a later `error` signal lands,
  while the new input records `error`.

## 3. Outbound signal message — backend-developer

- [x] 3.1 Add an optional `Severity` field to the `signal` message kind in
  `internal/chat/message.go`'s `SignalMessage`, filled from the
  input/provenance the same way `Labels` already is. Verify with a unit
  test asserting the field is present when the input carries one and
  omitted when it does not.

## 4. Console — frontend-developer

- [ ] 4.1 Carry `Severity` through `platform/console/transcript.go`'s
  `Message` struct, filled from the inbound op the same way `Payload`
  already is. Verify with a unit test.
- [ ] 4.2 Add `severity?: string` to the frontend `Message` interface and
  draw it on the signal card in
  `platform/console/ui/src/pages/Conversation.tsx`, tinted per value
  (`critical`/`error` to `--ao-danger`, `warning` to `--ao-warning`,
  `info` to `--ao-neutral`), rendering exactly as today when absent.
  Verify with `npm run test` in `platform/console/ui`, covering a present
  and an absent case.

## 5. Unit tests

- [ ] 5.1 `cd platform/manager && go build ./... && go vet ./... && go
  test ./...` green, covering the new field, its validation, and the
  provenance/input storage behavior.
- [ ] 5.2 `cd platform/console && go build ./... && go vet ./... && go
  test ./...` green, covering the transcript field.
- [ ] 5.3 `cd platform/console/ui && npm run test` green, covering the
  signal card's severity rendering.

## 6. System tests

- [ ] 6.1 Nothing here is decided by a cluster. No kubelet, RBAC,
  informer, pod-lifecycle, or context-continuity behavior changes —
  validation, storage, and rendering are all unit- and envtest-covered in
  section 5.

## 7. Documentation

### Reference docs

- [x] 7.1 `docs/concepts.md`: note the normalized signal shape's optional
  `severity` field and its vocabulary, distinct from any adapter's own
  `severity` label.
- [ ] 7.2 `docs/contracts.md`: the inbound signal shape and the `signal`
  message shape both gain `severity`.
- [ ] 7.2a `docs/contracts/signal-adapter.md`: the optional `severity` field,
  its four-value vocabulary, and the 400 refusal of an unmapped value.
- [ ] 7.3 Re-run `python3 .github/scripts/docs-generate.py` and commit the
  regenerated `docs/cr-reference.md` blocks for the two CRD-backed types.
- [ ] 7.4 `docs/console.md`: what the signal card now draws when present.
- [ ] 7.5 `docs/CHANGELOG.md`: add an entry for the new optional field,
  marked non-breaking.
- [ ] 7.6 `docs/guides/signal-adapter.md`: note that `severity` is a
  contract field no shipped adapter sets yet.

### Adopter site

- [ ] 7.7 `docs/console-guide.md`: a severity mark can appear in the
  transcript view.
- [ ] 7.8 Re-run `npm run screenshots` and `npm run demo` in
  `platform/console/ui` to refresh the published console captures.
