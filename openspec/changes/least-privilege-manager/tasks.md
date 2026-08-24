## 1. Re-establish the claim before removing anything

D2 in `design.md`. The investigation was done during the audit that produced this
change, and it is repeated here rather than trusted, because the tree moves. A
forbidden loop is silent from outside, so the checks come before the edit.

Record the VERDICT, never a matched value.

- [ ] 1.1 No explicit call — confirm no `corev1.ServiceAccount` appears in
  non-test manager code
- [ ] 1.2 **No informer** — confirm no `Owns(&corev1.ServiceAccount{})`, no
  `Watches` on the type, and no per-object entry in the manager's `cache.Options`.
  **This is the check that is easy to skip**: a controller needs `list` and
  `watch` on a type it watches without ever reading one
- [ ] 1.3 Assignment needs no verb — on a cluster, in a throwaway namespace,
  create a pod naming a ServiceAccount as an identity that answers `no` to
  `can-i get/list/watch serviceaccounts`. Confirm the pod is admitted, the
  account binds and its token is projected
- [ ] 1.4 The missing-account case is the API server's — confirm a pod naming an
  account that does not exist is refused AT ADMISSION, naming the account
- [ ] 1.5 Delete the throwaway namespace
- [ ] 1.6 Confirm the LIVE Role matches the chart before trusting the chart. A
  hand-patched field survives every `helm upgrade`, so the rendered manifest is
  not evidence about the cluster
- [ ] 1.7 Record each result as a verdict in this list

## 2. Remove the grant

- [ ] 2.1 Delete the `serviceaccounts` rule from `chart/templates/rbac.yaml`.
  **The whole rule** — D1: every verb on it is unused, and narrowing it would
  keep the half that reads as deliberate
- [ ] 2.2 Correct the comment above the adapter workload rules. It reads "owns
  adapter workloads + their zero-RBAC SAs" and the operator owns no
  ServiceAccounts
- [ ] 2.3 Confirm no OTHER rule in that file was touched, by diff. The manager's
  remaining grants are out of scope and each has a live caller

## 3. Flip the test from shape to absence

D3: `TestManagerCannotDeleteServiceAccounts` fails when NO `serviceaccounts` rule
is found, so it fails on task 2.1. The test that guarded the grant is the test
that has to change.

- [ ] 3.1 Rewrite it to assert the manager's Role names `serviceaccounts`
  NOWHERE, and rename it for what it now asserts
- [ ] 3.2 Keep the reason in the comment. It exists because a reconciler once
  attempted a verb it did not hold and wedged in a forbidden loop on a live
  cluster — what changed is "not that verb" becoming "not this resource"
- [ ] 3.3 Confirm it FAILS against the pre-change chart. A test that passes both
  before and after is pinning nothing
- [ ] 3.4 Run the operator's integration suite. It exercises both adapter
  reconcilers against a real API server, which is the check that a caller was
  missed

## 4. Correct the security policy

- [ ] 4.1 `SECURITY.md` — the *Out of scope* bullet names `rbacMode` as a current
  documented decision. State the current model instead: an account an install
  declares, stating its own rules, named on the routes that need it
- [ ] 4.2 Confirm the replacement does not name the retired key as a current
  claim. Naming it as a RECORD is permitted, but this line has no reason to

## 5. Close the gap that hid it

D4: the scan set was written as the files somebody had in mind rather than as a
definition of "published".

- [ ] 5.1 Add the published root files and `docs/adr/` to the `scan` globs in
  `.github/retired-vocabulary.json`
- [ ] 5.2 Do NOT add `.claude/rules/`. Those files record retired vocabulary as
  history on purpose, and scanning them would fail the build on the documents
  whose job is to remember
- [ ] 5.3 Run the guard over the whole widened set and fix what it reports.
  Measured at one occurrence during the audit — if it is now many, stop and split
  the scan-set change out, per D4
- [ ] 5.4 Confirm the guard FAILS on the pre-change `SECURITY.md` once the scan
  set is widened. That is the proof the widening reaches the file, and it is the
  same argument as 3.3

## 6. Close out

- [ ] 6.1 `helm template` renders, and `--dry-run=server` against the cluster
  accepts the Role
- [ ] 6.2 Every module builds, vets and tests
- [ ] 6.3 `python3 .github/scripts/publication-guard.py` and
  `python3 .github/scripts/retired-vocabulary-guard.py` both pass
- [ ] 6.4 `openspec validate least-privilege-manager --strict`

## 7. Documentation

Both halves, listed separately because they are skipped independently.

### 7.1 The reference docs

- [ ] 7.1.1 `docs/CHANGELOG.md` — a `Security` entry naming the removed
  permission. Nothing to migrate, but an operator reading what a chart version
  changed about the operator's own rights must find it
- [ ] 7.1.2 `docs/concepts.md`, `.claude/rules/wiring.md` and
  `openspec/specs/pipeline-model/` — **confirm they need NO edit.** Their claim
  that the manager holds no such RBAC becomes true. Editing them would be fixing
  the half that was already right, and confirming it is the point
- [ ] 7.1.3 `.claude/rules/retired-vocabulary.md` — record that the guard's scan
  set is a definition of "published", not a list, and that adding a published
  document means adding it to the scan
- [ ] 7.1.4 No CRD field, chart value or api doc comment changed, so
  `docs-generate.py` is NOT required. Confirm that rather than assume it
- [ ] 7.1.5 No console UI changed, so neither asset harness is required. Confirm

### 7.2 The adopter site

- [ ] 7.2.1 `docs/security.md` — it states what the platform itself holds, and
  the manager's permissions are part of that. Add nothing if the page's claims
  stay true, and say so deliberately rather than skipping the check
- [ ] 7.2.2 `docs/installation.md` — confirm no values table or section
  described the removed permission
- [ ] 7.2.3 Confirm the finished change matches what this list planned. Written
  last, it documents what the change ACTUALLY did — if a check in section 1 came
  back differently, the specs are updated to match, not the memory of it
