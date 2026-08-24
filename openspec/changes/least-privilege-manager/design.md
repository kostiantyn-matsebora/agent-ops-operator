## Context

See `proposal.md` — Why. What shapes the approach is that the verification is
the hard part and the edit is three lines.

**Removing a permission is not obviously safe**, and this project has already
paid for getting it wrong in the other direction: the comment beside the rule
records a reconciler that attempted a `delete` it did not hold and wedged in a
forbidden loop, found on a live cluster rather than in a test. A forbidden loop
is silent from the outside — the reconciler retries, nothing progresses, and the
symptom surfaces far from the cause.

So the constraint is: **establish that no caller exists before deleting the
grant, by more than one method, and settle the one question RBAC intuition gets
wrong.**

That question is whether assigning an identity needs a permission on it. It is
the natural assumption — several resources do work that way — and it is the
reason this grant looked load-bearing for as long as it did.

## Goals / Non-Goals

**Goals:**

- The manager's Role holds nothing on `serviceaccounts`.
- The absence is pinned by a test, so re-adding it fails rather than passes.
- The class of drift is closed, not just this instance.

**Non-Goals:**

- **Auditing the manager's other grants.** `pods`, `deployments`, `services`,
  `configmaps`, `persistentvolumeclaims` and `leases` all have live callers. A
  full least-privilege sweep is a different change with a different risk profile.
- **Changing any non-test line of the operator.** If this change needed one, the
  claim that nothing calls the API would be false.
- **Editing the documents that already state the rule.** `pipeline-model`,
  `docs/concepts.md` and `wiring.md` become TRUE. Editing them would be fixing
  the half that was right.
- **Reworking the guard.** Its scan set is a list in a JSON file, and this widens
  the list.

## Decisions

### D1 — Delete the rule, do not narrow it

Every verb on it is unused, so narrowing to `get`/`list`/`watch` would keep the
part that reads as a deliberate capability while removing the part that is
obviously stale.

**Alternative considered: keep `get` for a future `Ready` validation.** Rejected,
and `pipeline-model` already argued it: validating the reference would spend a
standing permission to produce a WARNING, when the failure it pre-empts is
already loud, local and names the account at admission.

### D2 — The claim is settled by MEASUREMENT, on a cluster

Four checks, and the last two are the ones that matter:

| Check | Answers |
|---|---|
| no `corev1.ServiceAccount` in non-test code | is there an explicit call |
| no `Owns` / `Watches` / cache entry for the type | is there an INFORMER, which needs `list`/`watch` with no visible read |
| a pod created naming an account, by an identity denied every verb on it | does ASSIGNMENT need a permission |
| a pod naming an account that does not exist | who refuses it, and on whose authority |

The third and fourth were run against a live cluster in an isolated namespace.
The pod was admitted, the account bound, the token projected and the pod ran. The
missing-account case was refused by the API server at admission, naming the
account, with no requester permission consulted.

**The informer check is the one that is easy to skip**, because a controller can
need `list` and `watch` on a type it never explicitly reads. Grepping for reads
alone would have produced a confident wrong answer.

**Reasoning would have produced the wrong answer here**, which is why it is a
measurement. "The manager assigns the account, so it must be able to read it" is
how several other resources behave.

### D3 — The test flips from asserting the shape to asserting the absence

`TestManagerCannotDeleteServiceAccounts` renders the chart, finds the
`serviceaccounts` rule, and fails if the next line grants `delete`. **It also
fails when no such rule exists at all**, so it would fail on this fix — the test
that guarded the grant is the test that has to change.

Rewritten, it asserts the manager's Role names the resource nowhere. That is
strictly stronger: the old test permitted every verb except one.

**Alternative considered: delete the test.** Rejected. It exists because a
forbidden loop was found on a live cluster, and the lesson survives the specific
verb — what changed is that the answer went from "not that verb" to "not this
resource".

### D4 — The guard's scan set is widened once, to what "published" means

The gap is not that one file was forgotten. It is that the scan set was written
as the files somebody had in mind rather than as a definition.

Widening it to the root policy files and `docs/adr/` costs one correction —
measured, not assumed: run over every published file the guard misses and it
reports a single occurrence.

**Alternative considered: fix the one line and leave the scan set.** Rejected —
that repairs the instance and keeps the mechanism that produced it, and the next
stale claim in the same file passes CI exactly as this one did.

**Alternative considered: scan the whole tree.** Rejected: the guard's subject is
what a STRANGER READS, and `.claude/rules/` deliberately records retired
vocabulary as history. Scanning it would fail the build on the files whose job is
to remember.

## Risks / Trade-offs

- **A caller exists that all four checks missed** → The failure mode is a
  forbidden loop, silent from outside. Bounded by running the operator's
  integration suite, which exercises both adapter reconcilers against a real API
  server, and by the grant being restorable in one line.

- **The measurement was taken on one Kubernetes version** → Assignment without a
  verb on the account is core API-server behaviour rather than a distribution's
  choice, and the missing-account refusal comes from the same admission plugin.
  Both were exercised on the cluster this project actually runs on.

- **Widening the scan set surfaces findings this change did not plan for** →
  Measured first: exactly one. Had it been twenty, the scan-set change would have
  been its own change rather than a bullet in this one.

- **`manager-rbac` is a capability with two requirements** → Small, and
  deliberately so. It exists because no capability owned the manager's Role,
  which is how a grant outlived its caller while three documents asserted its
  absence. The alternative — filing it under an adapter's lifecycle spec — makes
  an operator-wide fact look adapter-specific.

## Migration Plan

Not applicable. Removing a permission nothing exercises changes nothing an
adopter can observe, and there is no upgrade step.

**Rollback** is restoring the rule in `chart/templates/rbac.yaml` and reverting
the test. Nothing persists, and no object is created or deleted by this change.
