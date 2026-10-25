#!/usr/bin/env bash
# docs-generate.py and its docs_diagrams.py helper were never exercised by
# any test in this suite — both were entirely absent from the coverage
# report. Both scripts resolve their paths from `__file__`, not from a cwd
# or an argument, so they always operate on THIS checkout's own chart, CRDs
# and docs — there is no throwaway-repo fixture to build for them, unlike
# every other script in this directory.
#
# So this runs the real generator in its two READ-ONLY modes against this
# checkout: `--check` (the exact command CI runs, and the one the project's
# own docs already pass) and `--emit-templates` to a scratch file (which,
# because nothing is stale, still writes no repository file — see the `not
# stale` branch of main()). Together they walk load_crds, the CRD page set,
# every guide/integration/runtime/security diagram through
# docs_diagrams.palette()/render() (all four dicts, both themes, the
# back-path arrows, every box `kind`), rewrite() over every generated block
# in docs/, assert_placeholders, audit_identifiers and check_versions.
#
# NEITHER MODE WRITES. If a future drift makes `--check` fail, this test is
# reporting a real, pre-existing generator/doc mismatch — fix the docs (or
# regenerate them) rather than the test.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
GEN="$ROOT/.github/scripts/docs-generate.py"

# Scoped to docs/ — the only tree either mode can write to — because this is
# a shared worktree: several other sections of this same openspec change are
# being worked concurrently, and a repo-wide `git status` churns from that on
# its own.
before=$(cd "$ROOT" && git status --porcelain -- docs | sort)

it "--check reports the generated docs, CRD pages and every diagram as up to date against this checkout"
out=$(cd "$ROOT" && python3 "$GEN" --check 2>&1)
assert_status 0 "$?"
assert_contains "$out" "generated file(s) up to date"

it "it touched nothing under docs/"
after=$(cd "$ROOT" && git status --porcelain -- docs | sort)
assert_equals "$before" "$after"

it "--emit-templates writes every generated CR template to the given file, and still nothing in the tree"
tmp=$(mktemp -d)
out=$(cd "$ROOT" && python3 "$GEN" --emit-templates "$tmp/templates.yaml" 2>&1)
assert_status 0 "$?"
assert_contains "$out" "template(s) to $tmp/templates.yaml"
assert_contains "$out" "already up to date"
test -s "$tmp/templates.yaml"
assert_status 0 "$?"
assert_contains "$(cat "$tmp/templates.yaml")" "apiVersion: agentops.dev/v1alpha1"

it "and that mode touched nothing under docs/ either"
after2=$(cd "$ROOT" && git status --porcelain -- docs | sort)
assert_equals "$before" "$after2"

rm -rf "$tmp"

# The `struct-table` marker: a second source of truth (a Go struct, not a CRD)
# for the work-unit/report shapes `docs/contracts/runtime.md` states as law.
# This was tried once already by `runtime-specification` and reversed in the
# same change: a Go doc comment copied verbatim produced unreadable table
# cells. So this exercises the two things that matter about THIS attempt
# rather than that one — fields read correctly off json tags, and `--check`'s
# own staleness comparison (`produced != path.read_text()`) actually catches a
# struct that moved — against a throwaway fixture, never the real
# dispatch.go/server.go (which --check already covers once a page carries the
# marker).
pytmp=$(mktemp -d)
cat > "$pytmp/check_struct_table.py" <<'PYEOF'
import importlib.util
import pathlib
import sys

repo = pathlib.Path(sys.argv[1])
spec = importlib.util.spec_from_file_location(
    "docs_generate", repo / ".github/scripts/docs-generate.py"
)
mod = importlib.util.module_from_spec(spec)
sys.modules["docs_generate"] = mod
spec.loader.exec_module(mod)

tmp = pathlib.Path(sys.argv[2])
go_file = tmp / "fixture.go"
go_file.write_text(
    "package fixture\n\n"
    "type Sample struct {\n"
    "\t// Name is the sample's own name, never blank.\n"
    "\tName string `json:\"name\"`\n"
    "\t// Count is optional, omitted when zero.\n"
    "\tCount int32 `json:\"count,omitempty\"`\n"
    "\t// Legacy is kept for a deployment with no explicit wire name.\n"
    "\tLegacy string `json:\",omitempty\"`\n"
    "\t// Internal carries no struct tag at all.\n"
    "\tInternal string\n"
    "}\n"
)

rows = mod.go_struct_fields(go_file, "Sample")
want = [
    {"path": "name", "type": "string", "required": True,
     "description": "The sample's own name, never blank."},
    {"path": "count", "type": "integer", "required": False,
     "description": "Optional, omitted when zero."},
    {"path": "Legacy", "type": "string", "required": False,
     "description": "Kept for a deployment with no explicit wire name."},
]
assert rows == want, f"fields read incorrectly: {rows}"
print("ok: json tags, Go types and doc comments read correctly")
print("ok: an empty tag name keeps the Go field name instead of being dropped")
print("ok: a field with no struct tag at all is skipped, not a parse error")

doc = tmp / "runtime-test.md"
marker = f"<!-- generated: struct-table file={go_file} struct=Sample -->\n"
# The dispatcher owns the block's exact shape (it inserts a blank line so
# kramdown sees a table), so the canonical text comes from rewrite() itself,
# never from hand-assembling marker + body.
doc.write_text(marker + "<!-- /generated -->\n")
doc.write_text(mod.rewrite(doc, {}))

fresh = mod.rewrite(doc, {})
assert fresh == doc.read_text(), "freshly generated content must already be up to date"
print("ok: --check reports no drift immediately after generating")

original = go_file.read_text()
mutations = {
    "a field added with no regeneration": original.replace(
        "}\n", "\t// Extra is new and nobody ran the generator for it.\n"
        "\tExtra string `json:\"extra,omitempty\"`\n}\n"
    ),
    "a field removed with no regeneration": original.replace(
        "\t// Count is optional, omitted when zero.\n"
        "\tCount int32 `json:\"count,omitempty\"`\n", ""
    ),
    "a field's wire name renamed with no regeneration": original.replace(
        'json:"count,omitempty"', 'json:"amount,omitempty"'
    ),
}
for label, mutated in mutations.items():
    assert mutated != original, f"{label}: fixture did not actually change"
    go_file.write_text(mutated)
    stale = mod.rewrite(doc, {})
    assert stale != doc.read_text(), f"--check must flag drift: {label}"
    go_file.write_text(original)
    print(f"ok: --check catches {label}")
PYEOF

it "struct-table: fields are read off json tags, types and doc comments correctly"
out=$(python3 "$pytmp/check_struct_table.py" "$ROOT" "$pytmp" 2>&1)
assert_status 0 "$?"
assert_contains "$out" "json tags, Go types and doc comments read correctly"

it "struct-table: an empty tag name keeps the Go field name instead of being dropped"
assert_contains "$out" "an empty tag name keeps the Go field name instead of being dropped"

it "struct-table: a field with no struct tag at all is skipped, not a parse error"
assert_contains "$out" "a field with no struct tag at all is skipped, not a parse error"

it "struct-table: --check's own staleness comparison catches an added field"
assert_contains "$out" "catches a field added with no regeneration"

it "struct-table: --check's own staleness comparison catches a removed field"
assert_contains "$out" "catches a field removed with no regeneration"

it "struct-table: --check's own staleness comparison catches a renamed field"
assert_contains "$out" "catches a field's wire name renamed with no regeneration"

rm -rf "$pytmp"

# The per-CRD page machinery: the two block kinds the spec pages carry
# (`crd-example`, `crd-fields`), the page-set check (docs/crds/ and
# chart/crds/ must agree, scaffolding excluded by name, a stale exclusion
# loud) and the diagram edge-label check. All against a FAKE schema dict and
# throwaway dirs -- the real pages are covered by --check at the top.
crdtmp=$(mktemp -d)
cat > "$crdtmp/check_crd_blocks.py" <<'PYEOF'
import importlib.util
import pathlib
import sys

repo = pathlib.Path(sys.argv[1])
spec = importlib.util.spec_from_file_location(
    "docs_generate", repo / ".github/scripts/docs-generate.py"
)
mod = importlib.util.module_from_spec(spec)
sys.modules["docs_generate"] = mod
spec.loader.exec_module(mod)

tmp = pathlib.Path(sys.argv[2])

schemas = {
    "Widget": {"properties": {"spec": {
        "type": "object",
        "required": ["pipelineRef"],
        "properties": {
            "pipelineRef": {"type": "string", "description":
                "PipelineRef names the Pipeline this widget serves; never blank."},
            "replicas": {"type": "integer", "default": 1, "description":
                "Replicas is how many to run. A second sentence stays off the table."},
            "agents": {"type": "array", "items": {
                "type": "object",
                "required": ["capabilityRef"],
                "properties": {
                    "capabilityRef": {"type": "string", "description": "CapabilityRef names one."},
                    "mode": {"type": "string", "enum": ["fan", "serial"],
                             "description": "Mode picks the dispatch shape."},
                },
            }},
            "tokenFrom": {"type": "object", "description":
                "TokenFrom resolves the credential.", "properties": {
                "configMapKeyRef": {"type": "object"}, "fieldRef": {"type": "object"},
                "resourceFieldRef": {"type": "object"}, "secretKeyRef": {"type": "object"},
            }},
        },
    }}},
    "Gizmo": {"properties": {"spec": {
        "type": "object",
        "properties": {"key": {"type": "string", "description": "Key is scaffolding."}},
    }}},
}

example = mod.build_crd_example(schemas, "Widget", "my-widget")
assert "kind: Widget" in example, example
assert "name: my-widget" in example, example
assert "pipelineRef" in example and "replicas" in example, example
assert "#" in example, f"no trailing comments: {example}"
assert ";" not in example, f"a doc comment's semicolon reached the example: {example}"
print("ok: crd-example emits the kind, the name, every top-level spec field and a comment per field")

table = mod.build_crd_fields(schemas, "Widget")
assert table.splitlines()[0] == "| Field | Type | Required | Meaning |", table
assert "| `pipelineRef` | `string` | **yes** |" in table, table
assert "names the Pipeline this widget serves." in table, table
assert "| `replicas` | `integer` |  | How many to run. Defaults to `1`. |" in table, table
assert "never blank" not in table, f"the semicolon tail reached the cell: {table}"
assert mod.build_crd_fields(schemas, "Gizmo"), "an excluded kind's builder still works"
print("ok: crd-fields emits the header, required mark and the semicolon-trimmed first sentence")

assert "| `agents[].capabilityRef` | `string` | **yes** | CapabilityRef names one. |" in table, table
assert "One of `fan`, `serial`." in table, table
print("ok: crd-fields unrolls a nested shape as dotted paths, enums and defaults on the row")

assert "`configMapKeyRef`, `secretKeyRef`, `fieldRef` or `resourceFieldRef`" in table, table
assert "tokenFrom.configMapKeyRef" not in table, (
    f"a recognized core-Kubernetes shape must stop the walk: {table}"
)
print("ok: crd-fields states a core-Kubernetes shape's choices on one row instead of unrolling it")

doc = tmp / "widget.md"
doc.write_text(
    "<!-- generated: crd-fields kind=Widget -->\n<!-- /generated -->\n"
)
doc.write_text(mod.rewrite(doc, schemas))
text = doc.read_text()
assert "-->\n\n| Field" in text, f"no blank line after the marker, kramdown sees a paragraph: {text}"
assert "|\n\n<!-- /generated -->" in text, (
    f"no blank line before the close marker, kramdown paragraphs the table: {text}"
)
assert mod.rewrite(doc, schemas) == text, "a second rewrite must be a no-op"
print("ok: the dispatcher blank-lines the table away from both markers, idempotently")

pages = tmp / "crds"
pages.mkdir()
mod.CRD_PAGES = pages
mod.SCAFFOLDING_KINDS = {"Gizmo"}

(pages / "widget.md").write_text("x\n")
assert mod.check_crd_pages(schemas) == [], mod.check_crd_pages(schemas)
print("ok: page-set check passes when every adopter-facing kind has its page and no more")

(pages / "widget.md").unlink()
probs = mod.check_crd_pages(schemas)
assert any("Widget has no page" in p for p in probs), probs
print("ok: page-set check fails on a kind with no page")

(pages / "widget.md").write_text("x\n")
(pages / "gizmo.md").write_text("x\n")
probs = mod.check_crd_pages(schemas)
assert any("scaffolding the site excludes" in p for p in probs), probs
(pages / "gizmo.md").unlink()
print("ok: page-set check fails on a page for a kind the site excludes")

(pages / "doodad.md").write_text("x\n")
probs = mod.check_crd_pages(schemas)
assert any("matches no kind" in p for p in probs), probs
(pages / "doodad.md").unlink()
print("ok: page-set check fails on a page matching no kind at all")

mod.SCAFFOLDING_KINDS = {"Ghost"}
probs = mod.check_crd_pages(schemas)
assert any("remove it from SCAFFOLDING_KINDS" in p for p in probs), probs
mod.SCAFFOLDING_KINDS = {"Gizmo"}
print("ok: a stale exclusion naming no CRD fails rather than rotting silently")

svg = tmp / "domain-model.svg"
mod.DOMAIN_MODEL = svg
mod.REPO = tmp  # the check prints the path relative to the repo root
svg.write_text('<svg><text data-field="pipelineRef">x</text>'
               '<text data-field="agents[].capabilityRef">x</text></svg>\n')
assert mod.check_diagram_fields(schemas) == [], mod.check_diagram_fields(schemas)
svg.write_text('<svg><text data-field="renamedRef">x</text></svg>\n')
probs = mod.check_diagram_fields(schemas)
assert any("renamedRef" in p and "no CRD spec" in p for p in probs), probs
svg.write_text('<svg><text data-field="pipelineRef.replicas">x</text></svg>\n')
probs = mod.check_diagram_fields(schemas)
assert probs, "a path stitched from real names of different fields must fail"
print("ok: diagram edge labels pass on real paths and fail on renamed or stitched ones")
PYEOF

it "crd-example: kind, name, every top-level spec field, one comment per field, no semicolons"
out=$(python3 "$crdtmp/check_crd_blocks.py" "$ROOT" "$crdtmp" 2>&1)
assert_status 0 "$?"
assert_contains "$out" "crd-example emits the kind, the name, every top-level spec field"

it "crd-fields: header, required mark, semicolon-trimmed first sentence"
assert_contains "$out" "crd-fields emits the header, required mark and the semicolon-trimmed first sentence"

it "crd-fields: nested shapes unroll as dotted paths with enums and defaults"
assert_contains "$out" "unrolls a nested shape as dotted paths"

it "crd-fields: a core-Kubernetes shape is one row stating its choices, not a subtree"
assert_contains "$out" "states a core-Kubernetes shape's choices on one row"

it "the dispatcher blank-lines the table away from both markers, idempotently"
assert_contains "$out" "blank-lines the table away from both markers, idempotently"

it "page-set check: passes when docs/crds/ and chart/crds/ agree"
assert_contains "$out" "passes when every adopter-facing kind has its page"

it "page-set check: a kind with no page fails"
assert_contains "$out" "fails on a kind with no page"

it "page-set check: a page for an excluded scaffolding kind fails"
assert_contains "$out" "fails on a page for a kind the site excludes"

it "page-set check: a page matching no kind fails"
assert_contains "$out" "fails on a page matching no kind at all"

it "page-set check: a stale exclusion fails loudly"
assert_contains "$out" "a stale exclusion naming no CRD fails"

it "diagram edge labels: real field passes, renamed field fails"
assert_contains "$out" "pass on real paths and fail on renamed or stitched ones"

rm -rf "$crdtmp"
summary
