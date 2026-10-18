## Why

`sonar-ratings-baseline` fixed 47 SonarCloud findings toward `manager`'s
quality gate but deliberately left 26 `go:S3776` (cognitive complexity)
findings open in `manager`'s PRODUCTION code — its own task 2.1 states the
reason: "the hour, and the risk of reshaping the core reconciler
unsupervised." That change is archiving now with those findings still open,
task 2.2 (confirming the branch-wide backlog reads clean) blocked on them,
and this proposal is what carries that named, deferred scope forward as its
own change rather than letting it evaporate once the parent archives.

## What Changes

- Extract the control-flow-heavy body of each of the 26 flagged functions
  into genuinely separate, named top-level helper functions — the same
  technique `sonar-ratings-baseline` proved on 35 test-file findings and
  recorded in `.claude/rules/gotchas.md`: a nested closure does NOT lower
  `go:S3776`'s score (the nesting is charged to the enclosing function
  regardless), only a real function boundary does.
- Every extraction preserves behavior exactly — these are hot-path
  reconciler, dispatch and httpapi functions — verified by the existing
  test suite, with new unit tests added wherever a function's current
  behavior isn't already covered by one.
- Re-run the finding count for `manager` (`sonar-findings-baseline.py` or
  the live SonarCloud read) after the fixes land, and record honestly
  whether `go:S3776` reads zero for this component or what's left, in the
  same style `sonar-ratings-baseline`'s own tasks.md already uses.

**Not in scope:** `sonar-ratings-baseline`'s task 1.2 — the full org-wide
finding enumeration across all ~16 components beyond `manager` — stays
deferred, noted in that change's own archived record, not carried here.

## Capabilities

### New Capabilities

(none — see Impact)

### Modified Capabilities

(none)

## Impact

- `platform/manager/internal/controller/conversation_controller.go` (7
  findings, one at complexity 61) and `pipeline_controller.go` (1)
- `platform/manager/internal/httpapi/{server,signals,channelread,status}.go`
  (9)
- `platform/manager/internal/chat/{ops,delivery,pipelines}.go` (4)
- `platform/manager/internal/runtimepod/podspec.go` (2)
- `platform/manager/internal/dispatch/dispatch.go` (1)
- `platform/manager/internal/mcpcompile/compile.go` (1)
- `platform/manager/internal/activity/log.go` (1)
- No CRD field, contract, chart value, or adopter-visible behavior changes —
  this is an internal refactor for a lint/quality metric, not a behavior
  change. No reference doc or adopter-site page describes internal function
  shape, so neither needs an update; `docs/CLAUDE.md`'s routing table has no
  row for this. (Verify at implementation time that no doc actually
  references one of these functions by name or shape before closing the
  documentation task as not-applicable.)
