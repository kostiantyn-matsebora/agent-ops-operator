# Tasks: smart-k8s-events

Ordered so the loop breaker lands first and independently — it is the one part
that is urgent on a live cluster, and it needs neither the informer nor the
rules engine to be useful.

## 1. Self-exclusion (the loop breaker)

- [x] 1.1 Add `signal-k8s-events/selfexclude.go`: name-prefix predicate (`agentops-conv-`, `agentops-adapter-`, `agentops-signal-`) over `involvedObject.name`, requiring no object read
- [x] 1.2 Add the release-namespace predicate, reading `POD_NAMESPACE` (already injected by `kubernetesAccess`), with a config override
- [x] 1.3 Wire both into `deliver()` BEFORE filter matching, so excluded events never reach the rules engine or the cursor logic
- [x] 1.4 Tests: a `agentops-conv-*` pod event emits nothing; a same-namespace non-agentops object emits when the override is set; the prefix rule works with an empty object cache
- [x] 1.5 Record the invariant in `CLAUDE.md` next to no-relay-loops

## 2. Object cache (pods + replicasets)

- [x] 2.1 Extend `kube.go` with generic list/watch over an arbitrary resource path, reusing the existing `ErrWatchExpired`/relist handling
- [x] 2.2 Add `podcache.go`: a trimmed cache holding owner references, phase, `Ready` condition, container waiting reasons, and selected labels — never whole pod objects
- [x] 2.3 Add the replicaset cache (owner references only) for the second owner hop
- [x] 2.4 Scope watches to the same namespaces the events watch covers; re-reconcile scopes when a source's `namespaces` changes (resolve the design's open question first)
- [x] 2.5 Report a missing pods/replicasets grant as `Ready=False` naming the permission, rather than degrading silently
- [x] 2.6 Tests: cache populates from list, updates on watch, survives 410 relist, and reports a 403 as a source condition

## 3. Enrichment (`workload`, `node`, labels)

- [x] 3.1 Owner resolution: Pod → ReplicaSet → Deployment, plus the direct-controller case (StatefulSet, DaemonSet, Job); unowned objects are their own workload
- [x] 3.2 Add `workload`, `node`, and selected pod labels to `normalize()`'s label map
- [x] 3.3 Tests pinning the owner chain for Deployment, StatefulSet, DaemonSet, Job, and bare pods — explicitly asserting that no name parsing is involved
- [x] 3.4 Tests: enrichment degrades to the un-enriched label set (no `workload`) when the cache has no entry, rather than blocking the signal

## 4. Rules engine

- [x] 4.1 Matcher parser: `=`, `!=`, `=~`, `!~` with quoted values over a flat label map; reject anything else with a precise error
- [x] 4.2 Ordered first-match-wins evaluation with `action: drop` and `for` durations; empty `matchers` = catch-all
- [x] 4.3 Translate legacy `includeReasons`/`excludeReasons` into equivalent rules so existing sources are unaffected
- [x] 4.4 Shadowed-rule detection → warning on the source's `Ready` condition, without failing the source
- [x] 4.5 Config validation errors → False `Ready` condition naming the offending rule (extends the existing `parseConfig` error path)
- [x] 4.6 Tests: ordering, each operator, drop-before-dwell, legacy translation, invalid syntax, shadowed rule warning

## 5. Dwell + liveness re-check

- [x] 5.1 Pending queue keyed by involved object, coalescing repeat events (count per reason, distinct object count, first-seen)
- [x] 5.2 Window ends `for` after the FIRST matched event — later events must not extend it
- [x] 5.3 Verification ladder rung 1 — kind-specific health predicates: Pod (phase, `Ready`, waiting reasons), Node (`Ready`), Job (`Failed`), PVC (phase)
- [x] 5.4 Rung 2 — for kinds with no predicate, decide on **recurrence during the window**, not on existence
- [x] 5.5 Rung 3 — **fail open (emit)** only when existence itself cannot be determined
- [x] 5.6 `for: 0` bypasses the queue entirely
- [x] 5.7 `escalateAfterObjects`: emit early once N distinct objects of one workload are pending
- [x] 5.8 Enriched payload: per-reason counts, object count, first-seen, confirmed-at
- [x] 5.9 Tests: healthy rollout emits nothing (both the gone and the recovered branch); broken rollout emits exactly once; burst coalesces; `for: 0` is immediate; unevaluable kind emits
- [x] 5.10 Tests for rung 2 specifically: a one-off warning on an uninspectable kind drops, a recurring one emits
- [x] 5.11 Test breadth escalation fires before the dwell elapses
- [x] 5.12 Test the pending queue survives a relist without double-emitting

## 6. Inhibition

- [x] 6.1 `route.inhibitRules` with `sourceMatchers`/`targetMatchers`/`equal`, evaluated before the dwell queue
- [x] 6.2 Active-source tracking with a bounded TTL (an inhibiting condition must expire, or one node event silences its pods forever)
- [x] 6.3 Tests: node-down inhibits its own pods' `Unhealthy`; a pod on a healthy node is unaffected; inhibition expires

## 7. Emit cap

- [x] 7.1 Per-source per-minute signal cap; stop emitting for the window when reached
- [x] 7.2 Report clipping on the source's `Ready` condition with the clipped count — never silent
- [x] 7.3 Tests: normal volume untouched; clipping reported

## 8. Chart

- [x] 8.1 `chart/charts/k8s-bundle/templates/events.yaml`: add pods/replicasets `list`/`watch` to both the ClusterRole and the namespaced Role paths
- [x] 8.2 Update the `SignalAdapter`'s `configSchema` to describe `rules` and `route`
- [x] 8.3 `values.yaml`: the six-tier default `rules` from design.md Decision 8, `signatureLabels: [namespace, workload]`, comments carrying the per-tier rationale (a rule set whose reasoning is not written down gets "tidied" into a broken one)
- [x] 8.4 `internal/integration/charttemplate_test.go`: pin the rendered RBAC verbs and the default signature labels
- [x] 8.5 Pin the default rules by test: every past-tense reason carries `for: 0`; the final rule is a catch-all with a dwell and not a drop; no reason appears in more than one tier

## 9. Docs

- [x] 9.1 `docs/k8s-bundle.md`: the values surface plus a rules cookbook (drop noise, dwell a flappy reason, urgent `for: 0`, inhibit by node)
- [x] 9.2 `docs/concepts.md`: the signal label vocabulary, including `workload`
- [x] 9.3 `CHANGELOG.md`: **BREAKING** default `signatureLabels` change — existing per-pod conversations orphan and age out of the 7-day window; no action required
- [x] 9.4 `CLAUDE.md`: self-exclusion invariant; the Prometheus-`for:` vs Alertmanager-`group_wait` distinction, so it is not re-litigated
- [x] 9.5 Note the stopgap values override in the changelog entry for anyone on the old image

## 10. Verification

- [x] 10.1 `go build ./... && go vet ./...` in `signal-k8s-events/` and at the root; `go test ./...` in both
- [x] 10.2 `helm template` the bundle with defaults and with `rbac.clusterWide=false`; confirm both RBAC paths cover all three resources
- [x] 10.3 Server-side dry-run the rendered manifests against the live cluster before applying
- [x] 10.4 Live smoke: roll a Deployment and confirm no conversation opens; break the same Deployment's image and confirm exactly one opens
- [x] 10.5 Live smoke for the loop: set an impossible resource request on the runtime so conversation pods cannot schedule, and confirm no new conversations appear
