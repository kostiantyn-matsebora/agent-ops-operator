# Design — signal-adapter-spec-compliance

## Context

See proposal.md — Why. The audit's full findings, per adapter with code
locations, ground this change. Every adapter is already behaviorally on the
2.0.0 sender-minted `initiationId`, so nothing here touches the wire shape.

Two constraints shape the approach:

- `docs/contracts/signal-adapter.md` is never edited without explicit
  approval (`signal-rules.md`). The deltas state what the page must say.
  The user's approval of this change is the approval, and the page is
  written only at apply.
- A MINOR version move on the contract page triggers the conformance walk:
  every shipped adapter and every claimed version moves in this same
  change. That walk is this change's content anyway.

## Goals / Non-Goals

**Goals:**

- Every shipped signal adapter conformant with its specs, verified against
  the page, claimed versions moved to 2.2.0 (they sit at 2.0.0 today —
  the 2.1.0 severity move was additive and deliberately moved none).
- One rule left standing wherever two spec sentences disagreed.
- `pulse` named everywhere "every shipped adapter" is enumerated, its
  binary in the conformance suite.

**Non-Goals:**

- No wire or CRD change. No controller-gen run.
- No manager-side change — every fix is adapter-side or spec-side.
- No retired-vocabulary guard extension to Go files. The terms are already
  in the JSON, the guard's window logic is tuned for prose, and the Go
  comments are simply purged here.
- No new signal types, surfaces or suppression axes.

## Decisions

### Contract decisions (settled by the delta specs)

The deltas are normative. Summary of what they settled:

| Decision | Settled as |
|---|---|
| Refractory window | blessed as the contract's ONE named ADP-3 carve-out, five bounds, declared per adapter — `ha` and `k8s-events` deltas carry the declarations |
| Pulse secret rotation | create-or-repair keys on a non-secret digest registered beside the webhook, so redaction cannot blind the repair |
| Pulse token rotation | "heals without manual re-registration" — env is fixed per pod lifetime, so the heal follows the next pod start |
| ha id scope | the dwell-path mint wins — dwelled conditions key on source+integration, zero-dwell on the record's own identity |
| Suppression rung 3 | the scenarios win — recurrence gates undeterminable existence, fail-open applies to the adapter's OWN unreadable state |
| Alertmanager register | no `name` field, `namespace` defaults, knobs omitted when unset, four-row degradation table, remediation names only live CRD fields |
| Signature enforcement | follows the secret, not registration state — a hand-registered Pulse presenting the same secret verifies identically |

The last row goes one sentence beyond the proposal's pulse bullet. It is
deliberate: the unsigned-inbound fix had no clean spec sentence without it.

### Implementation decisions

- **Pulse digest header.** `X-Agentops-Secret-Digest: sha256:<hex>` of the
  signing secret, registered in `webhookHeaders`. Pulse returns headers
  unredacted, so `comparable()` keeps them and rotation shows as drift.
  The delta deliberately does not pin the name — this design does.
- **k8s-events relist clip.** On a list pass that hits the batch cap, the
  cursor advances only to the newest EXAMINED event, and the clip is
  reported on the source's Ready condition. No pagination rework — the
  next pass resumes from the honest cursor.
- **ha fail-open.** `health()` gets a third verdict for "the log itself is
  unreadable" (nil or stale snapshot), distinct from "record present, no
  predicate". `decide()` emits on it unconditionally, matching the ladder
  comment the code already carries.
- **k8s-events fail-open.** Same split: unsynced cache or permanently
  denied reads at the close → emit. A tracked kind absent from a synced
  cache stays rung-2 recurrence-gated.
- **Mute visibility.** The existing drain sweep timer also closes mute
  windows: `MuteEnded` with the count fires from the sweep, not from the
  next batch. The active report carries the running count.
- **Shadowed-rule warnings.** `refreshSources` appends `ruleSet.warnings`
  to the Ready report it already posts — detection exists, only the wiring
  is missing.
- **Remediation texts.** Both name `spec.serviceAccountName` and the
  chart-rendered account, per the delta's "only fields the current CRD
  carries".
- **send_resolved.** Becomes `*bool` in the register block, omitted when
  nil — the one knob that was not already omit-when-unset.
- **Malformed register block.** `parseRegister` returns the unmarshal error
  distinct from absence, surfaced as RegistrationManual naming the parse
  problem.
- **cron reported map.** Pruned to the served set on every refresh — fixes
  both the recreation suppression and the unbounded growth.
- **telegram title.** Truncate on a rune boundary with a non-ASCII test.
- **Per-pod fallback bound (k8s-events).** The delta says "short bounded
  wait". Bound: one informer resync tick, not a new constant — the cache
  either resolves the owner by then or the object is gone.
- **emitDrainExceeded cap bypass.** Kept, documented in code: volume is
  bounded to one signal per node per drain, and routing it through the cap
  could silence the one signal the bound exists to deliver.

## Risks / Trade-offs

- [Blessing the refractory window weakens ADP-3's simplicity] → the five
  bounds table keeps it inert: same id, post-emit, dwell-capped, declared.
  An adapter cannot grow a dedup policy inside it.
- [Pulse digest header leaks secret material] → sha256 of the secret is
  one-way, and the header travels only to the Pulse instance that already
  holds the secret itself.
- [The ha id-scope delta ratifies coarser grouping than the old surfaces
  table promised] → deliberate: the shipped defaults already behave this
  way, and per-entry ids under a dwell would split one integration's
  history. The table now says what ships.
- [Version walk touches five doc pages at once] → `docs-generate.py
  --check` and the conformance suite pin the claims mechanically.

## Migration Plan

No deployment migration. All fixes are adapter-internal, images roll with
the next release tags. The contract page's 2.2.0 entry is additive —
existing third-party adapters stay conformant at 2.0.0 claims.

## Open Questions

None. The one deferred constant (pulse digest header name) is settled
above.
