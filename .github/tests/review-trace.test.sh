#!/usr/bin/env bash
# The program that says where a model step's time went, from its execution
# record — and hands the final result to the validator.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/review-trace.py"
tmp=$(mktemp -d)

cat > "$tmp/exec.jsonl" <<'EOF'
{"type":"system","subtype":"init"}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Workflow","input":{}}]}}
{"type":"system","subtype":"task_progress","usage":{"duration_ms":1000},"workflow_progress":[{"type":"workflow_agent","index":1,"label":"docs/a.md","state":"start","startedAt":5000}]}
{"type":"system","subtype":"task_progress","usage":{"duration_ms":61000},"workflow_progress":[{"type":"workflow_agent","index":1,"label":"docs/a.md","state":"done","startedAt":5000},{"type":"workflow_agent","index":2,"label":"docs/b.md","state":"start","startedAt":65000}]}
{"type":"system","subtype":"task_progress","usage":{"duration_ms":150000},"workflow_progress":[{"type":"workflow_agent","index":1,"label":"docs/a.md","state":"done","startedAt":5000},{"type":"workflow_agent","index":2,"label":"docs/b.md","state":"done","startedAt":65000}]}
{"type":"system","subtype":"permission_denied","tool_name":"Bash"}
{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}
{"type":"result","subtype":"success","num_turns":2,"duration_ms":160000,"duration_api_ms":150000,"usage":{"input_tokens":10,"cache_read_input_tokens":2000,"cache_creation_input_tokens":500,"output_tokens":300},"structured_output":{"component":"docs","findings":[],"changedNames":[],"threads":[]}}
EOF

it "reports the session: turns, wall, api, tokens, tool calls, denials"
out=$(python3 "$S" "$tmp/exec.jsonl" --out "$tmp/r.json")
assert_contains "$out" "turns 2"
assert_contains "$out" "wall 160s; api 150s"
assert_contains "$out" "cache read 2000"
assert_contains "$out" "{'Workflow': 1}"
assert_contains "$out" "permission denials 1"

it "reports each workflow agent's start and duration, from when its state first read done"
assert_contains "$out" "docs/a.md"
assert_contains "$out" "start    +0s  dur   61s"
assert_contains "$out" "start   +60s  dur   90s"

it "hands the final result to the validator"
assert_contains "$(cat "$tmp/r.json")" '"structured_output"'
assert_status 0 "$(python3 "$ROOT/.github/scripts/review-reading-check.py" "$tmp/r.json" --group docs --out "$tmp/reading.json" >/dev/null 2>&1; echo $?)"

it "a record with no result says so, and writes an empty envelope"
head -3 "$tmp/exec.jsonl" > "$tmp/partial.jsonl"
out=$(python3 "$S" "$tmp/partial.jsonl" --out "$tmp/p.json")
assert_contains "$out" "NO RESULT EVENT"
assert_equals "{}" "$(cat "$tmp/p.json")"

rm -rf "$tmp"
summary
