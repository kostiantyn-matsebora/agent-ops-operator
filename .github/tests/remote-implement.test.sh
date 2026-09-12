#!/usr/bin/env bash
# A LABEL STARTS A MACHINE WRITING TO THIS REPOSITORY, so who may place it is a
# decision, not a transformation — and decisions are what these suites exist
# for. Three properties, each of which somebody could simplify away:
#
#   the gate    anyone with TRIAGE may label an issue, and triage is NOT write.
#               A gate reading the label rather than the labeller would let a
#               drive-by start a session.
#   the record  the fire's session link is posted once. A second comment reads
#               as two sessions racing on one issue.
#   the payload the number and nothing else. The platform wraps fire text as
#               untrusted; sending a stranger's title and body would put their
#               prose where the routine's prompt is.
#
# NO NETWORK: `gh` is stubbed and the fire endpoint is a local file-backed stub
# reached through --fire-url, so a run can neither comment nor start anything.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/remote-implement.py"

# A `gh` that answers a permission question and records every call.
stub_gh_perm() {  # stub_gh_perm <bindir> <permission>
  local bin="$1"; mkdir -p "$bin"
  cat > "$bin/gh" <<STUB
#!/usr/bin/env bash
printf '%s\n' "\$*" >> "\$GH_CALLS"
case "\$*" in
  *"collaborators/"*"/permission"*) echo "$2" ;;
  *"issues/"*"/comments"*)          cat "\${GH_COMMENTS:-/dev/null}" 2>/dev/null || true ;;
  *) : ;;
esac
exit 0
STUB
  chmod +x "$bin/gh"
}

# The fire endpoint, as a file the program POSTs to over http. `python3 -m
# http.server` cannot answer a POST, so this is a 20-line handler.
start_endpoint() {  # start_endpoint <status> <body-file> <record-file>
  ENDPOINT_PORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
  python3 - "$1" "$2" "$3" "$ENDPOINT_PORT" <<'PY' &
import http.server, json, sys
status, body_file, record, port = int(sys.argv[1]), sys.argv[2], sys.argv[3], int(sys.argv[4])

class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        n = int(self.headers.get("content-length") or 0)
        payload = self.rfile.read(n).decode()
        with open(record, "w") as f:
            json.dump({"body": payload, "headers": dict(self.headers)}, f)
        self.send_response(status)
        self.send_header("content-type", "application/json")
        self.end_headers()
        self.wfile.write(open(body_file, "rb").read())
    def log_message(self, *a): pass

http.server.HTTPServer(("127.0.0.1", port), H).serve_forever()
PY
  ENDPOINT_PID=$!
  for _ in $(seq 50); do
    (exec 3<>/dev/tcp/127.0.0.1/"$ENDPOINT_PORT") 2>/dev/null && break
    sleep 0.1
  done
}
stop_endpoint() { [ -n "${ENDPOINT_PID:-}" ] && kill "$ENDPOINT_PID" 2>/dev/null; wait "$ENDPOINT_PID" 2>/dev/null; ENDPOINT_PID=""; }
trap stop_endpoint EXIT

event() {  # event <file> <label> <number> <sender> [pull_request] [extra_label]
  local pr=""; [ -n "${5:-}" ] && pr=', "pull_request": {"url": "u"}'
  # THE ISSUE'S OWN labels[] CARRIES AT LEAST THE FIRED LABEL, as GitHub's
  # real payload does -- an OPTIONAL sixth arg adds a second, for the case
  # where a standing instruction already sits on the issue beside whichever
  # label just fired this event.
  local labels="{\"name\": \"$2\"}"
  [ -n "${6:-}" ] && labels="$labels, {\"name\": \"$6\"}"
  cat > "$1" <<JSON
{"action": "labeled",
 "label": {"name": "$2"},
 "issue": {"number": $3, "title": "A thing that is broken", "labels": [$labels]$pr},
 "sender": {"login": "$4"}}
JSON
}

setup() {
  DIR=$(mktemp -d); BIN="$DIR/bin"; export GH_CALLS="$DIR/calls"; : > "$GH_CALLS"
  EVENT="$DIR/event.json"; RECORD="$DIR/fired.json"; BODY="$DIR/reply.json"
  echo '{"session_url": "https://claude.ai/code/session_abc"}' > "$BODY"
}

run_it() {  # run_it [extra args]
  PATH="$BIN:$PATH" GITHUB_REPOSITORY=o/r ROUTINE_FIRE_TOKEN="${TOKEN-tok}" \
    python3 "$S" --event "$EVENT" --repo o/r "$@" 2>&1
}

# --- the wrong label ---------------------------------------------------------

it "another label does nothing at all: this workflow sees every label event"
setup; stub_gh_perm "$BIN" admin
event "$EVENT" "bug" 7 someone
out=$(run_it --fire-url "http://127.0.0.1:1/never"); status=$?
assert_status 0 "$status"
assert_equals "" "$(cat "$GH_CALLS")"

# --- the gate ----------------------------------------------------------------

for perm in read triage none; do
  for label in conveyor:implement conveyor:run; do
    it "a $perm labeller placing $label starts nothing: the label comes off and one comment says who may place it"
    setup; stub_gh_perm "$BIN" "$perm"
    event "$EVENT" "$label" 7 stranger
    out=$(run_it --fire-url "http://127.0.0.1:1/never"); status=$?
    assert_status 1 "$status"
    assert_contains "$(cat "$GH_CALLS")" "issue edit 7 --repo o/r --remove-label $label"
    assert_contains "$(cat "$GH_CALLS")" "issue comment 7"
    assert_not_contains "$out" "fired"
  done
done

it "a pull request carrying the label fires nothing: it is an issue to that API, not a request to implement"
setup; stub_gh_perm "$BIN" admin
event "$EVENT" conveyor:implement 7 owner pr
out=$(run_it --fire-url "http://127.0.0.1:1/never"); status=$?
assert_status 0 "$status"
assert_not_contains "$(cat "$GH_CALLS")" "issue comment"

# --- the fire ----------------------------------------------------------------

it "a writer's label fires once, carrying the NUMBER and the beta header, and comments the session link"
setup; stub_gh_perm "$BIN" write
event "$EVENT" conveyor:implement 42 maintainer
start_endpoint 200 "$BODY" "$RECORD"
out=$(run_it --fire-url "http://127.0.0.1:$ENDPOINT_PORT/fire"); status=$?
stop_endpoint
assert_status 0 "$status"
assert_equals '{"text": "42"}' "$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["body"])' "$RECORD")"
assert_contains "$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["headers"])' "$RECORD")" "experimental-cc-routine"
assert_contains "$(cat "$GH_CALLS")" "issue comment 42"
assert_contains "$(cat "$GH_CALLS")" "https://claude.ai/code/session_abc"
# THE SESSION PLACES NO LABEL ON ITS OWN WORK -- #201's fix. conveyor:implement
# is a single-station label, so the comment says a PERSON places the next
# one, never that a workflow carries the grant forward (that only happens
# when the fire itself was conveyor:run -- see the next test).
assert_contains "$(cat "$GH_CALLS")" "unlabelled"
assert_contains "$(cat "$GH_CALLS")" "a person places a label to start the fixing loop"
assert_not_contains "$(cat "$GH_CALLS")" "a workflow reads"

# conveyor:implement CAN FIRE WHILE conveyor:run ALREADY STANDS -- the later
# carry (carry-grant.py, at the `open` job) reads the ISSUE'S CURRENT labels,
# never which one fired this session, so the standing instruction still gets
# carried forward even though conveyor:implement, not conveyor:run, is what
# triggered this particular run. The comment must say so.
it "conveyor:implement fires while conveyor:run already stands on the issue, and the comment says the grant WILL be carried"
setup; stub_gh_perm "$BIN" write
event "$EVENT" conveyor:implement 44 maintainer "" conveyor:run
start_endpoint 200 "$BODY" "$RECORD"
out=$(run_it --fire-url "http://127.0.0.1:$ENDPOINT_PORT/fire"); status=$?
stop_endpoint
assert_status 0 "$status"
assert_contains "$(cat "$GH_CALLS")" "issue comment 44"
assert_contains "$(cat "$GH_CALLS")" "carries it forward as"
assert_not_contains "$(cat "$GH_CALLS")" "a person places a label to start the fixing loop"
# THE TEXT MUST NAME conveyor:run, THE LABEL THAT ACTUALLY GETS CARRIED --
# never conveyor:implement, the one that happened to fire this session.
# carry-grant.py reads run_label off the issue's live labels regardless of
# which label fired remote-implement.py, so naming the firing label here
# would describe a carry that is not the one that actually happens.
assert_contains "$(cat "$GH_CALLS")" "reads \`conveyor:run\` again and carries it forward as \`conveyor:fix\`"
assert_contains "$(cat "$GH_CALLS")" "a workflow carries \`conveyor:run\` forward again to archive"
assert_not_contains "$(cat "$GH_CALLS")" "reads \`conveyor:implement\` again"
assert_not_contains "$(cat "$GH_CALLS")" "carries \`conveyor:implement\` forward again to archive"

# conveyor:run FIRES THE SAME SESSION, and its comment says the standing
# instruction is what a workflow reads again later to carry the grant forward.
it "a writer's conveyor:run fires the same session, and says the grant will be CARRIED forward, not placed now"
setup; stub_gh_perm "$BIN" write
event "$EVENT" conveyor:run 43 maintainer
start_endpoint 200 "$BODY" "$RECORD"
out=$(run_it --fire-url "http://127.0.0.1:$ENDPOINT_PORT/fire"); status=$?
stop_endpoint
assert_status 0 "$status"
assert_equals '{"text": "43"}' "$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["body"])' "$RECORD")"
assert_contains "$(cat "$GH_CALLS")" "issue comment 43"
assert_contains "$(cat "$GH_CALLS")" "carries it forward as"
assert_contains "$(cat "$GH_CALLS")" "conveyor:fix"

it "another conveyor: label (conveyor:fix, which belongs to the fixing loop, not this program) does nothing"
setup; stub_gh_perm "$BIN" admin
event "$EVENT" "conveyor:fix" 7 someone
out=$(run_it --fire-url "http://127.0.0.1:1/never"); status=$?
assert_status 0 "$status"
assert_equals "" "$(cat "$GH_CALLS")"

it "the comment carries the marker, so a second run does not post a second one"
setup; stub_gh_perm "$BIN" write
event "$EVENT" conveyor:implement 42 maintainer
echo '<!-- remote-implement:fired -->' > "$DIR/comments"
GH_COMMENTS="$DIR/comments"; export GH_COMMENTS
out=$(run_it --fire-url "http://127.0.0.1:1/never"); status=$?
unset GH_COMMENTS
assert_status 0 "$status"
assert_not_contains "$(cat "$GH_CALLS")" "issue comment"

# AN UNREADABLE COMMENT LIST FAILS CLOSED. Answering "not fired" on a rate
# limit or a dropped connection starts a SECOND session on the same issue, which
# is the case the marker exists to prevent; refusing is recoverable by
# re-labelling, a duplicate run is not.
it "a comments read that FAILS is treated as already fired, never as a licence to fire again"
setup; mkdir -p "$BIN"
cat > "$BIN/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$GH_CALLS"
case "$*" in
  *"collaborators/"*"/permission"*) echo "admin" ;;
  *"issues/"*"/comments"*) echo "the comments API is unavailable" >&2; exit 1 ;;
esac
exit 0
STUB
chmod +x "$BIN/gh"
event "$EVENT" conveyor:implement 42 maintainer
out=$(run_it --fire-url "http://127.0.0.1:1/never"); status=$?
assert_status 0 "$status"
assert_not_contains "$out" "fired for"
assert_contains "$out" "already fired"

# --- the endpoint says no ----------------------------------------------------

it "a 5xx from the endpoint comments the status and fails the job, rather than failing silently"
setup; stub_gh_perm "$BIN" admin
event "$EVENT" conveyor:implement 9 owner
start_endpoint 503 "$BODY" "$RECORD"
out=$(run_it --fire-url "http://127.0.0.1:$ENDPOINT_PORT/fire"); status=$?
stop_endpoint
assert_status 1 "$status"
assert_contains "$(cat "$GH_CALLS")" "issue comment 9"
assert_contains "$(cat "$GH_CALLS")" "503"

it "no endpoint configured says so on the issue instead of starting nothing quietly"
setup; stub_gh_perm "$BIN" admin
event "$EVENT" conveyor:implement 9 owner
out=$(PATH="$BIN:$PATH" GITHUB_REPOSITORY=o/r python3 "$S" --event "$EVENT" --repo o/r --fire-url "" 2>&1); status=$?
assert_status 1 "$status"
assert_contains "$(cat "$GH_CALLS")" "no routine"

# A COPIED URL CARRIES A NEWLINE, AND THAT REALLY HAPPENED. The first live fire
# crashed in http.client with `URL can't contain control characters` — a stack
# trace on the runner and nothing on the issue, where a person would look.
it "a fire url carrying an EMBEDDED newline is refused with one line, not a stack trace"
setup; stub_gh_perm "$BIN" admin
event "$EVENT" conveyor:implement 42 maintainer
# THE SHAPE THAT ACTUALLY HAPPENED: `gh variable set` stored a value copied out
# of a wrapped display, so the break sits INSIDE the id rather than at the end.
# `.strip()` cannot help there — http.client raises InvalidURL from four frames
# down, and the issue gets nothing.
out=$(run_it --fire-url "https://api.anthropic.com/v1/routines/trig_01ABC
DEF/fire"); status=$?
assert_status 1 "$status"
assert_not_contains "$out" "Traceback"
assert_contains "$(cat "$GH_CALLS")" "issue comment 42"

it "a fire url that is not a url at all is REFUSED on the issue, never a stack trace"
setup; stub_gh_perm "$BIN" admin
event "$EVENT" conveyor:implement 42 maintainer
out=$(run_it --fire-url "trig_01ABC/fire"); status=$?
assert_status 1 "$status"
assert_contains "$(cat "$GH_CALLS")" "issue comment 42"
assert_contains "$out" "not a URL"
assert_not_contains "$out" "Traceback"

# THE TOKEN BREAKS THE SAME WAY AND READS WORSE. A newline in a header is
# refused like the url's; a token that merely lost characters comes back 401,
# indistinguishable from a revoked credential.
it "a token carrying a line break is refused before it is sent, and never printed"
setup; stub_gh_perm "$BIN" admin
event "$EVENT" conveyor:implement 42 maintainer
out=$(PATH="$BIN:$PATH" GITHUB_REPOSITORY=o/r ROUTINE_FIRE_TOKEN="$(printf 'sk-ant-oat01-AAA\nBBB')" \
  python3 "$S" --event "$EVENT" --repo o/r --fire-url "http://127.0.0.1:1/fire" 2>&1); status=$?
assert_status 1 "$status"
assert_contains "$out" "ROUTINE_FIRE_TOKEN contains whitespace"
# THE VALUE ITSELF NEVER APPEARS — not in the log, not in the comment.
assert_not_contains "$out" "sk-ant-oat01-AAA"
assert_contains "$(cat "$GH_CALLS")" "issue comment 42"
assert_not_contains "$(cat "$GH_CALLS")" "sk-ant-oat01-AAA"

# --- the payload -------------------------------------------------------------

it "an event whose issue number is not a number never reaches the fire"
setup; stub_gh_perm "$BIN" admin
cat > "$EVENT" <<'JSON'
{"action":"labeled","label":{"name":"conveyor:implement"},
 "issue":{"number":"7; rm -rf /","title":"t"},"sender":{"login":"owner"}}
JSON
out=$(run_it --fire-url "http://127.0.0.1:1/never"); status=$?
assert_status 1 "$status"
assert_equals "" "$(cat "$GH_CALLS")"

summary
