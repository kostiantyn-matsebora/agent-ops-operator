# Tasks: signal-triage

Three stages, each independently shippable and independently revertible. The cap
lands first because it is the floor that makes the later stages safe to fail.

## 1. Creation cap (deterministic floor)

- [ ] 1.1 `api/v1alpha1/signalsource_types.go`: add the per-window creation cap to `GroupingSpec` (unset = no cap) and the `Throttled` condition to `SignalSourceStatus`
- [ ] 1.2 Regenerate deepcopy + CRDs (`controller-gen object` / `crd`)
- [ ] 1.3 `internal/ingest/`: creation-rate accounting per source, in-memory alongside `Cooldown`, with an injectable clock as `Cooldown` already has
- [ ] 1.4 `internal/httpapi/signals.go`: consult the cap in `routeSignalGroup` on the CREATE branch only — never on the attach branch
- [ ] 1.5 Set/clear the `Throttled` condition with the refused count and window end
- [ ] 1.6 Tests: cap refuses creation; attachment still works while throttled; recurrence still appends; window roll clears the condition; unset cap is unbounded
- [ ] 1.7 Integration test (envtest): a source that would create N conversations creates exactly the cap

## 2. Dedup seam

- [ ] 2.1 `internal/ingest/`: define the strategy interface and the three verdicts
- [ ] 2.2 Candidate collection: live conversations within the window, in namespace — reusing the existing signature-hash listing where possible
- [ ] 2.3 Deterministic strategy returning create unconditionally
- [ ] 2.4 Wire the seam into `routeSignalGroup`'s CREATE branch, after the cap
- [ ] 2.5 Reject an attach verdict naming a conversation outside the candidate set → treat as create
- [ ] 2.6 **Equivalence test**: the existing ingest/routing test suite passes unchanged with the seam in place — behavior identity demonstrated, not asserted

## 3. AI triage

- [ ] 3.1 `internal/dispatch/`: triage lane template — prompt, candidate rendering, and the expected verdict shape; empty allowlist, no MCP
- [ ] 3.2 Decide and implement the triage host (per-namespace reserved Conversation per design.md; resolve the per-namespace vs per-source open question first)
- [ ] 3.3 Resolve the `default` AgentRuntime for the triage unit, reusing `conversation_controller.go`'s existing resolution path rather than a second copy of it
- [ ] 3.4 Verdict parsing from the run result, with a strict shape and an explicit unparsable path
- [ ] 3.5 Timeout → fail open (create); unavailable runtime → fail open; unknown candidate → fail open
- [ ] 3.6 Verdict cache keyed by signature with a bounded TTL
- [ ] 3.7 Bounded verdict record on `SignalSourceStatus`, with the reason required for every drop
- [ ] 3.8 Triage-lane self-exclusion: signals about the triage lane take the deterministic path; triage failures report as conditions and emit no signal
- [ ] 3.9 Reject attach verdicts naming conversations outside the candidate set (shares 2.5, verified again here as an injection defense)
- [ ] 3.10 Tests: each of the three verdicts; every fail-open path; cache hit; drop without reason rejected; triage never consulted for attach or throttled signals
- [ ] 3.11 Injection test: a signal payload instructing the agent to drop everything does not produce an un-audited drop and cannot produce an out-of-set attach

## 4. Chart

- [ ] 4.1 Ship the toolless triage profile, off by default
- [ ] 4.2 Values for the cap and the triage opt-in, with the cost model stated plainly in comments — novel-signature rate, not event rate
- [ ] 4.3 Chart template test pinning that triage renders nothing when disabled

## 5. Docs

- [ ] 5.1 `docs/concepts.md`: the cap, the dedup seam, and triage — including the ordering diagram, since "the model is asked last" is the property most likely to be misremembered
- [ ] 5.2 `docs/contracts.md`: the verdict contract and the fail-open rules
- [ ] 5.3 `CLAUDE.md`: that the no-Secrets invariant is what forces triage to be a work unit — this is the reasoning most likely to be re-litigated by someone proposing a direct model call
- [ ] 5.4 `CHANGELOG.md` only if a default changes (none planned — every stage defaults to off)

## 6. Verification

- [ ] 6.1 `go build ./... && go vet ./...`; full test run with `KUBEBUILDER_ASSETS`
- [ ] 6.2 Server-side dry-run the regenerated CRDs before applying
- [ ] 6.3 Live: set a low cap, provoke a burst, confirm `Throttled` and confirm existing conversations still receive inputs
- [ ] 6.4 Live: enable triage on one source, run a week, read the verdict record, and confirm every drop is explainable before recommending it anywhere
