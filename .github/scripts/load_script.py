"""Import a hyphenated script file as a module, by path.

`.github/scripts/*.py` files are invoked as CLIs, so most are named with
hyphens -- not valid Python identifiers, so a plain `import` cannot reach
them. `review-prompt.py` and `review-context.py` each need another one's
functions and each carried the same four-line `importlib.util` dance to get
them; this is that dance, once.
"""
from __future__ import annotations

import importlib.util
import pathlib
import types

HERE = pathlib.Path(__file__).resolve().parent


def load(name: str) -> types.ModuleType:
    """`name` without `.py`, e.g. `"review-rules"` for `review-rules.py` in
    this same directory."""
    path = HERE / f"{name}.py"
    spec = importlib.util.spec_from_file_location(name.replace("-", "_"), path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod
