## The conveyor engine (a standalone, unwired tool)

**`tools/conveyor-engine/` IS A NEW TOP-LEVEL DIRECTORY, AND IT IS NOT A
COMPONENT.** It carries no `Dockerfile` and no `go.mod`, so
`.github/components.sh` never discovers it — the same carve-out
`structure.md` gives `test/`, for the same reason: this repository's own
delivery tooling, not something it ships.

It is the generic engine that runs any workflow
`.github/conveyor-model/workflows.desired.yaml` declares, reading the same
file's sibling `labels.yaml` for which real GitHub label implements which
state or trigger. No workflow's shape is hardcoded in it anywhere.

### IT IS NOT LIVE. DO NOT ASSUME OTHERWISE

- **No real caller in `.github/scripts/` calls it.** `conveyor.py` is the
  production decision mechanism, untouched, and keeps running exactly as
  it did before this engine existed.
- **No workflow in `.github/workflows/` invokes it.** `claude-review.yml`'s
  `review` workflow declaration sits in the YAML model unused.
- **It depends on nothing under `.github/scripts/`** — not `conveyor.py`,
  not `conveyor_io.py`, not `load_script.py` — and nothing under
  `.github/` depends on it. The only thing it reads from `.github/` is the
  two model files, as data.
- **It is proven correct through its own test suite alone**
  (`python3 -m unittest discover -s tools/conveyor-engine`), against the
  real YAML, with no network and no `gh` call in any test.

**A FOLLOW-UP CHANGE DOES THE WIRING, DELIBERATELY DEFERRED.** Rewiring the
real callers (`carry.py`, `remote-implement.py`, `dispatch-gate.py`,
`land-dispatch.py`, `failed-checks.py`, `autofix-guard.py`,
`recover-loop-state.py`, `refresh-loop-state.py`) onto this engine, and
wiring `claude-review.yml` for the `review` workflow, was scoped OUT of the
`conveyor-engine` openspec change that built this package.

Until that follow-up lands, this engine existing changes nothing about how
the conveyor actually runs.
