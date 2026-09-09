## Why

`README.md`'s "Try it" install command pulls the chart with no `--version`, so
a reader who copies it gets whatever the OCI registry currently tags `latest`
as, rather than a pinned, reproducible install — unlike `docs/installation.md`,
which already pins `--version 13.4.0`. `docs-generate.py --check` already
walks `README.md` looking for a `--version X.Y.Z` pattern to hold against the
chart's own version (`check_versions()`, added by
`docs/CLAUDE.md`'s release-docs rule), but has had nothing to check there
since the command never printed one.

## What Changes

- **`README.md`'s install command pins `--version 13.4.0`** (the chart's
  current version), matching the form `docs/installation.md` already uses.
- **No behavior change.** This is a documentation-only edit — no CRD field,
  HTTP contract, CLI flag or chart value changes. `skip_specs` is set on this
  change for that reason.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

_None — no spec describes README's install command, so no spec-level
requirement changes._

## Impact

- `README.md` — the one line changes.
- `docs/concepts.md`, `docs/contracts.md` — untouched; no CRD field,
  semantics or contract changed.
- Adopter site (landing page, `introduction.md`, `getting-started.md`,
  `installation.md`, `docs/guides/*`) — untouched; none of them describe
  README's own install command, and none of them printed a stale number
  before this change.
- `python3 .github/scripts/docs-generate.py --check` — already enforces the
  new line: `check_versions()` scans `README.md` for `--version X.Y.Z` and
  fails if it drifts from `chart/Chart.yaml`'s version. This change gives
  that existing check something to hold in README for the first time.
