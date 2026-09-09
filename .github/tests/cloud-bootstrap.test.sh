#!/usr/bin/env bash
# THE BOOTSTRAP DECIDES WHAT A MACHINE GETS INSTALLED ON IT, which is why it is
# tested rather than tried once. Two failures it must never have:
#
#   1. installing anything on a WORKSTATION. The marker is the whole of the
#      remote test, and getting it backwards would run six installers on
#      somebody's own machine.
#   2. failing the session because one download failed. The platform reads a
#      non-zero setup as a failed session, so the script must exit 0 having
#      NAMED what it could not install.
#
# NO NETWORK. Every installer is a stub on PATH that records being called, so a
# run of this suite can neither download nor install anything.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/cloud-bootstrap.sh"

# A PATH holding only what the script may find. `stubs present` puts every tool
# there so no installer has a reason to run; `stubs absent` leaves them out.
make_env() {  # make_env <mode: present|absent>
  BIN=$(mktemp -d); FAKEHOME=$(mktemp -d); CALLS="$FAKEHOME/calls"
  : > "$CALLS"
  for i in curl npm pip uv tar go_install; do
    cat > "$BIN/$i" <<STUB
#!/bin/sh
echo "$i \$*" >> "$CALLS"
exit "\${STUB_EXIT:-0}"
STUB
    chmod +x "$BIN/$i"
  done
  if [ "$1" = present ]; then
    for t in helm openspec serena; do printf '#!/bin/sh\nexit 0\n' > "$BIN/$t"; chmod +x "$BIN/$t"; done
    printf '#!/bin/sh\necho "go version go1.25.1 linux/amd64"\n' > "$BIN/go"; chmod +x "$BIN/go"
    mkdir -p "$FAKEHOME/envtest/k8s/1.31.0-linux-amd64"
    touch "$FAKEHOME/envtest/k8s/1.31.0-linux-amd64/kube-apiserver"
  else
    # Go is present but OLD, so its own section is the one that must act.
    printf '#!/bin/sh\necho "go version go1.23.4 linux/amd64"\n' > "$BIN/go"; chmod +x "$BIN/go"
  fi
}

run_bootstrap() {  # run_bootstrap [args...]
  PATH="$BIN:/usr/bin:/bin" HOME="$FAKEHOME" ENVTEST_DIR="$FAKEHOME/envtest" \
    bash "$S" "$@" 2>&1
}

# --- the workstation, which is the dangerous direction ----------------------

for marker in "" 0 false; do
  make_env absent
  it "the marker ${marker:-<unset>} is a workstation: nothing is installed and it exits 0"
  if [ -z "$marker" ]; then
    out=$(PATH="$BIN:/usr/bin:/bin" HOME="$FAKEHOME" bash "$S" 2>&1); status=$?
  else
    out=$(PATH="$BIN:/usr/bin:/bin" HOME="$FAKEHOME" CLAUDE_CODE_REMOTE="$marker" bash "$S" 2>&1); status=$?
  fi
  assert_status 0 "$status"
  assert_equals "" "$(cat "$CALLS")"
  assert_equals "" "$out"
done

it "--verify on a workstation prints nothing, so a local session's context is untouched"
make_env present
out=$(PATH="$BIN:/usr/bin:/bin" HOME="$FAKEHOME" bash "$S" --verify 2>&1)
assert_equals "" "$out"

# --- remote, everything already there ---------------------------------------

it "remote with every tool present installs nothing: the setup is idempotent across cache rebuilds"
make_env present
out=$(CLAUDE_CODE_REMOTE=1 run_bootstrap); status=$?
assert_status 0 "$status"
assert_equals "" "$(grep -E '^(curl|npm|pip|uv) ' "$CALLS" || true)"
assert_contains "$out" "go1.25.1"

it "the marker true is remote too, not only 1"
make_env present
out=$(CLAUDE_CODE_REMOTE=true run_bootstrap)
assert_contains "$out" "envtest present"

# --- remote, one installer fails --------------------------------------------

it "a failed installer names its tool on stderr, still runs the others, and exits 0"
make_env absent
out=$(CLAUDE_CODE_REMOTE=1 STUB_EXIT=1 run_bootstrap); status=$?
# THE PLATFORM REQUIRES SUCCESS. A session missing one tool beats no session.
assert_status 0 "$status"
assert_contains "$out" "could not install"
assert_contains "$out" "helm"
# EVERY section ran, rather than the first failure ending the script.
assert_contains "$out" "installing @fission-ai/openspec"
assert_contains "$out" "installing serena"

# --- verify ------------------------------------------------------------------

it "--verify installs nothing and answers present or missing for every tool"
make_env absent
out=$(CLAUDE_CODE_REMOTE=1 run_bootstrap --verify); status=$?
assert_status 0 "$status"
assert_equals "" "$(cat "$CALLS")"
# EVERY tool --verify reports, pyyaml included, and `pass` only if none failed:
# calling `pass` unconditionally after the loop printed an ok line beside the
# failure, so the case read as green in the summary.
missing_line=""
for tool in helm openspec pyyaml envtest serena go; do
  case "$out" in *"$tool "*) ;; *) missing_line="$tool" ;; esac
done
[ -z "$missing_line" ] && pass || fail "no --verify line for $missing_line"
assert_contains "$out" "helm missing"

# ITS OWN RUN, not the previous case's `$out`. Reusing that variable made this
# assertion pass without calling the script at all — a test that cannot fail.
it "--verify names the Go floor when the toolchain is older than the modules need"
make_env absent
out=$(CLAUDE_CODE_REMOTE=1 run_bootstrap --verify)
assert_contains "$out" "go missing"
assert_contains "$out" "need >= "

it "--verify says present for what is there"
make_env present
out=$(CLAUDE_CODE_REMOTE=1 run_bootstrap --verify)
assert_contains "$out" "helm present"
assert_contains "$out" "envtest present"

summary
