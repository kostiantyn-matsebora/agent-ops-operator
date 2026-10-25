## Context

See `proposal.md` for motivation. This note covers only what shapes the
approach.

`SignalSource.spec` today carries `adapter`, `cooldownHours`,
`credentialsSecretRef`, and opaque `config` — no admission filter of any
kind. Every signal that clears the source's `Wired` check and cooldown
reaches a serving pipeline.

Ingest lives in `platform/manager/internal/httpapi/signals.go`.
`handleSignalInbound` validates the request, resolves the `SignalSource`,
emits one `signal.received` event per signal, then dispatches to either
`routeSignals` (alert/job/task) or `routeChatSignals` (chat), both of
which check `Wired` before calling `cooldown()`/`cd.Fresh()`.

`k8s.io/apimachinery/pkg/apis/meta/v1` and `k8s.io/apimachinery/pkg/labels`
are already manager dependencies (the Kubernetes client brings them in).
No new dependency is needed for this change.

## Goals / Non-Goals

**Goals:**

- Correct `In`/`NotIn`/`Exists`/`DoesNotExist` matching against a signal's
  `labels`, with the same semantics a Kubernetes label selector applies.
- Zero behavior change for a `SignalSource` that sets no
  `attributeFilters` — identical routing, cooldown, and bookkeeping.
- No new obligation on any signal adapter.

**Non-Goals:**

- Filtering on anything other than attribute labels (`kind`, `payload`,
  `initiationId`, `reader` stay out of scope — see Decisions).
- Any change to the signal adapter wire contract.
- Any change to cooldown or `Wired` semantics themselves.

## Decisions

### Reuse the `matchExpressions` shape and matching semantics, not the upstream Go type verbatim

`metav1.LabelSelectorRequirement`'s `Operator` field carries no
`+kubebuilder` enum marker upstream, so embedding that type directly as
`AttributeFilters`' element type would generate a CRD schema accepting
any string for `operator` — the opposite of the spec's "an unknown or
malformed `operator` SHALL be rejected at the API level" requirement.

`platform/manager/api/v1alpha1/signalsource_types.go` therefore gains a
small owned type instead:

```go
type AttributeFilterOperator string

const (
	AttributeFilterIn           AttributeFilterOperator = "In"
	AttributeFilterNotIn        AttributeFilterOperator = "NotIn"
	AttributeFilterExists       AttributeFilterOperator = "Exists"
	AttributeFilterDoesNotExist AttributeFilterOperator = "DoesNotExist"
)

// +kubebuilder:validation:Enum=In;NotIn;Exists;DoesNotExist
type AttributeFilterRequirement struct {
	Key      string                  `json:"key"`
	Operator AttributeFilterOperator `json:"operator"`
	Values   []string                `json:"values,omitempty"`
}
```

(the `+kubebuilder:validation:Enum` marker is placed on the `Operator`
field, not the struct, in the actual implementation — shown here beside
the type for readability).

Matching itself is NOT reimplemented. Each `AttributeFilterRequirement` is
converted field-for-field into a `metav1.LabelSelectorRequirement`,
collected into a `metav1.LabelSelector{MatchExpressions: ...}`, and
handed to the existing `metav1.LabelSelectorAsSelector` plus
`Selector.Matches(labels.Set(...))`.

That function already encodes every operator's semantics. It is the same
code path `kubectl`/informers use for a real label selector — no bespoke
matching code, no new test surface for "what does `NotIn` mean against a
missing key".

**Alternative considered**: a bespoke filter DSL (string expression
parsed at ingest). Rejected — it would need its own parser, its own
validation, and its own tests for exactly the semantics
`matchExpressions` already has a well-known reading of.

**Alternative considered**: embed `metav1.LabelSelectorRequirement`
directly and validate `operator` with a CEL
`+kubebuilder:validation:XValidation` rule instead of an owned enum type.

Rejected in favor of the owned type. A CEL rule over a vendored type's
field is less discoverable in the CRD schema's own `enum:` than a native
OpenAPI enum, and the owned type costs one small Go file.

### A malformed value/operator combination fails closed, not loud

`metav1.LabelSelectorAsSelector` can still return an error at RUNTIME for
a requirement that passes the enum check but fails value-count validation
(e.g. `In` with an empty `values`, `Exists` with a non-empty one) —
`labels.NewRequirement`'s own validation, which the CRD enum marker does
not reach.

Such a requirement is treated as never satisfied. The signal it gates is
dropped, under the same `CodeFiltered` code any other filter drop uses.

Failing closed (never-match) over failing open (always-match) is the
only choice that keeps "a requirement I couldn't construct" from
silently admitting a signal it should have excluded.

### Filtering happens once, in `handleSignalInbound`, ahead of the lane split

`handleSignalInbound` validates the batch and resolves the `SignalSource`,
then emits `signal.received` for every signal — unconditionally, because
receipt is a delivery fact and holds whether a signal is later claimed,
deduped, or dropped (see that function's own comment).

Right after that emission and before `route := s.routeSignals` /
`routeChatSignals` is chosen, a new step partitions `in.Signals` into
`admitted` and `filteredOut` using `source.Spec.AttributeFilters`.
`filteredOut` is reported (subject to `logFiltered`, below) and dropped.
`route(...)` is then called on `admitted` only — skipped entirely when
`admitted` is empty.

Inserting the check here, rather than inside `routeSignals` AND
`routeChatSignals` separately, means:

- one implementation serves both lanes (alert/job/task and chat) — a
  chat source may filter too, e.g. on a sender attribute.
- a filtered signal never reaches `s.cooldown()` or `cd.Fresh()`, so it
  spends no cooldown state — satisfies "never reaches cooldown" without
  threading a new parameter through either routing function.
- a filtered signal never reaches `s.bumpReceived` either, since that call
  lives inside `routeSignals`/`routeChatSignals` and only runs over
  signals that reached them (see `receivedTotal`, below).

### `receivedTotal` is not incremented for a filtered-out signal

`bumpReceived` is called only from inside `routeSignals` (with `landed`,
the count that actually reached a conversation) and `routeChatSignals`
(with `answered`/the routed remainder) — both AFTER the lane has already
received only the `admitted` slice.

Because filtering happens upstream of the lane split, a filtered-out
signal is never part of either function's input and therefore never
reaches `bumpReceived`.

`status.receivedTotal` counts only signals that were, at minimum,
eligible for cooldown and routing. A filtered signal never reached
ingest counting, by construction rather than by a separate conditional.

### The new drop code and what `logFiltered` gates

`platform/manager/internal/activity/event.go` gains one more bounded drop
code beside `CodeUnclaimed`/`CodeAtCapacity`/`CodeAmbiguous`:

```go
// CodeFiltered: a signal failed the source's attributeFilters.
CodeFiltered = "filtered"
```

`emitDropped(source, filteredOut, activity.CodeFiltered, reason)` is
called exactly where the other three are already called — same function,
same event shape. The only new condition is the gate in front of THIS one
call:

```go
if source.Spec.LogFiltered == nil || *source.Spec.LogFiltered {
	s.emitDropped(source, filteredOut, activity.CodeFiltered, reason)
}
```

No other `emitDropped` call site gains this gate — `CodeUnclaimed`,
`CodeAtCapacity`, and `CodeAmbiguous` keep logging unconditionally,
exactly as the spec requires.

### The inbound response's `reason` reports a filter drop, same as the other three

`ReasonAtCapacity` already sets the batch-level `reason` string returned to
the caller (and surfaced in `out["reason"]`) whenever any group in the
batch was dropped for that cause, alongside the `Unclaimed`/`Ambiguous`
reasons set unconditionally for a whole-batch drop.

A filter drop gets the same treatment, for parity: a new
`ReasonFiltered` constant, set on the response whenever `filteredOut` is
non-empty.

This follows the same "overwrite is acceptable, single-string summary,
not a per-signal structure" convention the existing three reasons
already use. It is not a new response-shape concept — it reuses the one
field every other drop reason already reports through.

### Non-Goal restated: `docs/contracts/signal-adapter.md` is untouched

Confirmed with the user: filtering is manager-side ingest only. No
signal adapter gains visibility into `SignalSource.spec.attributeFilters`
or any role in evaluating it, so the adapter wire contract carries no
change and no version bump.

## Risks / Trade-offs

**An overly narrow filter silently drops every signal a source
receives** → mitigated by `logFiltered`'s default of `true`: the drop is
still visible on the source's activity feed (`signal.dropped`, code
`filtered`) even though nothing routes.

An operator who sets `logFiltered: false` on top of an overly narrow
filter loses that visibility too. That is the trade-off
`logFiltered: false` exists to accept, for a source already known to be
noisy.

**A value-count-malformed requirement is accepted by the CRD.** The enum
marker only checks `operator`, not the `In`/`NotIn` vs
`Exists`/`DoesNotExist` value-count rule, so a right-operator,
wrong-`values`-arity requirement is still admitted.

→ mitigated by failing closed at evaluation time (see Decisions). The
worst case is an over-broad drop, never an under-broad admit.
