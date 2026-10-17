## Context

See proposal.md — Why. The current state that shapes the approach:

- `docs/contracts.md` holds three extension-point contracts plus five
  manager-owned surfaces in one 1540-line reference page.
- The work-contract section is narrative. The work unit's `threadId`,
  `resumeSessionId`, `systemPrompt` and `maxTurns` fields appear in no
  document. The `status` enum is enumerated nowhere. The environment contract
  exists only in `internal/runtimepod/` source.
- `docs/guides/agent-runtime.md` states the manager checks the repository out.
  Every shipped runtime clones it itself from `REPO_URL` and `GIT_*`.
- Both sides decode with plain `json.Unmarshal`, so unknown fields are already
  ignored. Nothing states that as a rule.
- `docs-generate.py` already owns the generated-block pattern: a marker in the
  page, CI fails on a difference.

## Goals / Non-Goals

**Goals:**

- A runtime built from the specification alone works on the first attempt.
- The specification cannot rot silently — schema tables are generated, drift
  fails CI.
- The agent working in this repository is bound to the specification the same
  way an adopter is.

**Non-Goals:**

- No manager, runtime or chart behavior changes. The spec documents what is.
- No wire version negotiation. The mechanism is reserved in prose only.
- No conformance harness. The checklist ships, the kit is a named follow-up.
- Channel and signal adapter contracts move verbatim, unimproved. Each gets
  its own change to this standard.

## Decisions

### One page per extension point, under `docs/contracts/`

A runtime author, a channel author and a signal author are three audiences.
None needs the others' contract in context.

- `docs/contracts.md` keeps its URL and the manager-owned surfaces (aops MCP
  server, activity, introspection, metrics, HTTP API).
- Each moved section leaves a one-line pointer at its old anchor.

Alternative rejected: expanding the work-contract section in place. The spec's
scope (two parties, lifecycle, environment, conformance) outgrows a section,
and the page would serve nobody well.

### Two documents, two genres, one-directional references

The specification is law — a reference page, no front matter, read on GitHub.
The guide is procedure — a site page of ordered stations, each citing a spec
section by anchor and ending in a verification.

The guide states no requirement of its own. The spec never depends on the
guide.

Anchors are referenced by NAME, never by section number, so renumbering is not
a breaking change but renaming an anchor is.

Alternative rejected: one combined document. Law read linearly buries the
path, procedure interleaved with requirements hides the requirements.

### Document versioning now, wire negotiation reserved

The spec carries its own semver, independent of chart and API group, with a
changelog section at its foot:

| Bump | Means |
|---|---|
| MAJOR | a conformant runtime stops conforming |
| MINOR | a new optional capability or optional field |
| PATCH | clarification, no behavior change |

A MAJOR bump also lands upgrade steps in `docs/CHANGELOG.md`, per the existing
rule. One sentence reserves a future version parameter on `GET /work`, so
adding negotiation later is MINOR.

Alternative rejected: a version query parameter now. Pre-1.0 with a
provisional API group it is machinery without a customer.

### Ignore-unknown-fields becomes normative

Verified: `json.Unmarshal` on both sides, no `DisallowUnknownFields`. Stating
it as a requirement is what makes MINOR additive changes safe, and what stops
a future strict decoder landing as an accident.

### Schema tables generated from the Go structs

`docs-generate.py` gains a `struct-table` marker: given a Go file and struct
name, it renders a table row per json-tagged field.

Semantics stay hand-written prose around the block. The generator reads json
tags and doc comments only — no type checking, fail loudly on a struct it
cannot parse.

Sources of truth: the work unit struct in
`platform/manager/internal/dispatch/dispatch.go`, the report struct in
`platform/manager/internal/httpapi/`.

Alternative rejected: hand-written tables. The drift this change exists to fix
would return with the next field.

### Governance is a scoped rule, enforced socially plus one hook surface

A new `.claude/rules/runtime-spec.md` with `paths:` scoping to `runtimes/**`,
the manager's dispatch, runtimepod and httpapi packages, and `docs/contracts/runtime.md`.
It states three obligations:

1. A change on the governed surface is verified against the specification
   before it is claimed done.
2. The specification is never edited without the user's explicit approval.
3. An approved specification change brings every shipped runtime into
   conformance in the same change, and moves every vendor page's claimed
   version. The change is not archivable before that.

Alternative rejected: a PreToolUse hook refusing edits to the spec file.
Approval is a conversation fact a hook cannot read. The rule plus review is
the honest mechanism, and the docs-task guard already gates archiving.

### The spec starts at 1.0.0 and documents current behavior

The known fixes (the checkout claim, the missing fields, the enums) are
corrections of documents, not changes of behavior. Nothing shipped needs to
change to conform, so the three vendor pages claim 1.0.0 at birth.

The claude runtime's page is `docs/claude.md`, not under `docs/runtimes/`. It
gets its conformance line where it is — moving it is out of scope, and the
delta spec binds "each shipped runtime's documentation page" rather than a
path.

Two nuances the spec document states explicitly:

- `status` is `succeeded` or `failed`, and the manager reads any other value
  as failed.
- `REPO_URL` and `REPO_REF` are always set and may be empty. An empty
  `REPO_URL` means no repository, so no clone.

## Risks / Trade-offs

- [Moved anchors break bookmarks and inbound links] → each moved section
  leaves a pointer line at its old heading in `contracts.md`, and the link
  check in the documentation-structure spec covers every renamed target.
- [The struct-table generator misparses Go source] → scope is json tags and
  doc comments of named structs only, it fails the run loudly rather than
  rendering a partial table, and the rendered tables are reviewed in this
  change's PR against the structs.
- [The spec states a behavior wrong, as the guide did] → every normative
  sentence is traced to source or a test during authoring, and the conformance
  checklist cites the existing runtime test files per item.
- [Governance rule is prose, not enforcement] → accepted. The verify-and-
  approve obligations are review facts. The archivability half rides the
  existing docs-task guard.

## Migration Plan

Documentation only. No deploy step, no rollback beyond reverting the PR.
External links to moved sections keep resolving through the pointer lines.
