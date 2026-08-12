#!/usr/bin/env python3
"""Export agent-ops.drawio to the two SVG variants the site serves.

Run from the repository root:  python docs/diagrams/export.py

Which page
----------
The file holds two: `why` is the standalone poster, which carries its own
eyebrow pills, headline and standfirst because nothing around it does. `site`
is the same drawing with that masthead removed — the landing page states those
in real, selectable, translatable text, and a page that says the headline twice
reads as a mistake. Only `site` is exported here.

Why a script rather than two docker commands in a comment
---------------------------------------------------------
drawio bakes a `light-dark()` pair into every fill it OWNS and pins the
resolution with `color-scheme` on the SVG root, so `--svg-theme light|dark`
really does produce two palettes of one drawing. It cannot recolour an
EMBEDDED IMAGE, though, and the diagram's icons are `shape=image` cells
carrying their own SVG. Their ink is hard-coded `#1A1A1A`, so in the dark
export they come out near-black on a near-black ground — present, and
invisible.

The fix is one substitution: in the DARK variant only, repaint that ink to the
same light grey drawio maps text to (`#D7D7D7`). The accent orange is left
alone; it reads on both grounds. Doing this by hand after every export is the
kind of step that gets forgotten once and ships a broken diagram, which is why
it lives here.

Requires: docker, python 3.
"""

import base64
import pathlib
import re
import shutil
import subprocess
import sys
import tempfile

HERE = pathlib.Path(__file__).resolve().parent
SOURCE = HERE / "agent-ops.drawio"
OUT = HERE.parent / "assets" / "img"
IMAGE = "rlespinasse/drawio-export:latest"
PAGE = "site"

ICON_INK_LIGHT = b"#1A1A1A"
ICON_INK_DARK = b"#D7D7D7"


def export(workdir: pathlib.Path, theme: str) -> pathlib.Path:
    """Run the exporter in a container and return the generated SVG."""
    subprocess.run(
        [
            "docker", "run", "--rm",
            "-v", f"{workdir}:/data",
            IMAGE,
            "-f", "svg", "--svg-theme", theme, "-o", f"export-{theme}",
        ],
        check=True,
    )
    # The exporter writes one file per page, named "<file>-<page>.svg".
    produced = list((workdir / f"export-{theme}").glob(f"*-{PAGE}.svg"))
    if len(produced) != 1:
        found = [p.name for p in (workdir / f"export-{theme}").glob("*.svg")]
        sys.exit(f"no single '{PAGE}' page in the {theme} export; found {found}")
    return produced[0]


def repaint_icons(svg: bytes) -> tuple[bytes, int]:
    """Repaint the embedded icons' ink for the dark ground.

    Each icon is a base64 data URI, so the colour cannot be reached with a
    plain substitution over the file — decode, swap, re-encode.
    """
    count = 0

    def swap(match: re.Match) -> bytes:
        nonlocal count
        payload = base64.b64decode(match.group(1))
        if ICON_INK_LIGHT not in payload:
            return match.group(0)
        count += 1
        payload = payload.replace(ICON_INK_LIGHT, ICON_INK_DARK)
        return b'"data:image/svg+xml;base64,' + base64.b64encode(payload) + b'"'

    return re.sub(rb'"data:image/svg\+xml;base64,([A-Za-z0-9+/=]+)"', swap, svg), count


def main() -> None:
    if not SOURCE.exists():
        sys.exit(f"no diagram at {SOURCE}")
    OUT.mkdir(parents=True, exist_ok=True)

    with tempfile.TemporaryDirectory() as tmp:
        work = pathlib.Path(tmp)
        shutil.copy(SOURCE, work / SOURCE.name)

        light = export(work, "light")
        shutil.copy(light, OUT / "agent-ops-light.svg")
        print(f"wrote {OUT / 'agent-ops-light.svg'}")

        dark = export(work, "dark")
        patched, repainted = repaint_icons(dark.read_bytes())
        if repainted == 0:
            print("WARNING: no icon ink repainted — did the icon colours change?")
        (OUT / "agent-ops-dark.svg").write_bytes(patched)
        print(f"wrote {OUT / 'agent-ops-dark.svg'} ({repainted} icons repainted)")


if __name__ == "__main__":
    main()
