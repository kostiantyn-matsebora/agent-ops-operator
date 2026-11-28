## 1. Re-establish the claim before removing anything

D2 in `design.md`. The investigation was done during the audit that produced this
change, and it is repeated here rather than trusted, because the tree moves. A
forbidden loop is silent from outside, so the checks come before the edit.

Record the VERDICT, never a matched value.

- [x] 1.1 No explicit call — confirm no `corev1.ServiceAccount` appears in
  non-test manager code. **VERDICT: confirmed, none found**
- [x] 1.2 **No informer** — confirm no `Owns(&corev1.ServiceAccount{})`, no
  `Watches` on the type, and no per-object entry in the manager's `cache.Options`.
  **This is the check that is easy to skip**: a controller needs `list` and
  `watch` on a type it watches without ever reading one. **VERDICT: confirmed,
  no `Owns`/`Watches` on the type and no `cache.Options` entry for it**
- [x] 1.3 Assignment needs no verb — on a cluster, in a throwaway namespace,
  create a pod naming a ServiceAccount as an identity that answers `no` to
  `can-i get/list/watch serviceaccounts`. Confirm the pod is admitted, the
  account binds and its token is projected. **VERDICT: confirmed on a fresh k3d
  cluster — the creating identity held no verb on `serviceaccounts`, the pod
  reached `Running` bound to the named account with its token volume
  projected**
- [x] 1.4 The missing-account case is the API server's — confirm a pod naming an
  account that does not exist is refused AT ADMISSION, naming the account.
  **VERDICT: confirmed — refused with `error looking up service account
  <ns>/<name>: ... not found`, no requester RBAC consulted**
- [x] 1.5 Delete the throwaway namespace. **Done**
- [ ] 1.6 Confirm the LIVE Role matches the chart before trusting the chart. A
  hand-patched field survives every `helm upgrade`, so the rendered manifest is
  not evidence about the cluster. **NOT PERFORMED: this session has no access to
  the author's live install — workstation-only, see the pull request**
- [x] 1.7 Record each result as a verdict in this list

## 2. Remove the grant

- [x] 2.1 Delete the `serviceaccounts` rule from `chart/templates/rbac.yaml`.
  **The whole rule** — D1: every verb on it is unused, and narrowing it would
  keep the half that reads as deliberate
- [x] 2.2 Correct the comment above the adapter workload rules. It reads "owns
  adapter workloads + their zero-RBAC SAs" and the operator owns no
  ServiceAccounts
- [x] 2.3 Confirm no OTHER rule in that file was touched, by diff. The manager's
  remaining grants are out of scope and each has a live caller. **VERDICT:
  confirmed — `git diff chart/templates/rbac.yaml` touches only the deleted
  rule, its header comment and the corrected adapter comment**

## 3. Correct the security policy

- [x] 3.1 `SECURITY.md` — the *Out of scope* bullet names `rbacMode` as a current
  documented decision. State the current model instead: an account an install
  declares, stating its own rules, named on the routes that need it
- [x] 3.2 Confirm the replacement does not name the retired key as a current
  claim. Naming it as a RECORD is permitted, but this line has no reason to.
  **VERDICT: confirmed, `grep -n "rbacMode" SECURITY.md` finds nothing**

## 4. Close the gap that hid it

D4: the scan set was written as the files somebody had in mind rather than as a
definition of "published".

- [x] 4.1 Add the published root files and `docs/adr/` to the `scan` globs in
  `.github/retired-vocabulary.json`. **Added `SECURITY.md`, `CODE_OF_CONDUCT.md`,
  `.github/ISSUE_TEMPLATE/*.yml`, `.github/PULL_REQUEST_TEMPLATE.md` and
  `docs/adr/*.md`**
- [x] 4.2 Do NOT add `.claude/rules/`. Those files record retired vocabulary as
  history on purpose, and scanning them would fail the build on the documents
  whose job is to remember
- [x] 4.3 Run the guard over the whole widened set and fix what it reports.
  Measured at one occurrence during the audit — if it is now many, stop and split
  the scan-set change out, per D4. **VERDICT: 3 occurrences, not 1 — still far
  short of D4's "twenty" threshold for splitting out, so fixed in place:
  `SECURITY.md`'s `rbacMode` line (task 3), an incomplete `POST /task` row in
  `docs/adr/0002-coordinated-agents.md` finished rather than left truncated,
  that same ADR's `worker` row recognized as a record by widening the `worker`
  term's own `allow` list to match every other term's (it already said "retired
  vocabulary" in the same cell), and a `.github/ISSUE_TEMPLATE/bug_report.yml`
  placeholder updated from "home volume" to "context volume". Fixing the two
  ADR semicolons the hook flagged on the same file is incidental, not part of
  this requirement**
- [x] 4.4 Confirm the guard FAILS on the pre-change `SECURITY.md` once the scan
  set is widened. That is the proof the widening reaches the file, and it is the
  same argument as 5.3. **VERDICT: confirmed — exit 1 naming `SECURITY.md:76`
  with the widened scan set and the pre-change `SECURITY.md` content, exit 0
  once reverted to the committed fix**

## 5. Unit tests

D3: `TestManagerCannotDeleteServiceAccounts` fails when NO `serviceaccounts` rule
is found, so it fails on task 2.1. The test that guarded the grant is the test
that has to change.

- [x] 5.1 Rewrite it to assert the manager's Role names `serviceaccounts`
  NOWHERE, and rename it for what it now asserts. **Renamed
  `TestManagerCannotDeleteServiceAccounts` →
  `TestManagerHoldsNoServiceAccountsVerb`**
- [x] 5.2 Keep the reason in the comment. It exists because a reconciler once
  attempted a verb it did not hold and wedged in a forbidden loop on a live
  cluster — what changed is "not that verb" becoming "not this resource"
- [x] 5.2.1 **A second caller the audit's four checks missed, found by 5.4**:
  `internal/integration/cachescope_rbac_test.go`'s `cachedKinds()` hardcoded
  `&corev1.ServiceAccount{}` into the list of kinds the suite asserts the
  manager's cache CAN sync under the chart's Role — a fifth check this
  change's own verification needed, not a production caller. No reconciler
  watches or reads the type (tasks 1.1/1.2), so it is removed from that list,
  and `cachescope.go`'s doc comment corrected to match
- [x] 5.3 Confirm it FAILS against the pre-change chart. A test that passes both
  before and after is pinning nothing. **VERDICT: confirmed — fails against the
  chart with the `serviceaccounts` rule restored, passes with it removed**
- [x] 5.4 Run the operator's integration suite. It exercises both adapter
  reconcilers against a real API server, which is the check that a caller was
  missed. **VERDICT: green after fixing the cache-scope test's stale
  `ServiceAccount` entry (5.2.1) — no other failure traced to this change**
- [x] 5.5 Every module builds, vets and tests. **VERDICT: every module builds,
  vets and tests clean except pre-existing failures unrelated to this change
  (verified identical on unmodified `master`): `platform/console` (no built
  `ui/dist` in this environment), `platform/context-sync`'s two checkpoint
  tests, `platform/manager`'s two `TestRuntimePi*`/pi-env tests (the
  `@earendil-works/pi-coding-agent` npm package is not installed here), and
  `.github/tests/run.sh`'s `--verify`-on-a-workstation lines. The operator's
  envtest integration suite and conformance suite are both green**
- [x] 5.6 `python3 .github/scripts/publication-guard.py` and
  `python3 .github/scripts/retired-vocabulary-guard.py` both pass. **VERDICT:
  both clean**
- [x] 5.7 `openspec validate least-privilege-manager --strict`. **VERDICT:
  "Change 'least-privilege-manager' is valid"**

## 6. System tests

What a cluster decides about this change: whether an identity really needs no
verb on `serviceaccounts` to be assigned one, and whether the rendered Role is
one the API server accepts.

- [x] 6.1 `helm template` renders, and `--dry-run=server` against the cluster
  accepts the Role. **VERDICT: confirmed on a fresh k3d cluster — the rendered
  Role (single- and split-namespace layouts) was accepted server-side**
- [x] 6.2 The RBAC admission behaviour this change relies on — an identity with
  no verb on `serviceaccounts` is admitted naming one, a missing account is
  refused at admission with no requester RBAC consulted — was proven live on a
  fresh k3d cluster in section 1 (tasks 1.3, 1.4), not re-run here since
  removing an unused grant changes nothing about that behaviour. **VERDICT:
  confirmed, see section 1**

## 7. Documentation

Both halves, listed separately because they are skipped independently.

### 7.1 The reference docs

- [x] 7.1.1 `docs/CHANGELOG.md` — a `Security` entry naming the removed
  permission. Nothing to migrate, but an operator reading what a chart version
  changed about the operator's own rights must find it. **VERDICT: added an
  `## [Unreleased]` / `### Security` entry — this change does not itself bump
  `chart/Chart.yaml`, so it queues for the next release commit to pick up, as
  prior non-breaking changes have**
- [x] 7.1.2 `docs/concepts.md`, `.claude/rules/wiring.md` and
  `openspec/specs/pipeline-model/` — **confirm they need NO edit.** Their claim
  that the manager holds no such RBAC becomes true. Editing them would be fixing
  the half that was already right, and confirming it is the point. **VERDICT:
  confirmed — `docs/concepts.md`'s "`Ready` does not check that one exists.
  That would need an API read the manager holds no RBAC for" and
  `openspec/specs/pipeline-model/spec.md`'s "needs a `serviceaccounts` read the
  manager holds no RBAC for" were both already written as the current claim,
  and are now literally true rather than aspirational. No edit made**
- [x] 7.1.3 `.claude/rules/retired-vocabulary.md` — record that the guard's scan
  set is a definition of "published", not a list, and that adding a published
  document means adding it to the scan. **Done**
- [x] 7.1.4 No CRD field, chart value or api doc comment changed, so
  `docs-generate.py` is NOT required. Confirm that rather than assume it.
  **VERDICT: confirmed — `python3 .github/scripts/docs-generate.py --check`
  reports "71 generated file(s) up to date"**
- [x] 7.1.5 No console UI changed, so neither asset harness is required.
  Confirm. **VERDICT: confirmed — this change touches only `chart/`,
  `platform/manager/internal/`, `SECURITY.md`, `.github/retired-vocabulary.json`
  and documentation, no file under `platform/console/`**

### 7.2 The adopter site

- [x] 7.2.1 `docs/security.md` — it states what the platform itself holds, and
  the manager's permissions are part of that. Add nothing if the page's claims
  stay true, and say so deliberately rather than skipping the check. **VERDICT:
  confirmed — the page names no `serviceaccounts` verb and no `rbacMode`, and
  its existing least-privilege claims ("the manager reads no Secrets", "it
  holds no verb on `secrets`") stay true and are joined by one more unnamed
  resource the manager now holds nothing on. Nothing added**
- [x] 7.2.2 `docs/installation.md` — confirm no values table or section
  described the removed permission. **VERDICT: confirmed — `grep -n -i
  "serviceaccount\|rbac" docs/installation.md` finds nothing**
- [x] 7.2.3 Confirm the finished change matches what this list planned. Written
  last, it documents what the change ACTUALLY did — if a check in section 1 came
  back differently, the specs are updated to match, not the memory of it.
  **VERDICT: matches — the one planned deviation is task 1.6 (the live-Role
  comparison), explicitly out of reach for this session and recorded as
  workstation-only. Every other planned check ran and its verdict is recorded
  above. `specs/manager-rbac/spec.md` and `specs/documentation-structure/spec.md`
  need no update to match, since neither claimed anything this change's actual
  results contradict**
