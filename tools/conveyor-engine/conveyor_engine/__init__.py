"""The generic conveyor engine.

STANDALONE AND UNWIRED. This package runs any workflow declared in
`.github/conveyor-model/workflows.desired.yaml`, against the real labels
`.github/conveyor-model/labels.yaml` names, with no workflow-specific code
anywhere in it. Nothing under `.github/scripts/` or `.github/workflows/`
imports it, and it imports nothing from `.github/scripts/` -- see
`openspec/changes/conveyor-engine/design.md` for why, and for the follow-up
change that wires a real caller onto it.

Path resolution. Every loader in this package accepts an optional
`repo_root: pathlib.Path`. Left unset, it is derived by walking up from
this package's own location (see `paths.default_repo_root`) until a
directory containing `.github/conveyor-model/workflows.desired.yaml` is
found -- so the package works whether it is invoked from the repository
root, from inside `tools/conveyor-engine/`, or imported from anywhere else
on `sys.path`.
"""
