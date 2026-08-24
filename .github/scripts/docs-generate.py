#!/usr/bin/env python3
"""Generate every custom resource shown on the site, from the CRDs and the chart.

Nothing here is hand-written, and that is the point. A CR example typed into a
guide is correct on the day it is typed and silently wrong after the next field
rename -- the reader copies a field that no longer exists and gets a resource
the API server accepts and the operator ignores.

Two sources, and no third:

    templates  <- chart/crds/          the fields, their types, their
                                             descriptions (the Go doc comments
                                             land here, so a template's comment
                                             cannot disagree with the type)
    examples   <- helm template <preset>     real objects the chart produces,
                                             verbatim, with their own values

A page declares what it wants with an HTML comment and this script fills the
region beneath it:

    <!-- generated: template kind=Pipeline name=my-route fields=channelRefs -->
    ```yaml
    ...
    ```
    <!-- /generated -->

    <!-- generated: example preset=tier1 kind=Pipeline name=k8s-observe -->

CI runs this with --check. A rename, a description edit or a chart value change
fails the build until the pages are regenerated, which is the whole reason to
generate rather than write.

Usage:
    python3 .github/scripts/docs-generate.py            # write
    python3 .github/scripts/docs-generate.py --check    # fail on any difference
"""
from __future__ import annotations

import argparse
import pathlib
import re
import subprocess
import sys
import textwrap

import yaml

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import docs_diagrams  # noqa: E402  -- beside this file, loaded by path

REPO = pathlib.Path(__file__).resolve().parents[2]
CRD_DIR = REPO / "chart" / "crds"
DOCS = REPO / "docs"
CHART = REPO / "chart"
REFERENCE = DOCS / "cr-reference.md"
STYLESHEET = DOCS / "assets" / "css" / "agentops.css"
DIAGRAM_DIR = DOCS / "assets" / "img" / "guides"
COMMAND = "python3 .github/scripts/docs-generate.py"

API_VERSION = "agentops.dev/v1alpha1"
RELEASE = "agent-ops"
NAMESPACE = "agent-ops"

# Values the chart cannot guess, so a render must supply them. An example is a
# thing an adopter COPIES, so a real endpoint or a real chat id here reaches
# every reader of the site -- and gets pasted into a cluster.
#
# Two classes, and the difference is enforced below rather than trusted:
#
#   DOCUMENTED   the chart's own values file carries this literal as its
#                example. `assert_placeholders` fails if it stops doing so, so
#                the site and the shipped values cannot come to disagree.
#   CREDENTIAL   a dummy that no chart may ever document, because a values file
#                carrying an example token teaches someone to ship it. Exempt
#                from the chart check by KEY, never by value.
DOCUMENTED_PLACEHOLDERS = {
    "https://ha.example.org",           # home-assistant/values.yaml, homeAssistant.endpoint
    "http://metrics.example:8428",      # RFC 2606 reserved domain
    "-1001234567890",                   # telegram/values.yaml, surface.chatId
}
CREDENTIAL_PLACEHOLDERS = {
    "placeholder-token",
    "placeholder:token",
}
PLACEHOLDERS = DOCUMENTED_PLACEHOLDERS | CREDENTIAL_PLACEHOLDERS

# A --set key naming a credential. Its value is a dummy, never an example.
CREDENTIAL_KEY = re.compile(r"(?i)credential|token|secret|password")

# Hosts a published example may name. Everything else is somebody's real
# infrastructure until proven otherwise.
ALLOWED_HOSTS = {
    "example", "example.com", "example.net", "example.org",
    "ha.example.org", "metrics.example",
    "github.com", "kubernetes.io", "keepachangelog.com",
    "localhost", "127.0.0.1",
    # The SVG namespace, which every generated drawing must declare. The audit
    # still runs over those files: a real hostname DRAWN into a diagram is as
    # published as one typed into prose, and harder to notice.
    "www.w3.org",
}

# Identifier shapes worth refusing on sight. A chat id is the one this
# repository has actually shipped as "the value to copy".
IDENTIFIER_SHAPES = {
    "telegram chat id": re.compile(r"-100\d{9,}"),
}

# Which render each tier's examples come from. `sets` are the values that
# subject requires -- omit them and the bundle refuses to render, so the guard
# would be testing the guards and never reaching the templates.
PRESETS = {
    "tier1": {
        "sets": {
            "global.demo.enabled": "true",
            "kubernetes.enabled": "true",
        },
    },
    "tier2": {
        "sets": {
            "home-assistant.enabled": "true",
            "home-assistant.pipelines.enabled": "true",
            "home-assistant.homeAssistant.endpoint": "https://ha.example.org",
            "home-assistant.homeAssistant.credentials.operatorToken": "placeholder-token",
        },
    },
    "tier3": {
        "sets": {
            "telegram.enabled": "true",
            "telegram.surface.enabled": "true",
            "telegram.surface.chatId": "-1001234567890",
            "telegram.surface.credentials.botToken": "placeholder:token",
        },
    },
    "tier4": {"sets": {}},
}

# Kinds an adopter writes, in the order the guides teach them. The reference
# lists these and nothing else -- Conversation and ConversationInput are
# materialized by the operator, and a reference that invites someone to write
# one teaches them to fight the reconciler.
AUTHORED_KINDS = [
    "AgentProfile",
    "Pipeline",
    "MCPToolset",
    "MCPConfig",
    "SignalSource",
    "SignalAdapter",
    "Channel",
    "ChannelAdapter",
    "AgentRuntime",
]
OPERATOR_KINDS = ["Conversation", "ConversationInput"]


# --------------------------------------------------------------------------
# CRDs
# --------------------------------------------------------------------------

def load_crds() -> dict[str, dict]:
    """kind -> the served version's openAPIV3Schema."""
    schemas: dict[str, dict] = {}
    for path in sorted(CRD_DIR.glob("*.yaml")):
        for doc in yaml.safe_load_all(path.read_text()):
            if not doc or doc.get("kind") != "CustomResourceDefinition":
                continue
            kind = doc["spec"]["names"]["kind"]
            for version in doc["spec"]["versions"]:
                if not version.get("served"):
                    continue
                schema = (version.get("schema") or {}).get("openAPIV3Schema")
                if schema:
                    schemas[kind] = schema
    if not schemas:
        raise SystemExit(f"no CRDs under {CRD_DIR}")
    return schemas


def resolve(schema: dict, path: str) -> dict:
    """Walk a dotted path into a schema. Raises on a field that is not there,
    which is what makes a rename fail loudly rather than emit a stale block."""
    node = schema
    walked: list[str] = []
    for part in path.split("."):
        if node.get("type") == "array":
            node = node.get("items", {})
        if part == "*":
            # A map keyed by a name the adopter chooses.
            inner = node.get("additionalProperties")
            if not isinstance(inner, dict):
                raise SystemExit(f"{'.'.join(walked)!r} is not a map, so it has no `*`")
            node = inner
            walked.append(part)
            continue
        props = node.get("properties") or {}
        if part not in props:
            known = ", ".join(sorted(props)) or "(none)"
            raise SystemExit(
                f"no field {'.'.join(walked + [part])!r}; known here: {known}"
            )
        node = props[part]
        walked.append(part)
    return node


def type_of(node: dict) -> str:
    t = node.get("type", "object")
    if t == "array":
        return f"[]{type_of(node.get('items', {}))}"
    if t == "object" and "additionalProperties" in node:
        inner = node["additionalProperties"]
        if isinstance(inner, dict):
            return f"map[string]{type_of(inner)}"
    if node.get("x-kubernetes-preserve-unknown-fields"):
        return "free-form"
    return t


def one_line(text: str | None) -> str:
    """A CRD description as one table cell. Backticks and pipes both survive."""
    if not text:
        return ""
    collapsed = " ".join(text.split())
    return collapsed.replace("|", "\\|")


def flatten(node: dict, prefix: str = "", required: bool = False) -> list[dict]:
    """Every field under a schema node, as dotted paths."""
    rows: list[dict] = []
    if node.get("type") == "array":
        node = node.get("items", {})
        prefix = prefix + "[]"
    props = node.get("properties") or {}
    req = set(node.get("required") or [])
    for name in sorted(props):
        child = props[name]
        path = f"{prefix}.{name}" if prefix else name
        rows.append(
            {
                "path": path,
                "type": type_of(child),
                "required": name in req,
                "description": one_line(child.get("description")),
            }
        )
        target = child.get("items", {}) if child.get("type") == "array" else child
        if (target.get("properties") or {}):
            rows.extend(flatten(child, prefix=path))
    return rows


# --------------------------------------------------------------------------
# Templates
# --------------------------------------------------------------------------

def first_sentence(text: str) -> str:
    """The opening sentence of a CRD description, for a template comment.

    Split on a full stop followed by a capital, so `(e.g. a webhook)` stays
    whole -- a comment cut mid-abbreviation reads as a truncation bug.
    """
    collapsed = " ".join(text.split())
    cut = re.split(r"(?<=[.!?]) +(?=[A-Z(])", collapsed, maxsplit=1)[0]
    return cut.rstrip(".")


def placeholder(node: dict, name: str) -> str:
    """A value a reader replaces, that a validator still accepts."""
    if node.get("default") not in (None, "", [], {}):
        return yaml.safe_dump(node["default"], default_flow_style=True, width=10_000).strip().rstrip("\n.")
    enum = node.get("enum")
    if enum:
        return f"{enum[0]}   # {' | '.join(str(e) for e in enum)}"
    kind = node.get("type")
    if kind in ("integer", "number"):
        return "8080" if name.endswith("ort") else str(node.get("minimum", 1))
    if kind == "boolean":
        return "false"
    if node.get("format") == "quantity" or name in ("cpu", "memory"):
        return '"100m"'
    return f"<{name}>"


def render_node(node: dict, name: str, wanted: set[str], base: str, indent: int,
                order: list[str], comments: bool) -> list[str]:
    """One field of a template, plus whatever of it the page asked for."""
    pad = " " * indent
    desc = node.get("description")
    lines: list[str] = []
    # Only top-level spec fields carry their comment. Below that the shapes are
    # small and the page's own prose covers them, while "Name of the referenced
    # object" repeated six times buries what the template is teaching.
    #
    # `comments=off` drops them entirely, for a step in a how-to: there the
    # prose above the block has already said what to fill in, and repeating it
    # as YAML comments doubles the height of the thing a reader must copy.
    if desc and indent <= 2 and comments:
        for wrapped in textwrap.wrap(first_sentence(desc), width=74 - indent):
            lines.append(f"{pad}# {wrapped}")
    if node.get("type") == "array":
        items = node.get("items", {})
        if (items.get("properties") or {}):
            lines.append(f"{pad}{name}:")
            lines.append(f"{pad}- ")
            body = render_object(items, wanted, f"{base}[]", indent + 2, order, comments)
            if body:
                lines[-1] = f"{pad}- {body[0].lstrip()}"
                lines.extend(body[1:])
            else:
                lines[-1] = f"{pad}- {{}}"
        else:
            lines.append(f"{pad}{name}:")
            lines.append(f"{pad}- {placeholder(items, name.rstrip('s') or 'item')}")
        return lines
    if (node.get("properties") or {}):
        body = render_object(node, wanted, base, indent + 2, order, comments)
        if not body and base in wanted:
            # Named by the page, but every property under it is optional. Show
            # them all rather than an empty key: the page asked for this shape.
            body = render_object(node, wanted | {f"{base}.{p}" for p in node["properties"]},
                                 base, indent + 2, order, comments)
        if not body:
            return []
        lines.append(f"{pad}{name}:")
        lines.extend(body)
        return lines
    inner = node.get("additionalProperties")
    if isinstance(inner, dict) and (inner.get("properties") or {}):
        # A map keyed by a name the adopter chooses. Show one entry.
        lines.append(f"{pad}{name}:")
        lines.append(f"{pad}  <{name.rstrip('s') or 'key'}>:")
        lines.extend(render_object(inner, wanted, f"{base}.*", indent + 4, order, comments))
        return lines
    if node.get("x-kubernetes-preserve-unknown-fields") and node.get("type") != "string":
        # Schema-less on purpose. The serving implementation defines the shape,
        # so inventing keys here would put one adapter's config in every guide.
        lines.append(f"{pad}{name}: {{}}")
        return lines
    lines.append(f"{pad}{name}: {placeholder(node, name)}")
    return lines


def render_object(node: dict, wanted: set[str], base: str, indent: int,
                  order: list[str] | None = None, comments: bool = True) -> list[str]:
    """Required properties always, optional ones only where the page named them.

    Order is required-first, then the order the PAGE asked for -- a guide
    teaches its fields in a sequence, and alphabetical throws that away.
    """
    props = node.get("properties") or {}
    req = set(node.get("required") or [])
    order = order or []

    def rank(name: str) -> tuple:
        path = f"{base}.{name}" if base else name
        asked = next((i for i, w in enumerate(order) if w == path or w.startswith(path + ".")),
                     len(order))
        return (name not in req, asked, name)

    lines: list[str] = []
    for name in sorted(props, key=rank):
        path = f"{base}.{name}" if base else name
        keep = name in req or path in wanted or any(
            w.startswith(path + ".") or w.startswith(path + "[]") for w in wanted
        )
        if not keep:
            continue
        lines.extend(render_node(props[name], name, wanted, path, indent, order, comments))
    return lines


def build_template(schema: dict, kind: str, name: str, fields: list[str],
                   comments: bool = True) -> str:
    spec = (schema.get("properties") or {}).get("spec") or {}
    wanted: set[str] = set()
    for field in fields:
        resolve(spec, field.replace("[]", ""))
        parts = field.split(".")
        for i in range(1, len(parts) + 1):
            wanted.add(".".join(parts[:i]))
    body = render_object(spec, wanted, "", 2, order=fields, comments=comments)
    lines = [
        f"apiVersion: {API_VERSION}",
        f"kind: {kind}",
        "metadata:",
        f"  name: {name}",
    ]
    if body:
        lines.append("spec:")
        lines.extend(body)
    else:
        lines.append("spec: {}")
    return "\n".join(lines)


# --------------------------------------------------------------------------
# Examples
# --------------------------------------------------------------------------

def chart_values_text() -> str:
    """Every values file the chart ships, comments included.

    Comments count: a values file documents its example in one, which is where
    `chatId` and `endpoint` live. The point is that the site and the chart name
    the same literal, not where the chart happens to put it.
    """
    files = [CHART / "values.yaml", *sorted(CHART.glob("charts/*/values.yaml"))]
    return "\n".join(f.read_text() for f in files if f.exists())


def assert_placeholders() -> list[str]:
    """No invented values: what a preset supplies, the chart already documents.

    Without this the generator is free to make up an example the chart has
    never heard of -- which is how a second set of values to keep true gets
    created, and how the better-looking real identifier gets pasted in.
    """
    values = chart_values_text()
    problems = []
    for preset, config in PRESETS.items():
        for key, value in config["sets"].items():
            if value in ("true", "false"):
                continue
            if CREDENTIAL_KEY.search(key):
                if value not in CREDENTIAL_PLACEHOLDERS:
                    problems.append(
                        f"preset {preset}: {key}={value!r} is a credential and is not a "
                        f"declared dummy. Add it to CREDENTIAL_PLACEHOLDERS -- and never "
                        f"to a values file."
                    )
                continue
            if value not in DOCUMENTED_PLACEHOLDERS:
                problems.append(
                    f"preset {preset}: {key}={value!r} is not a declared placeholder. "
                    f"Add it to DOCUMENTED_PLACEHOLDERS only once it names nothing real."
                )
            elif value not in values:
                problems.append(
                    f"preset {preset}: {key}={value!r} is no longer documented by any "
                    f"chart values file. The site and the chart must name the SAME "
                    f"example -- put it back, or follow the chart to its new one."
                )
    return problems


def audit_identifiers(produced: dict[pathlib.Path, str]) -> list[str]:
    """Nothing real reaches a published page.

    Every host an example names must be a reserved one, and no identifier may
    match a shape this repository has previously shipped as a real value. The
    check is by SHAPE and by ALLOWLIST, never by naming the literal it guards
    against -- a scrubbed value must not be re-introduced by its own guard.
    """
    problems = []
    for path, body in produced.items():
        for host in set(re.findall(r"https?://([A-Za-z0-9.:-]+)", body)):
            bare = host.split(":")[0]
            if host in ALLOWED_HOSTS or bare in ALLOWED_HOSTS:
                continue
            if bare.endswith(".svc.cluster.local") or bare.endswith(".example"):
                continue
            problems.append(f"{path.name}: names host {host!r}, which is not a reserved example")
        for what, shape in IDENTIFIER_SHAPES.items():
            for hit in set(shape.findall(body)):
                if hit not in PLACEHOLDERS:
                    problems.append(
                        f"{path.name}: {hit!r} is shaped like a {what} and is not the "
                        f"declared placeholder"
                    )
    return problems


_renders: dict[str, list[dict]] = {}


def render_preset(preset: str) -> list[dict]:
    """helm template a preset once, and keep every document's RAW text."""
    if preset in _renders:
        return _renders[preset]
    config = PRESETS[preset]
    argv = ["helm", "template", RELEASE, str(CHART), "-n", NAMESPACE]
    for key, value in config["sets"].items():
        if value not in ("true", "false") and value not in PLACEHOLDERS:
            raise SystemExit(
                f"preset {preset}: {key}={value!r} is not a declared placeholder. "
                "An example is copied by every reader -- add it to PLACEHOLDERS "
                "only once it names nothing real."
            )
        argv += ["--set", f"{key}={value}"]
    out = subprocess.run(argv, capture_output=True, text=True)
    if out.returncode != 0:
        raise SystemExit(f"preset {preset} failed to render:\n{out.stderr}")
    docs: list[dict] = []
    for chunk in out.stdout.split("\n---\n"):
        text = chunk.strip("\n")
        if not text.strip():
            continue
        try:
            parsed = yaml.safe_load(text)
        except yaml.YAMLError:
            continue
        if not isinstance(parsed, dict) or "kind" not in parsed:
            continue
        docs.append({"kind": parsed["kind"], "name": parsed["metadata"]["name"], "text": text})
    _renders[preset] = docs
    return docs


def build_example(preset: str, kind: str, name: str) -> str:
    for doc in render_preset(preset):
        if doc["kind"] == kind and doc["name"] == name:
            return doc["text"]
    available = ", ".join(
        sorted({f"{d['kind']}/{d['name']}" for d in render_preset(preset) if d["kind"] == kind})
    ) or "(no object of that kind)"
    raise SystemExit(f"preset {preset} renders no {kind}/{name}. Of that kind: {available}")


# --------------------------------------------------------------------------
# The reference
# --------------------------------------------------------------------------

def build_reference(schemas: dict[str, dict]) -> str:
    out: list[str] = []
    out.append("# Custom resource reference")
    out.append("")
    out.append(
        f"**Generated from `chart/crds/` by `{COMMAND}`. Do not edit.**"
    )
    out.append("")
    out.append(
        "Every field of every kind, with the type the API server enforces. "
        "What a field MEANS beyond its own sentence is in "
        "[concepts.md](concepts.md), and the contracts the adapter and runtime "
        "kinds serve are in [contracts.md](contracts.md)."
    )
    out.append("")
    out.append(f"API group: `{API_VERSION}`. Every kind is namespaced.")
    out.append("")
    out.append("## Kinds")
    out.append("")
    out.append("| Kind | You write it | Fields |")
    out.append("|---|---|---|")
    for kind in AUTHORED_KINDS + OPERATOR_KINDS:
        rows = flatten((schemas[kind].get("properties") or {}).get("spec") or {})
        written = "yes" if kind in AUTHORED_KINDS else "no — the operator does"
        anchor = kind.lower()
        out.append(f"| [{kind}](#{anchor}) | {written} | {len(rows)} |")
    out.append("")
    for kind in AUTHORED_KINDS + OPERATOR_KINDS:
        schema = schemas[kind]
        out.append(f"## {kind}")
        out.append("")
        spec = (schema.get("properties") or {}).get("spec") or {}
        summary = one_line(spec.get("description"))
        if summary:
            out.append(summary)
            out.append("")
        out.append("### spec")
        out.append("")
        out.extend(field_table(flatten(spec)))
        out.append("")
        status = (schema.get("properties") or {}).get("status") or {}
        status_rows = flatten(status)
        if status_rows:
            out.append("### status")
            out.append("")
            out.append("Written by the operator. Read it, never set it.")
            out.append("")
            out.extend(field_table(status_rows))
            out.append("")
    return "\n".join(out).rstrip() + "\n"


def field_table(rows: list[dict]) -> list[str]:
    if not rows:
        return ["This kind has no fields of its own."]
    out = ["| Field | Type | Required | Description |", "|---|---|---|---|"]
    for row in rows:
        req = "**yes**" if row["required"] else ""
        out.append(
            f"| `{row['path']}` | `{row['type']}` | {req} | {row['description']} |"
        )
    return out


# --------------------------------------------------------------------------
# Injection
# --------------------------------------------------------------------------

MARKER = re.compile(
    r"(?P<open><!-- generated: (?P<attrs>[^>]*?) -->\n)"
    r"(?P<body>.*?)"
    r"(?P<close><!-- /generated -->)",
    re.DOTALL,
)


def parse_attrs(text: str) -> dict[str, str]:
    attrs: dict[str, str] = {}
    for token in text.split():
        key, _, value = token.partition("=")
        attrs[key] = value
    return attrs


# Every template this run produced, so `--emit-templates` can hand them to a
# validator. A template a reader copies must be a resource the API server takes.
_emitted: list[str] = []


def generate_block(attrs: dict[str, str], schemas: dict[str, dict]) -> str:
    kind = attrs.get("kind")
    if "template" in attrs:
        fields = [f for f in attrs.get("fields", "").split(",") if f]
        yaml_text = build_template(schemas[kind], kind, attrs.get("name", kind.lower()), fields,
                                   comments=attrs.get("comments") != "off")
        _emitted.append(yaml_text)
    elif "example" in attrs:
        yaml_text = build_example(attrs["preset"], kind, attrs["name"])
    else:
        raise SystemExit(f"marker declares neither template nor example: {attrs}")
    return f"```yaml\n{yaml_text}\n```\n"


def rewrite(path: pathlib.Path, schemas: dict[str, dict]) -> str:
    text = path.read_text()

    def replace(match: re.Match) -> str:
        attrs = parse_attrs(match.group("attrs"))
        try:
            body = generate_block(attrs, schemas)
        except SystemExit as err:
            # Name the file, the marker and the command. A drift check that
            # reports a difference without the fix is a puzzle.
            raise SystemExit(
                f"{path.relative_to(REPO)}: {err}\n"
                f"  in marker: <!-- generated: {match.group('attrs')} -->\n"
                f"  fix the marker, then run:\n    {COMMAND}"
            ) from err
        return match.group("open") + body + match.group("close")

    return MARKER.sub(replace, text)


# --------------------------------------------------------------------------

def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="fail on any difference")
    parser.add_argument(
        "--emit-templates",
        metavar="PATH",
        help="also write every generated template to PATH, for a validator",
    )
    args = parser.parse_args()

    if problems := assert_placeholders():
        print("Example values that name something real, or that the chart no longer "
              "documents:", file=sys.stderr)
        for problem in problems:
            print(f"  {problem}", file=sys.stderr)
        return 1

    schemas = load_crds()
    produced: dict[pathlib.Path, str] = {REFERENCE: build_reference(schemas)}

    # The guide diagrams, both themes, coloured from the stylesheet's own tokens.
    light, dark = docs_diagrams.palette(STYLESHEET)
    for slug, spec in docs_diagrams.DIAGRAMS.items():
        produced[DIAGRAM_DIR / f"{slug}-light.svg"] = docs_diagrams.render(spec, light)
        produced[DIAGRAM_DIR / f"{slug}-dark.svg"] = docs_diagrams.render(spec, dark)
    for path in sorted(DOCS.rglob("*.md")):
        if "_site" in path.parts or path == REFERENCE:
            continue
        text = path.read_text()
        if "<!-- generated:" not in text:
            continue
        produced[path] = rewrite(path, schemas)

    if args.emit_templates:
        pathlib.Path(args.emit_templates).write_text(
            "---\n".join(f"{doc}\n" for doc in _emitted)
        )
        print(f"wrote {len(_emitted)} template(s) to {args.emit_templates}")

    # Audit what was PRODUCED, plus the hand-written prose around it: a real
    # identifier typed into a guide is exactly as published as a generated one.
    audited = dict(produced)
    for path in sorted(DOCS.glob("guides/*.md")):
        audited.setdefault(path, path.read_text())
    if problems := audit_identifiers(audited):
        print("Published pages name something real:", file=sys.stderr)
        for problem in problems:
            print(f"  {problem}", file=sys.stderr)
        return 1

    stale = [p for p, body in produced.items() if not p.exists() or p.read_text() != body]
    if args.check:
        if stale:
            print("Generated documentation is stale:", file=sys.stderr)
            for path in stale:
                print(f"  {path.relative_to(REPO)}", file=sys.stderr)
            print(f"\nRegenerate with:\n  {COMMAND}", file=sys.stderr)
            return 1
        print(f"{len(produced)} generated file(s) up to date")
        return 0

    for path in stale:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(body := produced[path])
        print(f"wrote {path.relative_to(REPO)} ({len(body.splitlines())} lines)")
    if not stale:
        print(f"{len(produced)} generated file(s) already up to date")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
