#!/usr/bin/env bash
# A PROGRAM MAY CARRY A GRANT FORWARD OR CONSUME ONE. IT MAY NEVER MINT ONE.
# carry-grant.py is the whole fix for #201: a session's own pull request must
# never carry a label the session placed, so this program relays a WRITER's
# standing instruction instead, re-checked every time it is called.
#
# NO NETWORK: `gh` is stubbed and every case is driven from a throwaway working
# directory (for the lane's sidecar glob) plus canned API answers.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/carry-grant.py"

tmp=$(mktemp -d)
export GH_CALLS="$tmp/calls"

# A `gh` answering exactly what carry-grant.py asks: the issue's labels, the
# timeline's labelling events, the collaborator permission, and the issue's
# (or pull request's) comments -- each from a file this test controls, so a
# case can shape one answer without touching the others.
stub_gh() {  # stub_gh <bindir>
  local bin="$1"; mkdir -p "$bin"
  cat > "$bin/gh" <<STUB
#!/usr/bin/env bash
printf '%s\n' "\$*" >> "\$GH_CALLS"
case "\$*" in
  "issue view "*"--json labels")       cat "\${LABELS_FILE:-/dev/null}" 2>/dev/null || echo '{"labels":[]}' ;;
  "api repos/"*"/timeline --paginate") cat "\${TIMELINE_FILE:-/dev/null}" 2>/dev/null || echo '[]' ;;
  "api repos/"*"/permission"*)         cat "\${PERM_FILE:-/dev/null}" 2>/dev/null || echo 'none' ;;
  "api repos/"*"/comments --paginate --jq .[].body")
                                       cat "\${COMMENTS_FILE:-/dev/null}" 2>/dev/null || printf '' ;;
  *) : ;;
esac
exit 0
STUB
  chmod +x "$bin/gh"
}

setup() {
  DIR=$(mktemp -d); BIN="$DIR/bin"; : > "$GH_CALLS"
  stub_gh "$BIN"
  LABELS_FILE="$DIR/labels.json"; TIMELINE_FILE="$DIR/timeline.json"
  PERM_FILE="$DIR/perm"; COMMENTS_FILE="$DIR/comments"
  echo '{"labels":[]}' > "$LABELS_FILE"
  echo '[]' > "$TIMELINE_FILE"
  echo -n "write" > "$PERM_FILE"
  : > "$COMMENTS_FILE"
  CWD="$DIR/cwd"; mkdir -p "$CWD/openspec/changes"
}

with_run_label() {  # with_run_label <placer> <when>
  printf '{"labels":[{"name":"conveyor:run"}]}' > "$LABELS_FILE"
  printf '[{"event":"labeled","label":{"name":"conveyor:run"},"actor":{"login":"%s"},"created_at":"%s"}]' \
    "$1" "$2" > "$TIMELINE_FILE"
}

mark_opsx() {  # mark_opsx <issue-number>
  mkdir -p "$CWD/openspec/changes/thing"
  printf '%s' "$1" > "$CWD/openspec/changes/thing/.github-issue"
}

run_it() {  # run_it <args...>
  ( cd "$CWD" && \
    LABELS_FILE="$LABELS_FILE" TIMELINE_FILE="$TIMELINE_FILE" PERM_FILE="$PERM_FILE" \
    COMMENTS_FILE="$COMMENTS_FILE" PATH="$BIN:$PATH" GH_CALLS="$GH_CALLS" \
    python3 "$S" --repo o/r "$@" 2>&1 )
}

# --- the argument contract, before anything is read -------------------------

it "--station fix without --pr is refused"
setup
out=$(run_it --issue 1 --station fix); rc=$?
assert_status 2 "$rc"
assert_contains "$out" "requires --pr"

it "--station archive WITH --pr is refused, never silently ignored"
setup
out=$(run_it --issue 1 --station archive --pr 5); rc=$?
assert_status 2 "$rc"
assert_contains "$out" "refuses --pr"

# --- no standing instruction: the ordinary case ------------------------------

it "no standing instruction: exits 0 and places nothing"
setup
mark_opsx 1
out=$(run_it --issue 1 --station fix --pr 5); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "--add-label"
assert_contains "$out" "nothing to carry"

# --- instruction present, placer has write access: places on the RIGHT target

it "instruction present, writer placed it: places conveyor:fix on the PULL REQUEST, once"
setup
mark_opsx 1
with_run_label maintainer 2026-09-01T10:00:00Z
out=$(run_it --issue 1 --station fix --pr 5); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "issue edit 5 --repo o/r --add-label conveyor:fix"
assert_contains "$(cat "$GH_CALLS")" "issue comment 5"
assert_contains "$(cat "$GH_CALLS")" "@maintainer"

it "instruction present, writer placed it, opsx lane: places conveyor:archive on the ISSUE"
setup
mark_opsx 1
with_run_label maintainer 2026-09-01T10:00:00Z
out=$(run_it --issue 1 --station archive); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "issue edit 1 --repo o/r --add-label conveyor:archive"
assert_contains "$(cat "$GH_CALLS")" "issue comment 1"

# --- the placer has since LOST write access ----------------------------------

it "the run_label's placer has lost write access: places nothing and says so"
setup
mark_opsx 1
with_run_label ex-maintainer 2026-09-01T10:00:00Z
printf '%s' "read" > "$PERM_FILE"
out=$(run_it --issue 1 --station fix --pr 5); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "--add-label"
assert_contains "$out" "who now has \`read\`"

# --- already carried: no second comment --------------------------------------

it "already carried (marker present on the target): no second comment"
setup
mark_opsx 1
with_run_label maintainer 2026-09-01T10:00:00Z
printf 'earlier text\n<!-- carry-grant:fix -->\nalready carried' > "$COMMENTS_FILE"
out=$(run_it --issue 1 --station fix --pr 5); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "issue comment 5"
assert_contains "$out" "already carries"

# --- the plain lane has no archive station -----------------------------------

it "--station archive on a PLAIN-lane issue places nothing and says why"
setup
with_run_label maintainer 2026-09-01T10:00:00Z
out=$(run_it --issue 1 --station archive); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "--add-label"
assert_contains "$out" "plain lane"

it "--station archive on the SAME instruction, opsx lane, labels the issue"
setup
mark_opsx 1
with_run_label maintainer 2026-09-01T10:00:00Z
out=$(run_it --issue 1 --station archive); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "issue edit 1 --repo o/r --add-label conveyor:archive"

# --- a pull request body with no Refs #<n> is the CALLER's concern, not
# this program's -- covered by review-dispatch.test.sh's workflow assertions;
# this suite pins the program's OWN no-op cases instead.

it "the LATEST placement counts: an earlier placer with no access is superseded by a later writer"
setup
mark_opsx 1
printf '{"labels":[{"name":"conveyor:run"}]}' > "$LABELS_FILE"
printf '[{"event":"labeled","label":{"name":"conveyor:run"},"actor":{"login":"ex-writer"},"created_at":"2026-08-01T10:00:00Z"},
         {"event":"labeled","label":{"name":"conveyor:run"},"actor":{"login":"maintainer"},"created_at":"2026-09-01T10:00:00Z"}]' \
  > "$TIMELINE_FILE"
out=$(run_it --issue 1 --station fix --pr 5); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "issue edit 5 --repo o/r --add-label conveyor:fix"
assert_contains "$(cat "$GH_CALLS")" "@maintainer"
assert_not_contains "$(cat "$GH_CALLS")" "@ex-writer"

summary
