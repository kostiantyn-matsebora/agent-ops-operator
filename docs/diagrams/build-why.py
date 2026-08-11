import base64, os, sys
IC = "icons"
THEME = sys.argv[1] if len(sys.argv) > 1 else "light"
DARK = THEME == "dark"

def img(n):
    raw = open(os.path.join(IC, n + ".svg"), "rb").read().decode()
    if DARK:
        # icons bake their stroke in, so the dark set is generated, not hand-drawn
        raw = (raw.replace('stroke="#1A1A1A"', 'stroke="#E6EDF3"')
                  .replace('fill="#1A1A1A"',   'fill="#E6EDF3"')
                  .replace('fill="#FFFFFF"',   'fill="#0D1117"'))
    return "data:image/svg+xml,%s" % base64.b64encode(raw.encode()).decode()

c = []
def cell(i, v, s, x, y, w, h):
    c.append('<mxCell id="%s" value="%s" style="%s" vertex="1" parent="1">'
             '<mxGeometry x="%s" y="%s" width="%s" height="%s" as="geometry"/></mxCell>'
             % (i, v, s, x, y, w, h))
def edge(i, s, t, style, pts):
    p = "".join('<mxPoint x="%s" y="%s"/>' % (a, b) for a, b in pts)
    c.append('<mxCell id="%s" style="%s" edge="1" parent="1" source="%s" target="%s">'
             '<mxGeometry relative="1" as="geometry"><Array as="points">%s</Array>'
             '</mxGeometry></mxCell>' % (i, style, s, t, p))

if DARK:
    INK, MUTE, AMBER, LINE = "#E6EDF3", "#9AA4B2", "#F0A030", "#3E4A59"
    BODY, PANEL, PANEL_ST = "#AAB4C0", "#161B22", "#2B3138"
    CARD, CARD_ST, TILE_BG, CHIP_TXT, YAML_BG, BG = "#161B22", "#30363D", "#241C0E", "#F0A030", "#010409", "#0D1117"
else:
    INK, MUTE, AMBER, LINE = "#1A1A1A", "#5F6B7A", "#F59E0B", "#B7C0CB"
    BODY, PANEL, PANEL_ST = "#5A6472", "#F8FAFC", "#D8DEE6"
    CARD, CARD_ST, TILE_BG, CHIP_TXT, YAML_BG, BG = "#FFFFFF", "#E2E8F0", "#FFFCF5", "#B45309", "#0D1117", "#FFFFFF"
ZONE = ("rounded=1;arcSize=3;html=0;fillColor=none;strokeColor=%s;dashed=1;dashPattern=4 4;"
        "verticalAlign=top;align=center;spacingTop=20;fontColor=%s;fontSize=20;fontStyle=1;" % (LINE, INK))
ICON = "shape=image;imageAspect=1;aspect=fixed;html=0;image=%s;"
LBL  = "text;html=0;align=center;verticalAlign=top;fontSize=19;fontColor=%s;fontStyle=1;" % INK
def sbs(col=MUTE):
    return "text;html=0;whiteSpace=wrap;align=center;verticalAlign=top;fontSize=16;fontColor=%s;" % col
WIRE = ("edgeStyle=orthogonalEdgeStyle;html=0;rounded=0;strokeColor=%s;strokeWidth=2;"
        "endArrow=none;startArrow=oval;startFill=1;startSize=9;" % INK)

# ---- header -------------------------------------------------------------
CHIP = "rounded=1;arcSize=50;html=0;fontSize=15;fontStyle=1;letterSpacing=2;"
cell("hrule", "", "rounded=0;html=0;fillColor=%s;strokeColor=none;" % AMBER, 48, 18, 8, 150)
cell("eyebrow", "AUTOMATION THAT THINKS",
     CHIP + "fillColor=%s;strokeColor=none;fontColor=#FFFFFF;" % AMBER, 76, 18, 296, 38)
cell("h1b", "KUBERNETES-NATIVE, END TO END",
     CHIP + "fillColor=%s;strokeColor=%s;strokeWidth=2;fontColor=%s;" % (CARD, AMBER, CHIP_TXT), 388, 18, 360, 38)
cell("h1", "Something happens. An agent takes care of it.",
     "text;html=0;align=left;verticalAlign=middle;fontSize=40;fontStyle=1;fontColor=%s;" % INK, 76, 70, 1260, 52)
cell("h2", "A signal wakes it, your prompt tells it what to do, your YAML decides what it may touch "
     "&#8212; a crashlooping pod or the hallway lights.",
     "text;html=0;align=left;verticalAlign=middle;fontSize=20;fontColor=%s;" % BODY, 76, 130, 1260, 30)

stats = [("11", "custom resource kinds &#8212; kubectl, GitOps and RBAC already work", CARD, CARD_ST),
         ("3",  "pluggable contracts: signals, runtimes, channels",                    CARD, CARD_ST),
         ("0",  "Secrets the operator ever reads",                                     CARD, CARD_ST),
         ("3",  "ready-made bundles to switch on: Kubernetes, Telegram, VictoriaMetrics", TILE_BG, AMBER)]
for n, (num, lab, fill, stroke) in enumerate(stats):
    y = 18 + n * 56
    cell("st%d" % n, "", "rounded=1;arcSize=14;html=0;fillColor=%s;strokeColor=%s;" % (fill, stroke), 1390, y, 428, 50)
    cell("sn%d" % n, num, "text;html=0;align=right;verticalAlign=middle;fontSize=30;fontStyle=1;fontColor=%s;" % AMBER,
         1406, y, 56, 50)
    cell("sl%d" % n, lab, "text;html=0;whiteSpace=wrap;align=left;verticalAlign=middle;fontSize=15.5;fontColor=%s;" % INK,
         1478, y, 328, 50)

# ---- operator boundary --------------------------------------------------
cell("panel", "", "rounded=1;arcSize=2;html=0;fillColor=%s;strokeColor=%s;" % (PANEL, PANEL_ST), 480, 250, 820, 940)
cell("ppill", "ONE HELM INSTALL, IN YOUR OWN CLUSTER",
     "rounded=1;arcSize=50;html=0;fillColor=%s;strokeColor=none;fontColor=#FFFFFF;fontSize=18;fontStyle=1;" % AMBER,
     600, 228, 580, 44)

def stack(pfx, items, cx, y0, step, w=380):
    for n, it in enumerate(items):
        y = y0 + n * step
        col = it[3] if len(it) > 3 else MUTE
        cell("%s-i%d" % (pfx, n), "", ICON % img(it[0]), cx - 40, y, 80, 80)
        cell("%s-l%d" % (pfx, n), it[1], LBL, cx - w // 2, y + 90, w, 28)
        cell("%s-s%d" % (pfx, n), it[2], sbs(col), cx - w // 2, y + 118, w, 26)

def row(pfx, items, xs, y, w=256):
    for n, (ic, lab, s) in enumerate(items):
        cx = xs[n]
        cell("%s-i%d" % (pfx, n), "", ICON % img(ic), cx - 40, y, 80, 80)
        cell("%s-l%d" % (pfx, n), lab, LBL, cx - w // 2, y + 90, w, 28)
        cell("%s-s%d" % (pfx, n), s, sbs(), cx - w // 2, y + 118, w, 26)

cell("zL", "SOMETHING HAPPENS", ZONE, 48, 282, 380, 908)
stack("L", [("alert",   "An alert fires",       "Alertmanager"),
            ("cluster", "A pod crashloops",     "Cluster events"),
            ("clock",   "A schedule comes due", "Cron"),
            ("thermo",  "A room gets too warm", "Home Assistant"),
            ("chat",    "Someone asks",         "A message in chat")], 238, 336, 176)

cell("zT", "YOU DECLARE IT &#8212; AS CUSTOM RESOURCES", ZONE, 504, 282, 772, 598)
row("T", [("pipeline", "Pipeline",     "what wakes it"),
          ("badge",    "AgentProfile", "what it should do"),
          ("key",      "MCPToolset",   "what it may touch")], [633, 890, 1147], 330)

RAW = """apiVersion: agentops.dev/v1alpha1
kind: Pipeline
metadata:
  name: k8s-ops
spec:
  signalSourceRefs:
    - name: cluster-events      # what wakes it
  profileRef:
    name: k8s-engineer          # what it should do
  toolsets:
    refs:
      - name: agentops-observe  # what it may touch
  channelRefs:
    - name: telegram            # where you talk to it"""
YAML = RAW.replace("&", "&amp;").replace("<", "&lt;").replace(" ", "&#160;").replace("\n", "&#10;")
cell("yc", "", "rounded=1;arcSize=6;html=0;fillColor=%s;strokeColor=%s;" % (YAML_BG, PANEL_ST), 536, 492, 712, 360)
cell("yt", "pipeline.yaml",
     "text;html=0;align=left;verticalAlign=top;fontSize=14;fontColor=%s;fontFamily=monospace;letterSpacing=1;" % AMBER,
     564, 510, 320, 20)
cell("yb", YAML,
     "text;html=0;align=left;verticalAlign=top;fontSize=17;fontColor=#E6EDF3;fontFamily=monospace;",
     564, 542, 680, 300)

cell("zB", "THE OPERATOR RUNS IT", ZONE, 504, 910, 772, 250)
row("B", [("thread", "One conversation",          "per incident, its own thread"),
          ("pod",    "Its own pod",               "isolated, serial, capped"),
          ("resume", "Picks up where it stopped", "a restart loses nothing")], [633, 890, 1147], 966)

cell("rhead", "WHAT &#8220;TAKES CARE OF IT&#8221; MEANS",
     "text;html=0;align=center;verticalAlign=middle;fontSize=19;fontStyle=1;fontColor=%s;" % INK, 1360, 282, 400, 28)
stack("R", [("search",  "Investigates", "queries the system, reads state"),
            ("explain", "Explains",     "in a thread you can reply to"),
            ("act",     "Acts",         "ONLY where your wiring granted it", AMBER),
            ("ask",     "Asks",         "when it needs you, it says so")], 1560, 340, 216, 420)

edge("e1", "zL", "zT", WIRE, [(454, 530), (454, 480)])
edge("e2", "zL", "zB", WIRE, [(454, 950), (454, 1020)])
for n, y in enumerate((380, 596, 812, 1028)):
    edge("r%d" % n, "zB", "R-i%d" % n, WIRE, [(1318, 1020), (1318, y)])

# ---- pluggability: a socket on each part, wired down to its tile --------
SOCK = ("edgeStyle=orthogonalEdgeStyle;html=0;rounded=0;strokeColor=%s;strokeWidth=2.5;"
        "dashed=1;dashPattern=5 5;endArrow=none;startArrow=none;"
        "exitX=0.5;exitY=1;entryX=0.5;entryY=0;" % AMBER)
for n, (host, bx, by) in enumerate([("zL", 384, 296), ("zB", 1224, 924), ("R-i3", 1786, 344)]):
    cell("sock%d" % n, "", ICON % img("plus"), bx, by, 38, 38)
    c.append('<mxCell id="socke%d" style="%s" edge="1" parent="1" source="%s" target="xt%d">'
             '<mxGeometry relative="1" as="geometry"/></mxCell>' % (n, SOCK, host, n))

cell("xhead", "PLUGGABLE AT THREE SEAMS &#8212; documented HTTP contracts, no fork",
     "text;html=0;align=left;verticalAlign=middle;fontSize=19;fontStyle=1;fontColor=%s;letterSpacing=1;labelBackgroundColor=%s;"
     % (INK, BG), 290, 1240, 600, 28)
TILE = "rounded=1;arcSize=8;html=0;fillColor=%s;strokeColor=%s;dashed=1;dashPattern=6 4;" % (TILE_BG, AMBER)
seams = [(48,   380, "Your own signal source",
          "Datadog, Dynatrace, Sentry, a sensor on your bench."),
         (480,  820, "Your own agent runtime",
          "Codex, Gemini, Copilot or a script of your own. Swap the image &#8212; the work contract does not change."),
         (1340, 478, "Your own channel",
          "Slack, Teams, Discord, e-mail. The operator sends meaning; your adapter renders it.")]
for n, (x, w, t, b) in enumerate(seams):
    cell("xt%d" % n, "", TILE, x, 1276, w, 144)
    cell("xi%d" % n, "", ICON % img("plus"), x + 22, 1318, 48, 48)
    cell("xl%d" % n, t, "text;html=0;whiteSpace=wrap;align=left;verticalAlign=top;fontSize=18;fontStyle=1;fontColor=%s;" % INK,
         x + 86, 1296, w - 108, 28)
    cell("xb%d" % n, b, "text;html=0;whiteSpace=wrap;align=left;verticalAlign=top;fontSize=15;fontColor=%s" % BODY + ";",
         x + 86, 1326, w - 108, 84)

diffs = [("Judgment, not a fixed sequence",
          "You describe the job in prose. Nobody enumerates the steps, and nothing has to have been predicted in advance."),
         ("Self-hosted, end to end",
          "It runs in your cluster on your credentials. No prompt, transcript or alert is sent to a vendor."),
         ("Bounded by construction",
          "One isolated pod per conversation. Tools come only from the wiring, and the operator itself never reads a Secret.")]
for n, (t, b) in enumerate(diffs):
    x = 48 + n * 604
    cell("dr%d" % n, "", "rounded=0;html=0;fillColor=%s;strokeColor=none;" % AMBER, x, 1462, 60, 6)
    cell("dt%d" % n, t, "text;html=0;align=left;verticalAlign=top;fontSize=20;fontStyle=1;fontColor=%s;" % INK,
         x, 1482, 552, 30)
    cell("db%d" % n, b, "text;html=0;whiteSpace=wrap;align=left;verticalAlign=top;fontSize=15.5;fontColor=%s" % BODY + ";",
         x, 1514, 552, 62)

xml = ('<mxfile host="app.diagrams.net" type="device"><diagram name="why" id="page-why">'
       '<mxGraphModel dx="1860" dy="1600" grid="0" gridSize="10" guides="1" tooltips="1" connect="1" '
       'arrows="1" fold="1" page="1" pageScale="1" pageWidth="1860" pageHeight="1600" math="0" shadow="0">'
       '<root><mxCell id="0"/><mxCell id="1" parent="0"/>' + "".join(c) +
       "</root></mxGraphModel></diagram></mxfile>")
open("why-%s.xml" % THEME, "w", encoding="utf-8").write(xml)
print("cells=%d bytes=%d" % (len(c), len(xml)))
