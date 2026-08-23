#!/usr/bin/env python3
"""Export agent-ops.drawio to the SVG variants the site serves.

Run from the repository root:  python docs/diagrams/export.py

Which pages
-----------
The file holds two. `why` is the standalone poster, which carries its own
eyebrow pills, headline and standfirst because nothing around it does. It is
NOT exported here — a render for a slide or a post is made at that moment and
not committed.

The one the site serves is:

* `site` — the whole argument, behind the landing page's full-size link.

There WAS a third, `landing`, compressed to 960px for the landing page's own
strip. It is gone, and so is its export. The landing page states the model with
a presentation now — the same argument built one beat at a time, in real text —
and a still restating it would be the same claim twice, once in a form nobody
can select, translate or search. Every attempt to add detail to that still had
to remove an element to pay for it, and that ceiling is what the presentation
removes.

`site` has the poster's masthead removed: the page states the headline in real,
selectable, translatable text, and a page that says it twice reads as a mistake.

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

The scratch directory is made BESIDE THIS FILE, never in /tmp
--------------------------------------------------------------
A VM-backed daemon — Rancher Desktop, Docker Desktop — mounts the user's home
and not `/tmp`, so a `/tmp` workdir bind-mounts as an EMPTY directory. The
exporter then finds no drawing, writes nothing, and says nothing: the run looks
like a docker fault rather than a mount one. Keeping the scratch beside the
source keeps it on a path any such daemon can already see.

Requires: docker, python 3.
"""

import base64
import contextlib
import pathlib
import re
import shutil
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent
SOURCE = HERE / "agent-ops.drawio"
OUT = HERE.parent / "assets" / "img"
IMAGE = "rlespinasse/drawio-export:latest"

# page name -> the basename its variants are written under in assets/img/.
PAGES = {
    "site": "agent-ops",
}

ICON_INK_LIGHT = b"#1A1A1A"
ICON_INK_DARK = b"#D7D7D7"


@contextlib.contextmanager
def scratch() -> pathlib.Path:
    """A workdir beside the source, removed however this exits."""
    work = HERE / ".export-work"
    shutil.rmtree(work, ignore_errors=True)
    work.mkdir()
    try:
        yield work
    finally:
        shutil.rmtree(work, ignore_errors=True)


def export(workdir: pathlib.Path, theme: str) -> None:
    """Run the exporter in a container. It writes every page in one pass."""
    subprocess.run(
        [
            "docker", "run", "--rm",
            "-v", f"{workdir}:/data",
            IMAGE,
            "-f", "svg", "--svg-theme", theme, "-o", f"export-{theme}",
        ],
        check=True,
    )


def produced(workdir: pathlib.Path, theme: str, page: str) -> pathlib.Path:
    """Return the one SVG the exporter wrote for `page`, or fail saying so.

    The check is per page rather than per run: a rename in the drawing that
    silently dropped one page would otherwise leave a stale committed SVG
    beside a correct one, which is the failure this refuses to ship.
    """
    # The exporter writes one file per page, named "<file>-<page>.svg".
    found = list((workdir / f"export-{theme}").glob(f"*-{page}.svg"))
    if len(found) != 1:
        every = [p.name for p in (workdir / f"export-{theme}").glob("*.svg")]
        sys.exit(f"no single '{page}' page in the {theme} export; found {every}")
    return found[0]


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

    with scratch() as work:
        shutil.copy(SOURCE, work / SOURCE.name)

        export(work, "light")
        export(work, "dark")

        for page, base in PAGES.items():
            light = OUT / f"{base}-light.svg"
            shutil.copy(produced(work, "light", page), light)
            print(f"wrote {light}")

            dark = OUT / f"{base}-dark.svg"
            patched, repainted = repaint_icons(produced(work, "dark", page).read_bytes())
            if repainted == 0:
                print(f"WARNING: no icon ink repainted in '{page}' — "
                      "did the icon colours change?")
            dark.write_bytes(patched)
            print(f"wrote {dark} ({repainted} icons repainted)")


if __name__ == "__main__":
    main()
