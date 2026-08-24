#!/usr/bin/env python3
"""Draw the security page's threat model, one file per theme.

WHY THIS EXISTS AND WHY IT IS NOT `docs_diagrams.py`:

That renderer draws COLUMNS OF BOXES joined by arrows, which is the right shape
for a guide — here is the object you write, here is what it names. A threat
model is a different picture: what it has to show is TRUST BOUNDARIES and the
flows that CROSS them, because a threat is a crossing and a control is what sits
on it. A column renderer cannot draw a region, so this one draws its own.

ONE SCRIPT, TWO FILES, so the halves cannot drift — the same reason
`readme-flow.py` gives. Palette values are copied from
`docs/assets/css/agentops.css`, which copies them from the console theme.

THE UNMITIGATED FLOW IS DRAWN, NOT OMITTED. Flow 6 has no control, and a threat
model that showed only the mitigated crossings would be the diagram equivalent
of a security page listing only what is handled.

    python3 docs/diagrams/threat-model.py
"""
import pathlib

OUT = pathlib.Path(__file__).resolve().parent.parent / "assets" / "img" / "security"

THEMES = {
    "light": dict(
        band="#eef1f2", surface="#ffffff", surfaceAlt="#f3f5f6",
        border="#cfd6da", borderStrong="#9aa4ab",
        ink="#16191c", subtle="#5c656d",
        brand="#0d7d76", brandSoft="#d3ece9", brandInk="#0a615c",
        accent="#6b4bd6", accentSoft="#e9e3fb",
        danger="#b32c3f", dangerSoft="#f7e3e6",
    ),
    "dark": dict(
        band="#1b1f22", surface="#1b1f22", surfaceAlt="#23282c",
        border="#3a4247", borderStrong="#5c666d",
        ink="#e8ecee", subtle="#a5b0b7",
        brand="#43c9bd", brandSoft="#123b39", brandInk="#7fded5",
        accent="#a996f5", accentSoft="#2a2350",
        danger="#f2708a", dangerSoft="#3d2028",
    ),
}

# LANDSCAPE, AND AUTHORED NEAR THE FRAME'S OWN WIDTH.
#
# `.ao-diagram` is `min-width: 42rem` (672px) inside a content column that is
# 616px at 1280 and 720px above 1440, so a drawing is rendered somewhere in that
# band whatever its canvas says. Two failures were built before this shape:
#
#   980 wide - scaled to 0.69, and every label went unreadable.
#   672 x 872 - readable, but a portrait drawing that filled the whole screen.
#
# 760 wide renders at 0.88-0.95, so type is near its authored size and the whole
# picture is still one landscape glance.
W, H = 760, 480
FONT = ("ui-sans-serif, system-ui, -apple-system, 'Segoe UI', Roboto, "
        "'Helvetica Neue', Arial, sans-serif")


def esc(s):
    return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def text(x, y, s, fill, size=13, weight=400, anchor="start"):
    return (f'<text x="{x}" y="{y}" fill="{fill}" font-size="{size}" '
            f'font-weight="{weight}" text-anchor="{anchor}" '
            f'font-family="{FONT}">{esc(s)}</text>')


def boundary(x, y, w, h, label, p, tone="borderStrong"):
    """A trust boundary: dashed, labelled on its own top edge.

    Dashed because that is the convention every threat model uses, and the
    label sits ON the edge so it reads as the boundary's name rather than as a
    heading for whatever happens to be nearest it.
    """
    return (
        f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="10" '
        f'fill="none" stroke="{p[tone]}" stroke-width="1.5" '
        f'stroke-dasharray="7 5"/>'
        f'<rect x="{x + 14}" y="{y - 10}" width="{len(label) * 7 + 18}" '
        f'height="20" rx="5" fill="{p["band"]}"/>'
        + text(x + 22, y + 5, label, p[tone], size=13, weight=600)
    )


def node(x, y, w, title, sub, p, kind="plain"):
    fill, stroke, ink = p["surfaceAlt"], p["border"], p["ink"]
    if kind == "untrusted":
        fill, stroke, ink = p["dangerSoft"], p["danger"], p["ink"]
    if kind == "control":
        fill, stroke, ink = p["brandSoft"], p["brand"], p["ink"]
    h = 48 if sub else 34
    out = (f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="8" '
           f'fill="{fill}" stroke="{stroke}" stroke-width="1.4"/>')
    out += text(x + 13, y + (21 if sub else 25), title, ink, size=16, weight=600)
    if sub:
        out += text(x + 13, y + 39, sub, p["subtle"], size=13)
    return out


def badge(x, y, n, p, tone="accent"):
    """The numbered crossing. The number is the join to the threat table."""
    return (f'<circle cx="{x}" cy="{y}" r="11" fill="{p[tone]}"/>'
            + text(x, y + 5, str(n), p["band"], size=13, weight=700,
                   anchor="middle"))


def arrow(x1, y1, x2, y2, p, tone="borderStrong", dashed=False):
    dash = ' stroke-dasharray="5 4"' if dashed else ""
    return (f'<path d="M{x1},{y1} L{x2},{y2}" stroke="{p[tone]}" '
            f'stroke-width="1.6" fill="none" marker-end="url(#ar-{tone})"{dash}/>')


def render(p):
    s = [f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" '
         f'width="{W}" height="{H}" role="img">']
    s.append(f'<rect width="{W}" height="{H}" fill="{p["band"]}"/>')
    s.append('<defs>')
    for tone in ("borderStrong", "danger", "brand"):
        s.append(f'<marker id="ar-{tone}" viewBox="0 0 10 10" refX="9" refY="5" '
                 f'markerWidth="7" markerHeight="7" orient="auto-start-reverse">'
                 f'<path d="M0,0 L10,5 L0,10 z" fill="{p[tone]}"/></marker>')
    s.append('</defs>')

    # Outside the cluster entirely.
    s.append(node(20, 14, 250, "the model provider", "an external API", p))
    s.append(node(282, 14, 300, "chat and signal transports", "outside your control", p))

    # The cluster, and a neighbour workload inside it. The neighbour is on the
    # picture because crossing 1 is about what ANY pod can reach.
    s.append(boundary(12, 84, 736, 364, "your cluster", p))
    s.append(node(520, 100, 214, "any other pod", "in the cluster", p))

    # The runtime pod — where model output actually executes — beside the
    # operator's own namespace, because crossing 2 goes between them.
    s.append(boundary(28, 172, 340, 182, "runtime pod", p, tone="danger"))
    s.append(node(42, 190, 312, "the agent container", "runs the model's output", p,
                  kind="untrusted"))
    s.append(node(42, 248, 150, "egress proxy", "on by default", p, kind="control"))
    s.append(node(204, 248, 150, "context-sync", "on by default", p, kind="control"))
    s.append(node(42, 302, 312, "its ServiceAccount", "bound to nothing", p,
                  kind="control"))

    s.append(boundary(408, 172, 332, 182, "operator namespace", p))
    s.append(node(422, 190, 304, "the manager", "unauthenticated", p))
    s.append(node(422, 248, 304, "MCP servers", "accept any caller", p))
    s.append(node(422, 302, 304, "adapters, console", "credentials as env", p))

    # Cluster-scoped things the agent may try to reach.
    for x, t, sub in [
        (28, "Kubernetes API", "every namespace"),
        (206, "Secrets", "via the kubelet"),
        (384, "context volume", "cross-conversation"),
        (562, "pod logs", "the agent's output"),
    ]:
        s.append(node(x, 384, 172, t, sub, p))

    # The crossings. Each number is a row in the page's threat table.
    s.append(arrow(610, 148, 610, 168, p))                   # 1 neighbour -> operator ns
    s.append(badge(634, 158, 1, p))
    s.append(arrow(372, 272, 402, 272, p, tone="brand"))     # 2 agent -> MCP
    s.append(badge(387, 250, 2, p, tone="brand"))
    s.append(arrow(114, 358, 114, 380, p, tone="brand"))     # 3 -> API
    s.append(badge(138, 369, 3, p, tone="brand"))
    s.append(arrow(292, 358, 292, 380, p, tone="brand", dashed=True))  # 4 -> Secrets
    s.append(badge(316, 369, 4, p, tone="brand"))
    s.append(arrow(470, 358, 470, 380, p, tone="brand"))     # 5 -> context
    s.append(badge(494, 369, 5, p, tone="brand"))
    s.append(arrow(648, 358, 648, 380, p, tone="danger"))    # 6 -> logs, UNMITIGATED
    s.append(badge(672, 369, 6, p, tone="danger"))

    s.append(text(20, H - 16,
                  "Green crossings carry a control. Red carries none.",
                  p["subtle"], size=12))
    s.append('</svg>')
    return "\n".join(s)


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    for name, p in THEMES.items():
        path = OUT / f"threat-model-{name}.svg"
        path.write_text(render(p) + "\n")
        print(f"wrote {path.relative_to(OUT.parent.parent.parent)}")


if __name__ == "__main__":
    main()
