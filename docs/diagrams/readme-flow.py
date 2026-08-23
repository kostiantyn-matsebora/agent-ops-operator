#!/usr/bin/env python3
"""Draw the README's flow diagram, one file per theme.

WHY THIS EXISTS, AND WHY IT IS NOT MERMAID OR `export.py`:

- MERMAID was tried and removed. It ignores `direction` on a subgraph whose
  edges cross the boundary, so four source nodes stacked into a full page of
  scrolling on GitHub, and it clipped an edge label to its first letter. It also
  offers no real icons and no custom shapes.
- `agent-ops.drawio` / `export.py` draws the PAGE-SCALE picture, 1778x1349.
  A forge column shrinks that past legibility. This one is composed for a README
  column and nothing else.

ONE SCRIPT, TWO FILES, so the halves cannot drift. Palette values are copied
from `docs/assets/css/agentops.css`, which copies them from the console theme —
the same one-directional copy that file documents.

    python3 docs/diagrams/readme-flow.py
"""
import pathlib

OUT = pathlib.Path(__file__).resolve().parent.parent / "assets" / "img"

THEMES = {
    "light": dict(
        chip="#f3f5f6", chipEdge="#cfd6da", ink="#16191c", subtle="#5c656d",
        brand="#0d7d76", brandSoft="#d3ece9", brandInk="#0a615c",
        accent="#6b4bd6", accentSoft="#e9e3fb", accentInk="#4f35a8",
        edge="#9aa4ab", band="#eef1f2",
    ),
    "dark": dict(
        chip="#23282c", chipEdge="#3a4247", ink="#e8ecee", subtle="#a5b0b7",
        brand="#43c9bd", brandSoft="#123b39", brandInk="#7fded5",
        accent="#a996f5", accentSoft="#2a2350", accentInk="#a996f5",
        edge="#5c666d", band="#1b1f22",
    ),
}

W, H = 800, 392

ICONS = {
    # 16x16 glyphs, stroked in currentColor. Drawn rather than emoji: an emoji
    # renders in the reader's font and lands at a different weight per platform.
    "bell": "M8 2.2a3.2 3.2 0 0 0-3.2 3.2v2.8L3.2 10.4h9.6L11.2 8.2V5.4A3.2 3.2 0 0 0 8 2.2z M6.4 12.2a1.7 1.7 0 0 0 3.2 0",
    "box": "M8 1.9 13.7 4.8v6.4L8 14.1 2.3 11.2V4.8z M2.3 4.8 8 7.7l5.7-2.9 M8 7.7v6.4",
    "clock": "M8 2.4a5.6 5.6 0 1 0 0 11.2 5.6 5.6 0 0 0 0-11.2z M8 5.2V8l2.2 1.3",
    "chat": "M2.4 3.6h11.2v7.2H7.6l-3.4 2.8v-2.8H2.4z",
}

SOURCES = [
    ("bell",  "an alert fires",       "Alertmanager"),
    ("box",   "a pod crashloops",     "cluster events"),
    ("clock", "a schedule comes due", "cron"),
    ("chat",  "someone asks",         "chat"),
]


def esc(s):
    return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def icon(name, x, y, colour):
    return (f'<g transform="translate({x},{y})" fill="none" stroke="{colour}" '
            f'stroke-width="1.35" stroke-linecap="round" stroke-linejoin="round">'
            f'<path d="{ICONS[name]}"/></g>')


def text(x, y, s, colour, size=13, weight=400, anchor="start", spacing=0):
    ls = f' letter-spacing="{spacing}"' if spacing else ""
    return (f'<text x="{x}" y="{y}" fill="{colour}" font-size="{size}" '
            f'font-weight="{weight}" text-anchor="{anchor}"{ls}>{esc(s)}</text>')


def card(x, y, w, h, fill, stroke, r=10, sw=1):
    return (f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{r}" '
            f'fill="{fill}" stroke="{stroke}" stroke-width="{sw}"/>')


def band_label(x, y, s, c):
    return text(x, y, s, c, size=10.5, weight=700, spacing="1.4")


def build(t):
    c = THEMES[t]
    o = []
    o.append(f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" '
             f'width="{W}" height="{H}" role="img" font-family="system-ui,-apple-system,'
             f'Segoe UI,Helvetica,Arial,sans-serif">')
    o.append('<defs>'
             f'<marker id="a" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6.5" '
             f'markerHeight="6.5" orient="auto-start-reverse">'
             f'<path d="M0 0 10 5 0 10z" fill="{c["edge"]}"/></marker>'
             f'<marker id="ab" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6.5" '
             f'markerHeight="6.5" orient="auto-start-reverse">'
             f'<path d="M0 0 10 5 0 10z" fill="{c["brand"]}"/></marker>'
             f'<marker id="ac" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6.5" '
             f'markerHeight="6.5" orient="auto-start-reverse">'
             f'<path d="M0 0 10 5 0 10z" fill="{c["accent"]}"/></marker>'
             '</defs>')

    # ---- column 1: sources ----------------------------------------------
    o.append(band_label(18, 30, "SOMETHING HAPPENS", c["subtle"]))
    ys = [44, 100, 156, 212]
    for (ic, label, sub), y in zip(SOURCES, ys):
        o.append(card(16, y, 196, 46, c["chip"], c["chipEdge"]))
        o.append(icon(ic, 30, y + 15, c["subtle"]))
        o.append(text(56, y + 21, label, c["ink"], 13, 600))
        o.append(text(56, y + 36, sub, c["subtle"], 10.5))
        o.append(f'<path d="M212 {y+23} H224" stroke="{c["edge"]}" stroke-width="1.3" fill="none"/>')
    # A manifold: four stubs onto one spine, then ONE arrow into the card. The
    # arrow needs a RUN — at six pixels its head floated in the gap, detached
    # from both the spine and the card, and read as a rendering fault.
    o.append(f'<path d="M224 67 V235" stroke="{c["edge"]}" stroke-width="1.3" fill="none"/>')
    o.append(f'<path d="M224 151 H248" stroke="{c["edge"]}" stroke-width="1.6" fill="none" marker-end="url(#a)"/>')

    # ---- column 2: the Pipeline -----------------------------------------
    o.append(band_label(252, 30, "YOU DECLARE IT — ONE PIPELINE", c["subtle"]))
    o.append(card(252, 44, 252, 214, c["brandSoft"], c["brand"], r=12, sw=1.6))
    o.append(text(270, 72, "Pipeline", c["brandInk"], 15, 700))
    o.append(text(270, 90, "the only place wiring lives", c["subtle"], 10.5))
    rows = [("what starts it", "signalSourceRefs"),
            ("what it should do", "profileRef"),
            ("what it may touch", "toolsets · mcpConfigs"),
            ("where it answers", "channelRefs")]
    ry = 116
    for label, field in rows:
        o.append(f'<circle cx="274" cy="{ry-4}" r="2.6" fill="{c["brand"]}"/>')
        o.append(text(286, ry, label, c["ink"], 12, 600))
        o.append(text(286, ry + 14, field, c["subtle"], 10, weight=400))
        ry += 35
    o.append(f'<path d="M504 151 H542" stroke="{c["brand"]}" stroke-width="2" fill="none" marker-end="url(#ab)"/>')

    # ---- column 3: runs, then answers, STACKED --------------------------
    # Stacked rather than a fourth column: four bands made the drawing wider
    # than a forge column, and the reply loop reads better as a short hop up
    # one side than as a long run back across the whole picture.
    o.append(band_label(548, 30, "THE OPERATOR RUNS IT", c["subtle"]))
    o.append(card(548, 44, 196, 92, c["accentSoft"], c["accent"], r=12, sw=1.4))
    o.append(text(566, 72, "Conversation", c["ink"], 14, 700))
    o.append(text(566, 91, "one per incident, resumable,", c["subtle"], 10.5))
    o.append(text(566, 105, "its own thread", c["subtle"], 10.5))

    o.append(f'<path d="M646 136 V152" stroke="{c["accent"]}" stroke-width="1.6" fill="none" marker-end="url(#ac)"/>')
    o.append(card(548, 152, 196, 100, c["chip"], c["accent"], r=12, sw=1.4))
    o.append(text(566, 180, "its own agent pod", c["ink"], 13, 700))
    o.append(text(566, 199, "isolated · serial · capped", c["subtle"], 10.5))
    o.append(text(566, 217, "investigates, explains, acts", c["subtle"], 10.5))
    o.append(text(566, 231, "ONLY where your wiring", c["subtle"], 10.5))
    o.append(text(566, 245, "granted it", c["subtle"], 10.5))

    o.append(f'<path d="M646 252 V268" stroke="{c["brand"]}" stroke-width="2" fill="none" marker-end="url(#ab)"/>')
    o.append(card(548, 268, 196, 88, c["brandSoft"], c["brand"], r=12, sw=1.4))
    o.append(text(566, 296, "your channels", c["ink"], 13, 700))
    o.append(text(566, 315, "Telegram, the console,", c["subtle"], 10.5))
    o.append(text(566, 329, "your own adapter", c["subtle"], 10.5))

    # The reply returns to the CONVERSATION, not to the pod: a pod is
    # provisioned per unit of work and may not exist when the reply lands.
    o.append(f'<path d="M744 312 H772 V90 H748" stroke="{c["edge"]}" '
             f'stroke-width="1.3" fill="none" stroke-dasharray="4 4" marker-end="url(#a)"/>')
    o.append(text(548, 378, "you reply, and the SAME conversation continues", c["subtle"], 10.5))
    o.append('</svg>')
    return "\n".join(o)


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    for t in THEMES:
        p = OUT / f"readme-flow-{t}.svg"
        p.write_text(build(t) + "\n", encoding="utf-8")
        print(f"wrote {p.relative_to(OUT.parent.parent.parent)}  ({p.stat().st_size} bytes)")


if __name__ == "__main__":
    main()
