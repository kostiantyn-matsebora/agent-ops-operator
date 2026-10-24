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
{"type":"system","subtype":"task_started","task_id":"w1"}
{"type":"system","subtype":"task_progress","usage":{"duration_ms":1000},"workflow_progress":[{"type":"workflow_agent","index":1,"label":"docs/a.md","state":"start","startedAt":5000}]}
{"type":"system","subtype":"task_progress","usage":{"duration_ms":61000},"workflow_progress":[{"type":"workflow_agent","index":1,"label":"docs/a.md","state":"done","startedAt":5000},{"type":"workflow_agent","index":2,"label":"docs/b.md","state":"start","startedAt":65000}]}
{"type":"system","subtype":"task_progress","usage":{"duration_ms":150000},"workflow_progress":[{"type":"workflow_agent","index":1,"label":"docs/a.md","state":"done","startedAt":5000},{"type":"workflow_agent","index":2,"label":"docs/b.md","state":"done","startedAt":65000}]}
{"type":"system","subtype":"permission_denied","tool_name":"Bash"}
{"type":"result","subtype":"success","num_turns":1,"duration_ms":4000,"duration_api_ms":3000,"usage":{},"structured_output":{"component":"docs","findings":[],"changedNames":[],"threads":[],"invented":true}}
{"type":"system","subtype":"task_notification","task_id":"w1","status":"completed"}
{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}
{"type":"result","subtype":"success","num_turns":2,"duration_ms":160000,"duration_api_ms":150000,"usage":{"input_tokens":10,"cache_read_input_tokens":2000,"cache_creation_input_tokens":500,"output_tokens":300},"structured_output":{"component":"docs","findings":[],"changedNames":["X"],"threads":[]}}
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

it "hands the result that FOLLOWED the workflow's completion to the validator, and discards one invented before it"
assert_contains "$(cat "$tmp/r.json")" '"X"'
assert_not_contains "$(cat "$tmp/r.json")" '"invented"'
assert_contains "$out" "::warning::1 result(s) before the workflow completed were discarded"
assert_status 0 "$(python3 "$ROOT/.github/scripts/review-reading-check.py" "$tmp/r.json" --group docs --out "$tmp/reading.json" >/dev/null 2>&1; echo $?)"

it "a session that ran no workflow (the coordinator) keeps its last result"
cat > "$tmp/plain.jsonl" <<'EOF'
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{}}]}}
{"type":"result","subtype":"success","num_turns":3,"duration_ms":5000,"duration_api_ms":4000,"usage":{},"result":"{\"summaryPosted\": true}"}
EOF
out=$(python3 "$S" "$tmp/plain.jsonl" --out "$tmp/plain-out.json")
assert_contains "$out" "turns 3"
assert_not_contains "$out" "discarded"
assert_contains "$(cat "$tmp/plain-out.json")" "summaryPosted"

it "a record whose workflow never completed yields no reading, whatever the session answered"
{ echo '{"type":"system","subtype":"task_started","task_id":"w1"}'; grep -v task_notification "$tmp/exec.jsonl"; } > "$tmp/partial.jsonl"
out=$(python3 "$S" "$tmp/partial.jsonl" --out "$tmp/p.json")
assert_contains "$out" "NO RESULT AFTER THE WORKFLOW COMPLETED"
assert_equals "{}" "$(cat "$tmp/p.json")"

rm -rf "$tmp"
summary
