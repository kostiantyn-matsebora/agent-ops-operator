#!/usr/bin/env python3
"""Generate the guide diagrams: one drawing per guide, both themes.

Imported by docs-generate.py, which owns the writing and the drift check.

Why generated rather than drawn
-------------------------------
`agent-ops.drawio` is the right home for a drawing that is COMPOSED — the
landing spine is a picture somebody laid out. A guide's opening diagram is not
composed, it is the same primitive seven times: a left-to-right chain of stacks
with a labelled arrow between each pair. Drawing that by hand seven times, twice
per theme, is fourteen files to keep in step.

THE PALETTE IS READ FROM agentops.css, NEVER RESTATED
-----------------------------------------------------
`palette-and-mark.md` allows a standalone asset to state its colours literally,
because an <img> is its own document and inherits no custom properties. It
allows that on the condition that no colour is stated literally ANYWHERE else in
the site — so these read the token block out of the stylesheet instead of
carrying a third copy of it.

Change a token and the drift check fails until the drawings are regenerated,
which is the whole point.
"""
from __future__ import annotations

import pathlib
import re
import xml.sax.saxutils as sax

# --- geometry ----------------------------------------------------------------
# 950 IS THE SITE'S DIAGRAM WIDTH, and it is why three columns is the ceiling.
# The content column is 720 and there is no breakout, so a drawing is displayed
# at about 0.76 of what it is authored at. Type is sized for that scale — a
# fourth column drops it to 0.61, where a 14px label renders at 8.5px.
BOX_W, BOX_H = 190, 72
# THE GAP IS SIZED TO THE LONGEST LABEL, not to the arrow. `signalSourceRefs` is
# 16 monospace characters, so an 80px gap put it straight through the boxes on
# both sides. Every label here is kept under LABEL_MAX characters for the same
# reason, and the assertion below is what stops the next one being longer.
COL_GAP = 170
LABEL_MAX = 21
TITLE_MAX = 18
SUB_MAX = 24
ROW_GAP = 18
PAD = 20
FONT = ("ui-sans-serif, system-ui, -apple-system, 'Segoe UI', "
        "'Helvetica Neue', Arial, sans-serif")
MONO = ("ui-monospace, SFMono-Regular, 'Cascadia Mono', 'Liberation Mono', "
        "monospace")


def palette(css: pathlib.Path) -> tuple[dict[str, str], dict[str, str]]:
    """The light and dark `--ao-*` tokens, from the stylesheet's own blocks.

    The dark block is the one guarded for `prefers-color-scheme`. Its values
    OVERLAY the light ones rather than replacing them, exactly as the cascade
    applies them, so a token defined only once is shared by both.
    """
    # COMMENTS ARE STRIPPED FIRST. A selector capture runs back to the previous
    # brace, so it swallows the comment above the block -- and this stylesheet's
    # header explains `.pf-v6-theme-dark`, which put every LIGHT token in the
    # dark palette and produced a drawing that was dark in both themes.
    text = re.sub(r"/\*.*?\*/", "", css.read_text(), flags=re.S)
    # A selector may span lines (`:root,\n.pf-v6-theme-light {`), so the match
    # is over the whole block rather than anchored to a line start.
    blocks = re.findall(r"([^{}]*)\{([^{}]*?--ao-[^{}]*?)\}", text, re.S)
    light: dict[str, str] = {}
    dark: dict[str, str] = {}
    for selector, body in blocks:
        tokens = dict(re.findall(r"--ao-([a-z0-9-]+)\s*:\s*([^;]+);", body))
        if not tokens:
            continue
        target = dark if "theme-dark" in selector else light
        target.update({k: " ".join(v.split()) for k, v in tokens.items()})
    if not light or not dark:
        raise SystemExit(f"no --ao-* token blocks found in {css}")
    # Dark OVERLAYS light, exactly as the cascade applies it, so a token defined
    # once is shared rather than missing from one variant.
    return light, {**light, **dark}


# --- the drawings ------------------------------------------------------------
# A column is a stack of boxes. `arrows` holds the label between each pair of
# columns, so there is always one fewer than there are columns.
#
# `kind` picks the box's ink: "subject" is the thing the guide is about, "yours"
# is the thing the reader writes, "plain" is context.
DIAGRAMS: dict[str, dict] = {
    "pipeline": {
        "alt": "A SignalSource feeds a Pipeline, and the Pipeline names an "
               "AgentProfile to answer, Channels to answer on, and the toolsets "
               "and MCP servers it may use.",
        "cols": [
            [("SignalSource", "what starts it", "plain")],
            [("Pipeline", "the wiring you write", "yours")],
            [("AgentProfile", "who answers", "plain"),
             ("Channel", "where it answers", "plain"),
             ("MCPToolset", "and MCPConfig", "plain")],
        ],
        "arrows": ["signalSourceRefs", "names"],
    },
    "agent-profile": {
        "alt": "A Pipeline names an AgentProfile, which holds how the agent "
               "behaves, and an AgentRuntime executes it.",
        "cols": [
            [("Pipeline", "sources, channels, tools", "plain")],
            [("AgentProfile", "how it behaves", "yours")],
            [("AgentRuntime", "what executes it", "plain")],
        ],
        "arrows": ["profileRef", "runtimeRef"],
    },
    "agent-from-a-repository": {
        "alt": "An AgentProfile names your git repository, the runtime checks "
               "it out at /data/workspace, and the agent definition is read "
               "from there.",
        "cols": [
            [("AgentProfile", "repository and agent", "yours")],
            [("your git repo", "cloned with a deploy key", "yours")],
            [("/data/workspace", "holds .claude/agents/", "subject")],
        ],
        "arrows": ["repository.url", "checked out at"],
    },
    "toolsets": {
        "alt": "An MCPConfig and an MCPToolset are bound from a Pipeline, and "
               "the runtime composes them with the agent definition's own tools "
               "to produce the allowlist.",
        "cols": [
            [("MCPConfig", "the servers you write", "yours"),
             ("MCPToolset", "the patterns you write", "yours")],
            [("Pipeline", "mcpConfigs \u00b7 toolsets", "plain")],
            [("the allowlist", "composed by the runtime", "subject")],
        ],
        "arrows": ["bound from", "half, per toolsMode"],
    },
    "signal-adapter": {
        "alt": "Your signal adapter watches your transport and posts normalised "
               "signals to the manager, which groups them and opens a "
               "conversation.",
        "cols": [
            [("your transport", "a queue, a webhook", "plain")],
            [("your adapter", "normalises what it sees", "yours")],
            [("the manager", "groups, then opens one", "subject")],
        ],
        "arrows": ["watches", "POST /signal/inbound"],
    },
    "channel-adapter": {
        "alt": "The manager queues outbound operations for your channel adapter, "
               "which renders them onto your chat, and pushes what people type "
               "back to the manager.",
        "cols": [
            [("the manager", "composes meaning", "plain")],
            [("your adapter", "composes presentation", "yours")],
            [("your chat", "where people type", "subject")],
        ],
        "arrows": ["GET /channel/ops", "renders"],
        "back": ["POST /channel/inbound", "people type"],
    },
    "agent-runtime": {
        "alt": "The manager hands a work unit to your runtime inside the "
               "conversation's pod, which runs the agent and reports the result "
               "back.",
        "cols": [
            [("the manager", "one unit at a time", "plain")],
            [("your runtime", "in the conversation pod", "yours")],
            [("the agent", "with its own allowlist", "subject")],
        ],
        "arrows": ["GET /work", "runs"],
        "back": ["POST /work/done", "the answer"],
    },
}

# The security page's illustrations.
#
# They live in their own directory because they are NOT guide diagrams: a guide
# shows the objects a reader writes, and these show what a control bounds. The
# `dir` key is what keeps them apart, and every entry above defaults to "guides"
# rather than restating it.
#
# ONE ILLUSTRATION PER CLAIM THAT IS HARD TO READ IN PROSE. The Secrets boundary
# and the context mount are the two the page cannot state in a sentence anyone
# believes on first reading — a picture of the path is what makes them land.
# One per integration page, and each shows the SAME three seams a Pipeline
# wires: what starts work, the route, and what the agent reaches or answers on.
# A reader who met the model on the landing page meets it again per vendor.
# One per integration page. THREE COLUMNS, LIKE EVERY OTHER DIAGRAM HERE, and
# the geometry note above says why: the site displays a drawing in a 720px
# column with no breakout, so 950 authored is 0.76 on screen and a FOURTH
# column drops that to 0.55 -- where a 14px label renders under 8px. These were
# authored with four and looked exactly as small as that predicts.
#
# Each shows the same three beats an adopter reads the page for: what arrives,
# what decides whether it is real, and what answers.
INTEGRATION_DIAGRAMS: dict[str, dict] = {
    # TWO LANES, AND THE ROUTE IS WHERE THEY MEET. That is the product's own
    # model -- a Pipeline claims a source AND binds the tools -- so the picture
    # states it rather than showing a single file of boxes.
    #
    # It also keeps the MCP server, which a three-box version dropped. The
    # renderer pairs row to row when BOTH columns hold two boxes, so each lane
    # gets its own arrow instead of the server hanging off the rules.
    "kubernetes": {
        "dir": "integrations",
        "alt": "Two lanes meet on the route: cluster events filtered by your "
               "suppression rules, and the MCP server's tools filtered by the "
               "toolsets you bind.",
        "cols": [
            [("a cluster Event", "Warning, by default", "plain"),
             ("the MCP server", "reads the cluster", "subject")],
            [("your rules", "still broken?", "yours"),
             ("your toolsets", "what it may call", "yours")],
            [("k8s-observe", "where they meet", "subject")],
        ],
        "arrows": ["filtered by", "meet on"],
    },
    "prometheus": {
        "dir": "integrations",
        "alt": "Two lanes meet on the route: firing alerts your Alertmanager "
               "sends, and the metrics the query server exposes through the "
               "toolset you bind.",
        "cols": [
            [("a firing alert", "from Alertmanager", "plain"),
             ("the query server", "reads your metrics", "subject")],
            [("your receiver", "which alerts to send", "yours"),
             ("your toolsets", "what it may call", "yours")],
            [("alert-triage", "where they meet", "subject")],
        ],
        "arrows": ["chosen by", "meet on"],
    },
    "home-assistant": {
        "dir": "integrations",
        "alt": "A Home Assistant log record is re-checked by the rules, and "
               "opens a conversation on one of two routes: one that uses the "
               "house, one that repairs it.",
        "cols": [
            [("a log record", "from your house", "plain")],
            [("your rules", "live at the close?", "yours")],
            [("ha-control", "uses the house", "subject"),
             ("ha-ops", "repairs it", "subject")],
        ],
        "arrows": ["watched", "only if real"],
    },
    # TELEGRAM IS THE ONE THAT ANSWERS, so its second lane is the reply rather
    # than a toolset -- this bundle grants none. Both lanes run through the
    # single poller, which is the rule the whole integration is shaped by.
    "telegram": {
        "dir": "integrations",
        "alt": "One poller reads every update for a bot token: a message on "
               "the group surface starts a conversation, and a message in a "
               "topic continues the one that owns it.",
        # ONE BOX IN THE MIDDLE, DELIBERATELY. Both kinds of message fan IN to
        # it and it fans back OUT, which is the whole rule: a second concurrent
        # reader of one bot token gets 409s and steals updates. Drawn as two
        # middle boxes it would say the opposite.
        "cols": [
            [("on the group", "anyone can start one", "plain"),
             ("in a topic", "the conversation there", "plain")],
            [("one poller", "per bot token", "yours")],
            [("a new topic", "opened to answer in", "subject"),
             ("its next turn", "answered in place", "subject")],
        ],
        "arrows": ["both read by", "routed to"],
    },
}

SECURITY_DIAGRAMS: dict[str, dict] = {
    "connect": {
        "dir": "security",
        "alt": "Any pod in the cluster may reach the MCP servers and the work "
               "contract, because neither authenticates a caller, unless "
               "network policy segments them.",
        "cols": [
            [("any pod", "in the cluster", "plain")],
            [("NetworkPolicy", "off by default", "yours")],
            [("MCP servers", "unauthenticated", "subject"),
             ("work contract", "unauthenticated", "subject")],
        ],
        "arrows": ["may reach", "unless segmented"],
    },
    "tools": {
        "dir": "security",
        "alt": "An agent with a shell bypasses the command-line allowlist, but "
               "cannot bypass the egress proxy inside its own pod.",
        "cols": [
            [("the agent", "and its shell", "subject")],
            [("the allowlist", "configures the CLI", "plain"),
             ("egress proxy", "on by default", "yours")],
            [("an MCP server", "and its tools", "plain")],
        ],
        "arrows": ["bypasses", "cannot bypass"],
    },
    "secrets": {
        "dir": "security",
        "alt": "An agent allowed to create or enter a pod reads a Secret "
               "through the kubelet, without ever asking the API server for it.",
        "cols": [
            [("create a pod", "or exec into one", "subject")],
            [("the kubelet", "mounts what it names", "plain")],
            [("the Secret", "never asked the API", "yours")],
        ],
        "arrows": ["is allowed to", "hands over"],
    },
    "context": {
        "dir": "security",
        "alt": "Under context sync the agent container holds no mount of the "
               "durable volume, and a sidecar snapshots to it instead.",
        "cols": [
            [("agent container", "ephemeral, pod-local", "subject")],
            [("context-sync", "holds the volume", "yours")],
            [("durable volume", "one path per convo", "plain")],
        ],
        "arrows": ["no mount of it", "snapshots to"],
    },
}


# One per RUNTIME page. A runtime is not an integration -- it starts no work
# and answers nowhere -- so its three beats are its own: what hands it work,
# what it does itself, and what it reaches for. The middle box is the one the
# page is about, and for runtime-ollama the point of the picture is how much
# sits in that box: the loop, the tools and the transcript are the runtime's,
# and the model server is asked for one thing.
RUNTIME_DIAGRAMS: dict[str, dict] = {
    "ollama": {
        "dir": "runtimes",
        "alt": "The manager hands a work unit to runtime-ollama, which runs the "
               "agent loop and the tools itself, keeps one transcript per "
               "conversation, and asks Ollama only for the next message.",
        "cols": [
            [("the manager", "one unit at a time", "plain")],
            [("runtime-ollama", "loop, tools, transcript", "yours")],
            [("Ollama", "the next message only", "subject"),
             ("the context", "one transcript each", "plain")],
        ],
        # No return line: drawn, it ran from the model server back to the
        # manager and read as Ollama reporting the run. The report is the
        # runtime's, and the page says so in words.
        "arrows": ["GET /work", "asks / keeps"],
    },
}


def _ink(kind: str, p: dict[str, str]) -> tuple[str, str, str]:
    """fill, stroke, and the accent bar down the left edge.

    The bar is what stops these reading as a row of default rectangles: it
    carries the ROLE, so a reader can tell at a glance which box they write.
    """
    if kind == "yours":
        return p["brand-soft"], p["brand"], p["brand-strong"]
    if kind == "subject":
        return p["surface-alt"], p["border"], p["accent"]
    return p["lane-fill"], p["lane-border"], p["neutral"]


def render(spec: dict, p: dict[str, str]) -> str:
    cols = spec["cols"]
    for label in spec["arrows"] + spec.get("back", []):
        if len(label) > LABEL_MAX:
            raise SystemExit(
                f"arrow label {label!r} is {len(label)} characters, over the "
                f"{LABEL_MAX} the column gap fits. Shorten it or widen COL_GAP."
            )
    for col in cols:
        for title, sub, _ in col:
            if len(title) > TITLE_MAX or len(sub) > SUB_MAX:
                raise SystemExit(
                    f"box text {title!r}/{sub!r} is wider than a {BOX_W}px box "
                    f"(title max {TITLE_MAX}, sub max {SUB_MAX})"
                )
    tallest = max(len(c) for c in cols)
    height = PAD * 2 + tallest * BOX_H + (tallest - 1) * ROW_GAP
    if spec.get("back"):
        height += 54
    width = PAD * 2 + len(cols) * BOX_W + (len(cols) - 1) * COL_GAP

    def col_x(i: int) -> int:
        return PAD + i * (BOX_W + COL_GAP)

    def box_y(i: int, j: int) -> float:
        n = len(cols[i])
        block = n * BOX_H + (n - 1) * ROW_GAP
        top = PAD + (tallest * BOX_H + (tallest - 1) * ROW_GAP - block) / 2
        return top + j * (BOX_H + ROW_GAP)

    out: list[str] = []
    out.append(
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width} {height}" '
        f'width="{width}" height="{height}" role="img">'
    )
    # Its own ground: a transparent drawing on a dark page is invisible ink.
    out.append(f'<rect width="{width}" height="{height}" fill="{p["canvas"]}"/>')
    out.append(
        f'<defs><marker id="a" viewBox="0 0 10 10" refX="9" refY="5" '
        f'markerWidth="7" markerHeight="7" orient="auto-start-reverse">'
        f'<path d="M0,0 L10,5 L0,10 z" fill="{p["edge-idle"]}"/></marker>'
        f'<marker id="b" viewBox="0 0 10 10" refX="9" refY="5" '
        f'markerWidth="7" markerHeight="7" orient="auto-start-reverse">'
        f'<path d="M0,0 L10,5 L0,10 z" fill="{p["accent"]}"/></marker></defs>'
    )

    BAR = 4
    for i, col in enumerate(cols):
        for j, (title, sub, kind) in enumerate(col):
            x, y = col_x(i), box_y(i, j)
            fill, stroke, bar = _ink(kind, p)
            out.append(
                f'<rect x="{x}" y="{y:.0f}" width="{BOX_W}" height="{BOX_H}" rx="10" '
                f'fill="{fill}" stroke="{stroke}" stroke-width="1"/>'
            )
            # The bar is clipped to the rounded corner by a second rect drawn
            # inside it, rather than a path: fewer coordinates to get wrong.
            out.append(
                f'<path d="M{x + 10},{y:.0f} H{x + BAR} A10,10 0 0 0 {x},{y + 10:.0f} '
                f'V{y + BOX_H - 10:.0f} A10,10 0 0 0 {x + BAR},{y + BOX_H:.0f} '
                f'H{x + 10} V{y:.0f} Z" fill="{bar}"/>'
            )
            out.append(
                f'<text x="{x + 20}" y="{y + 31:.0f}" font-family="{FONT}" '
                f'font-size="16" font-weight="600" fill="{p["text"]}">'
                f'{sax.escape(title)}</text>'
            )
            out.append(
                f'<text x="{x + 20}" y="{y + 52:.0f}" font-family="{FONT}" '
                f'font-size="12.5" fill="{p["text-subtle"]}">{sax.escape(sub)}</text>'
            )

    def arrow(x1: float, y1: float, x2: float, y2: float) -> str:
        if abs(y1 - y2) < 1:
            d = f"M{x1:.0f},{y1:.0f} H{x2:.0f}"
        else:
            mid = (x1 + x2) / 2
            d = f"M{x1:.0f},{y1:.0f} H{mid:.0f} V{y2:.0f} H{x2:.0f}"
        return (f'<path d="{d}" fill="none" stroke="{p["edge-idle"]}" '
                f'stroke-width="1.5" marker-end="url(#a)"/>')

    for i, label in enumerate(spec["arrows"]):
        left, right = cols[i], cols[i + 1]
        x1 = col_x(i) + BOX_W
        x2 = col_x(i + 1)
        for j in range(len(left)):
            for k in range(len(right)):
                if len(left) > 1 and len(right) > 1 and j != k:
                    continue
                out.append(arrow(x1, box_y(i, j) + BOX_H / 2,
                                 x2 - 6, box_y(i + 1, k) + BOX_H / 2))
        # ABOVE ITS OWN ARROW, not above the drawing. Centring the label on the
        # diagram put `signalSourceRefs` straight through the Pipeline box the
        # moment a column had three rows in it.
        anchor_y = box_y(i, 0) + BOX_H / 2 if len(left) == 1 else box_y(i + 1, 0) + BOX_H / 2
        out.append(
            f'<text x="{(x1 + x2) / 2:.0f}" y="{anchor_y - 12:.0f}" '
            f'text-anchor="middle" font-family="{MONO}" font-size="11.5" '
            f'fill="{p["text-subtle"]}">{sax.escape(label)}</text>'
        )

    # The return path, for a contract that is answered rather than only fed.
    #
    # It drops from the CENTRE of each box rather than its edge, so the
    # horizontal run is a box wider than the gap. Routed edge to edge, the run
    # was 150px and the label 145, and every label grazed its own drop line.
    if spec.get("back"):
        y = height - 26
        for i, label in enumerate(spec["back"]):
            x_from = col_x(i + 1) + BOX_W / 2
            x_to = col_x(i) + BOX_W / 2
            bottom_from = box_y(i + 1, 0) + BOX_H
            bottom_to = box_y(i, 0) + BOX_H
            out.append(
                f'<path d="M{x_from:.0f},{bottom_from:.0f} V{y:.0f} H{x_to:.0f} '
                f'V{bottom_to + 6:.0f}" fill="none" stroke="{p["accent"]}" '
                f'stroke-width="1.5" stroke-dasharray="5 4" marker-end="url(#b)"/>'
            )
            out.append(
                f'<text x="{(x_from + x_to) / 2:.0f}" y="{y - 8:.0f}" '
                f'text-anchor="middle" font-family="{MONO}" font-size="11.5" '
                f'fill="{p["accent"]}">{sax.escape(label)}</text>'
            )

    out.append("</svg>")
    return "\n".join(out) + "\n"
