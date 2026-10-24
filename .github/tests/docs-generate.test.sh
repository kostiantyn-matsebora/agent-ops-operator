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
# stale` branch of main()). Together they walk load_crds, build_reference,
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

it "--check reports the generated docs, CRD reference and every diagram as up to date against this checkout"
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
body = mod.build_struct_table(str(go_file), "Sample")
doc.write_text(marker + body + "<!-- /generated -->\n")

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
summary
