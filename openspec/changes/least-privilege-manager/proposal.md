## Why

**The chart grants the manager permissions on `serviceaccounts` that nothing
calls, and three published documents already say it does not hold them.**

`openspec/specs/pipeline-model/spec.md`, `docs/concepts.md` and
`.claude/rules/wiring.md` all give the same reason for not validating
`spec.serviceAccountName`: checking an account exists *"needs a `serviceaccounts`
read the manager holds no RBAC for"*. The manager's Role grants `get`, `list`,
`watch`, `create`, `update` and `patch` on exactly that resource.

`documentation-structure` already names this case — a requirement that is true,
desirable and not satisfied by the code is recorded as a defect and raised as its
own change. This is that change.

**The grant outlived its caller.** `5dfb69b` added it for the ChannelAdapter
reconciler, which really did create adapter ServiceAccounts. `84a2654` deleted
that creation and never touched `chart/templates/rbac.yaml`.

**Nothing replaced the need.** Verified four ways:

- No `corev1.ServiceAccount` reference exists in non-test manager code.
- No `Owns` or `Watches` on the type, and no per-object cache entry — so no
  informer needs `list`/`watch`.
- **Assignment needs no verb**, measured on a live cluster rather than reasoned:
  an identity holding `pods` create and answering `no` to `can-i get/list/watch
  serviceaccounts` created a pod naming another account. The account bound, the
  token projected, the pod ran to `Succeeded`.
- **The missing-account case is the API server's**, not the caller's: naming an
  account that does not exist is refused at admission, naming it. No requester
  RBAC is consulted.

**A second, smaller drift shares a cause.** `SECURITY.md` names `rbacMode` as a
current documented decision. That key is deleted at every level and the chart
FAILS the render on one. It survived because the retired-vocabulary guard does
not scan `SECURITY.md` — its scan set is specs, `docs/*.md`, `docs/guides/`,
`README.md`, chart values and `NOTES.txt`, and nothing else. Run over the
published files it misses and it reports exactly one occurrence: that line.

**The security page now links `SECURITY.md` as authoritative**, which is what
turned a stale sentence into a page sending readers at it.

## What Changes

- **The manager's Role stops granting `serviceaccounts` anything.** The rule is
  deleted, not narrowed — every verb on it is unused.
- **The test that pinned the grant becomes the assertion that it is absent.**
  `TestManagerCannotDeleteServiceAccounts` currently fails when NO
  `serviceaccounts` rule is found, so it would fail on the fix. It is rewritten
  and renamed to assert the manager is granted no verb on the resource at all —
  strictly stronger than what it checked before.
- **The comment above the adapter rules stops describing deleted behaviour.** It
  reads *"owns adapter workloads + their zero-RBAC SAs"*, and the operator owns
  no SAs.
- **`SECURITY.md` states the current model** — an account an install declares
  stating its own rules — instead of naming a key the chart refuses.
- **The retired-vocabulary guard scans every published document.** The root
  policy files and `docs/adr/` join its scan set, so the class of drift that hid
  this one cannot hide the next.

**Not breaking.** Removing a permission nothing exercises changes no behaviour an
adopter can observe, and the reconcilers were verified against a live cluster
before the rule was touched.

## Capabilities

### New Capabilities
- `manager-rbac`: what the operator's own Role may hold — the rule that it grants
  only what the manager actually calls, the resources it is forbidden, and the
  verbs it holds on what it does use. No capability owned this, which is why a
  grant could outlive its caller with three documents asserting its absence.

### Modified Capabilities
- `documentation-structure`: the retired-vocabulary requirement is scoped to
  specs. It becomes every PUBLISHED document, and names the guard's scan set as
  the mechanism — a rule enforced over a subset of what it claims to cover is
  enforced nowhere the subset does not reach.

## Impact

**The chart**

- `chart/templates/rbac.yaml` — the `serviceaccounts` rule is deleted, and the
  comment above the adapter workload rules corrected.

**The operator**

- `platform/manager/internal/integration/servedby_test.go` —
  `TestManagerCannotDeleteServiceAccounts` rewritten as the absence assertion.
  **No non-test code changes**, which is the whole claim being made.

**The forge and the guard**

- `SECURITY.md` — one bullet under *Out of scope*.
- `.github/retired-vocabulary.json` — the `scan` globs gain the published root
  files and `docs/adr/`.

**Documentation**

- `docs/CHANGELOG.md` — a `Security` entry. Nothing to migrate, but an operator
  reading what a chart version changed about permissions must find it.
- `docs/concepts.md`, `.claude/rules/wiring.md`, `openspec/specs/pipeline-model/`
  — **read, not edited.** Their claim becomes true. That is the point, and
  editing them would be the wrong half of the fix.

**Not affected**

- No CRD, no contract, no adapter behaviour. `channel-adapter-lifecycle` and
  `signal-adapter-lifecycle` already require the reconciler to create no
  ServiceAccount, and that requirement is satisfied today — this change removes
  the permission it would have needed to break it.
