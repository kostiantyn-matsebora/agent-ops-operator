## Context

See `proposal.md` — Why. `docs-generate.py`'s `check_versions()` already
treats `README.md` as one of the pages it scans for `--version X.Y.Z`
(`pages = [*DOCS.glob("*.md"), *DOCS.glob("*/*.md"), REPO / "README.md"]`);
the command in README simply carries no such flag yet.

## Goals / Non-Goals

**Goals:**
- README's install command names the chart version it installs, exactly as
  `docs/installation.md` already does.

**Non-Goals:**
- Changing `docs-generate.py` itself — the check this change relies on
  already exists and already covers README.
- Any other README section. `documentation.md`'s 215-line budget is
  unaffected: this is a one-line edit inside an existing code block.

## Decisions

**Add `--version 13.4.0` to the existing `helm install` command** rather than
introducing a second, pinned example beside the current one. `documentation.md`
routes "a first-party image tag a worked example shows" and "the chart
version the install command prints" to `docs/installation.md` and
`docs/concepts.md` for the CANONICAL walkthrough; README's own "Try it"
snippet is a shorter mirror of that same command; giving it its own drifting,
unpinned copy is the inconsistency this change closes, not a case for a new
mechanism.

**No new guard.** `check_versions()` already fails the render on a `--version`
that disagrees with `chart/Chart.yaml`, for any page under `docs/**/*.md`
plus `README.md`. Nothing here is bespoke to README.

## Risks / Trade-offs

**[Risk] The pinned version drifts on the next chart release, exactly as
`docs/installation.md`'s did before `docs-generate.py --check` existed
(`documentation.md`: chart 13.1.0 shipped while the site still said 13.0.1)**
→ Mitigation: none needed beyond what already exists — `check_versions()`
now also covers this new line, and CI's `docs-generate` check already fails
the build on a stale number anywhere it scans, README included.

## Migration Plan

None. Single-line documentation edit, no runtime behavior, no deploy step.
