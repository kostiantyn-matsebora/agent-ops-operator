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
    table for either — both live on the two LOCKED pages below, each
    with its own entry (7.2a, 7.2b) rather than one combined note, so
    either blocker is found on its own line, not inferred from the
    other's prose.
- [ ] 7.2a `docs/contracts/signal-adapter.md`: the optional `severity` field,
  its four-value vocabulary, and the 400 refusal of an unmapped value.
  - **BLOCKED, not done — a human decision, not an implementation gap.**
    `.claude/rules/signal-rules.md`: "NEVER edited without the user's
    explicit approval," and an approved edit brings every shipped
    SIGNAL adapter — `cron`, `alertmanager`, `k8s-events`, `ha`,
    `telegram` — into conformance IN THE SAME CHANGE, plus a version
    bump on this page. This change's own `design.md` scopes that out on
    purpose (Non-Goals: "Mapping any shipped adapter onto the field").
    The code is already correct and additive (`COMPAT-2`-style: an
    unknown field is ignored, not required) — only this VERSIONED SPEC
    PAGE is unwritten, pending the user's explicit go-ahead (approve the
    edit plus its conformance bump, or accept the field as an
    intentionally-undocumented, backward-compatible extension for now).
- [ ] 7.2b `docs/contracts/channel-adapter.md`: the outbound `signal`
  message's "Message fields" table gains `severity`.
  - **BLOCKED, not done — same cause as 7.2a, a DIFFERENT locked page.**
    `.claude/rules/channel-spec.md` states the identical rule for this
    page: "NEVER edited without the user's explicit approval," and an
    approved edit brings every shipped CHANNEL adapter —
    `channels/telegram`, `platform/console` — into conformance in the
    same change, plus a version bump here too. This one is the page this
    PR actually edits code against: `internal/chat/message.go`'s
    `Message`/`SignalMessage` is exactly the surface this table
    describes, and `platform/console` (one of the two named adapters) is
    wired to draw the field while `channels/telegram` is not — both
    deliberate, per `design.md`'s non-goal, but both are this page's
    conformance question to answer, not something a code change can
    settle on its own. Same two resolutions as 7.2a: approve this edit
    plus its conformance bump, or accept the field as undocumented here
    for now — the user's call, not assumed.
- [x] 7.3 Re-run `python3 .github/scripts/docs-generate.py` and commit the
  regenerated `docs/cr-reference.md` blocks for the two CRD-backed types.
- [x] 7.4 `docs/console.md`: what the signal card now draws when present.
- [x] 7.5 `docs/CHANGELOG.md`: add an entry for the new optional field,
  marked non-breaking.
- [ ] 7.6 `docs/guides/signal-adapter.md`: note that `severity` is a
  contract field no shipped adapter sets yet.
  - **BLOCKED, not done — same cause as 7.2/7.2a/7.2b.** This guide states no
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
