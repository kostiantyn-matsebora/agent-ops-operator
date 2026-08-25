#!/usr/bin/env bash
# Shared helpers for the script tests.
#
# THESE SCRIPTS DECIDE THINGS, WHICH IS WHY THEY NEED TESTS. Between them they
# refuse commits, refuse to resolve somebody's review thread, and open GitHub
# issues. Each was verified once, by hand, in a directory that no longer exists —
# which proves it worked that afternoon and catches nothing afterwards.
#
# NO NETWORK. `gh` is stubbed, so a test never opens an issue, never resolves a
# thread, and cannot be made to by a mistake in the code under test. A suite that
# reached GitHub would be one nobody dares run.

set -uo pipefail

TESTS_RUN=0
TESTS_FAILED=0
CURRENT=""

# --- assertions -------------------------------------------------------------

it() { CURRENT="$1"; TESTS_RUN=$((TESTS_RUN + 1)); }

pass() { printf '    ok    %s\n' "$CURRENT"; }
fail() {
  printf '    FAIL  %s\n' "$CURRENT"
  printf '          %s\n' "$1"
  TESTS_FAILED=$((TESTS_FAILED + 1))
}

assert_status() {  # assert_status <expected> <actual>
  [ "$1" = "$2" ] && pass || fail "expected exit $1, got $2"
}

assert_contains() {  # assert_contains <haystack> <needle>
  case "$1" in
    *"$2"*) pass ;;
    *) fail "expected output to contain: $2" ;;
  esac
}

assert_not_contains() {
  case "$1" in
    *"$2"*) fail "expected output NOT to contain: $2" ;;
    *) pass ;;
  esac
}

assert_equals() {
  [ "$1" = "$2" ] && pass || fail "expected [$1], got [$2]"
}

# --- a repository to act on -------------------------------------------------

# A throwaway git repository with the shape these scripts expect. Never the real
# one: several of them stage files, and this repository's own rule is that other
# sessions have uncommitted work in the shared checkout.
make_repo() {
  local dir; dir=$(mktemp -d)
  git -C "$dir" init -q -b master
  git -C "$dir" config user.email test@example.com
  git -C "$dir" config user.name "Test"
  mkdir -p "$dir/openspec/changes" "$dir/.github/scripts" "$dir/.claude/hooks"
  echo seed > "$dir/README.md"
  git -C "$dir" add README.md
  git -C "$dir" commit -qm seed
  echo "$dir"
}

make_change() {  # make_change <repo> <name> [headline]
  local dir="$1/openspec/changes/$2"
  mkdir -p "$dir"
  printf 'schema: spec-driven\ncreated: 2026-01-01\n' > "$dir/.openspec.yaml"
  printf '# %s\n\n## Why\n\nbecause\n' "${3:-A change called $2}" > "$dir/proposal.md"
}

# --- a `gh` that answers without a network ----------------------------------
#
# Records every invocation to $GH_CALLS so a test can assert what WOULD have been
# sent, and answers from $GH_FIXTURE when a canned reply is needed.
stub_gh() {  # stub_gh <bindir>
  local bin="$1"; mkdir -p "$bin"
  cat > "$bin/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${GH_CALLS:-/dev/null}"
case "$*" in
  "issue create"*)  echo "https://github.com/o/r/issues/${GH_NEW_ISSUE:-101}" ;;
  "issue view"*)    cat "${GH_FIXTURE:-/dev/null}" 2>/dev/null || echo '{}' ;;
  "api graphql"*)   cat "${GH_FIXTURE:-/dev/null}" 2>/dev/null || echo '{"data":{}}' ;;
  *)                : ;;
esac
exit "${GH_EXIT:-0}"
STUB
  chmod +x "$bin/gh"
  export PATH="$bin:$PATH"
}

summary() {
  echo
  if [ "$TESTS_FAILED" -gt 0 ]; then
    echo "  $TESTS_FAILED of $TESTS_RUN failed"
    return 1
  fi
  echo "  $TESTS_RUN passed"
  return 0
}
