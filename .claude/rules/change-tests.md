## Tests are tasks (every change ends with unit, e2e, then documentation)

**THE TWO SECTIONS BEFORE THE DOCUMENTATION SECTION ARE TESTS.** In this
order, each its own numbered `## ` section, every task ticked before the change
is finished:

| Position | Section | Covers |
|---|---|---|
| last − 2 | **Unit tests** | what the change did — `go test ./...` in every module touched, `node --test` in a Node runtime, the chart's render tests for a template, `.github/tests/run.sh` for a workflow script |
| last − 1 | **E2E tests** | anything a CLUSTER decides — the kubelet, RBAC, an informer, a pod's lifecycle, context continuity — as a lane in `platform/manager/test/e2e/`, and the pack run |
| last | **Documentation** | both halves, see `documentation.md` |

- **Not a bullet under the task it verifies.** A test line appended to
  "implement X" is ticked with X and never written — the same failure the
  documentation task exists for, one section up.
- **THE E2E SECTION IS OWED EVEN WHERE IT DOES NOT APPLY.** Then it holds ONE
  task stating why — "nothing here is decided by a cluster" — and it is ticked.
  An absent section and a forgotten one look identical; a stated "not
  applicable" is a claim a reviewer can dispute.
- **`docs/testing.md` owns what each tier can decide**, which is what settles
  whether e2e applies. Rendering, parsing and what a reconciler writes are unit
  and envtest; what the kubelet or the authorizer does is e2e.

### ENFORCED WHERE THE DOCUMENTATION TASK IS, BY THE SAME SCRIPT

`.github/scripts/docs-task-guard.py` judges all three trailing sections —
present, in order, ticked — and both the `PreToolUse` hook on `openspec
archive` and CI's `docs-task` job call it. `openspec/config.yaml` injects the
shape when a tasks file is written.

- **Judged at FINISH, not at every touch.** The documentation shape is checked
  whatever the phase, because that rule predates every change in the tree. This
  one landed on 2026-08-29 with ten changes in flight, all planned under the old
  shape — a structural check on every touch would have failed each one's next
  pull request for a plan written before the rule existed, and been switched
  off within the week. A change in the old shape is `pending` until it claims
  to be finished, and then it owes the sections like any other.
- **An open test task is WORK, not a missing gate.** A change whose tests are
  unticked has not claimed completion, so CI reports it `pending`. Only a
  change with its work done and a test section missing, out of order or
  unticked FAILS.
