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

- [x] 4.1 Carry `Severity` through `platform/console/transcript.go`'s
  `Message` struct, filled from the inbound op the same way `Payload`
  already is. Verify with a unit test.
  - Also threaded through the REHYDRATION path (`rehydrate.go`,
    `conversations.go`'s `ConversationSummary.SignalSeverity`, read off
    the CR's `spec.signal.severity`), the same way `SignalLabels` already
    is — otherwise the live card's severity tag would vanish the moment
    the console restarted. Not named by this task, but the same gap the
    proposal's own provenance design implies should not exist, and
    `Labels` sets the precedent for closing it.
- [x] 4.2 Add `severity?: string` to the frontend `Message` interface and
  draw it on the signal card. **`platform/console/ui/src/pages/Conversation.tsx`
  does not exist in this tree** — the signal card renders in
  `platform/console/ui/src/pages/chat/Timeline.tsx`, which is where this
  landed, tinted per value (`critical`/`error` to `--ao-danger`, `warning`
  to `--ao-warning`, `info` to `--ao-neutral`), rendering exactly as today
  when absent. Verified with `npm run test` in `platform/console/ui`,
  covering a present and an absent case.

## 5. Unit tests

- [x] 5.1 `cd platform/manager && go build ./... && go vet ./... && go
  test ./...` green, covering the new field, its validation, and the
  provenance/input storage behavior.
- [x] 5.2 `cd platform/console && go build ./... && go vet ./... && go
  test ./...` green, covering the transcript field.
- [x] 5.3 `cd platform/console/ui && npm run test` green, covering the
  signal card's severity rendering.

## 6. System tests

- [x] 6.1 Nothing here is decided by a cluster. No kubelet, RBAC,
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
  - **BLOCKED, not done.** `docs/contracts.md` itself holds no shape
    table for either — both live on the two pages below, and both are
    gated the same way 7.2a is. See the note on 7.2a — this line and 7.6
    share its cause and its resolution.
- [ ] 7.2a `docs/contracts/signal-adapter.md`: the optional `severity` field,
  its four-value vocabulary, and the 400 refusal of an unmapped value.
  - **BLOCKED, not done — a human decision, not an implementation gap.**
    `.claude/rules/signal-rules.md` and `.claude/rules/channel-spec.md`
    both state their contract page is NEVER edited without the user's
    explicit approval, and that an approved edit brings every shipped
    adapter into conformance IN THE SAME CHANGE — for the signal
    contract, `cron`, `alertmanager`, `k8s-events`, `ha`, `telegram`,
    and for the channel contract, `channels/telegram`,
    `platform/console` — plus a version bump on each page. This
    change's own `design.md` scopes that out on purpose
    (Non-Goals: "Mapping any shipped adapter onto the field"). Editing
    either locked page here would either violate the approval gate or
    silently drag in out-of-scope adapter work the proposal never asked
    for. The code is already correct and additive
    (`COMPAT-2`/`COMPAT-3`-style: an unknown field is ignored, not
    required) — only the two VERSIONED SPEC PAGES are unwritten, pending
    the user's explicit go-ahead on a follow-up (either approve the two
    edits plus their conformance bumps, or accept the field as an
    intentionally-undocumented, backward-compatible extension until a
    later change states it).
- [x] 7.3 Re-run `python3 .github/scripts/docs-generate.py` and commit the
  regenerated `docs/cr-reference.md` blocks for the two CRD-backed types.
- [x] 7.4 `docs/console.md`: what the signal card now draws when present.
- [x] 7.5 `docs/CHANGELOG.md`: add an entry for the new optional field,
  marked non-breaking.
- [ ] 7.6 `docs/guides/signal-adapter.md`: note that `severity` is a
  contract field no shipped adapter sets yet.
  - **BLOCKED, not done — same cause as 7.2/7.2a.** This guide states no
    requirement of its own and treats `docs/contracts/signal-adapter.md`
    as the one source of truth, deferring to it by its own stated rule.
    Describing `severity` here before that locked page says so would
    make the guide, not the spec, the first place a reader meets a
    claim nobody approved.

### Adopter site

- [x] 7.7 `docs/console-guide.md`: a severity mark can appear in the
  transcript view.
- [ ] 7.8 Re-run `npm run screenshots` and `npm run demo` in
  `platform/console/ui` to refresh the published console captures.
  - The fixture's one signal message (`screenshots/fixture.ts`) gained
    `severity: 'critical'`, so the published `conversation-{light,dark}.png`
    actually show the new tag rather than refreshing to the same pixels.
  - `npm run screenshots` ran and its 18 PNGs are updated.
  - `npm run demo` ran the recorder, but the ffmpeg encode step shells out
    to `docker run mwader/static-ffmpeg`, and this sandbox has no docker
    daemon reachable — **not run, not claimed done**. The one touched
    byproduct (`console-demo-poster-light.png`, written before the failed
    encode) was reverted rather than committed half-refreshed. A
    workstation session with Docker owes this one.
