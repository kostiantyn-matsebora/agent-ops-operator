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
summary
