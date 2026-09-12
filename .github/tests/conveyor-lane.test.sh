#!/usr/bin/env bash
# THE LANE IS READ, NEVER JUDGED. An issue is on the opsx lane when a change's
# `.github-issue` sidecar names it, or it carries an `opsx:` phase label — both
# FACTS a program reads, written by `opsx-issue.sh` rather than typed by
# anyone. Neither test looks at how the issue is WORDED, which this suite
# proves by running the identical issue body through both fixtures and getting
# different lanes purely from the binding.
#
# NO NETWORK: `gh` is stubbed for the one call `is_opsx_lane` makes (the
# issue's labels), and the sidecar glob is a real file under a throwaway
# working directory `carry-grant.py` is imported from.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/carry-grant.py"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin"

# A `gh` answering `issue view --json labels` from a fixture, and recording
# nothing else this suite needs.
stub_gh_labels() {  # stub_gh_labels <bindir> <labels-json-array>
  local bin="$1"; mkdir -p "$bin"
  cat > "$bin/gh" <<STUB
#!/usr/bin/env bash
case "\$*" in
  "issue view "*"--json labels")
    echo '{"labels":$2}' ;;
  *) echo '{}' ;;
esac
exit 0
STUB
  chmod +x "$bin/gh"
}

lane() {  # lane <issue-number> <labels-json-array> -- run from $tmp/cwd
  stub_gh_labels "$tmp/bin" "$2"
  PATH="$tmp/bin:$PATH" python3 -c "
import sys
sys.path.insert(0, '$ROOT/.github/scripts')
import importlib.util
spec = importlib.util.spec_from_file_location('carry_grant', '$S')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
print('opsx' if m.is_opsx_lane('o/r', $1) else 'plain')
"
}

setup() {
  rm -rf "$tmp/cwd"; mkdir -p "$tmp/cwd/openspec/changes"
}

# --- a change's .github-issue names the issue -> opsx, whatever the body says

it "a .github-issue sidecar naming the issue is the opsx lane, regardless of wording"
setup
mkdir -p "$tmp/cwd/openspec/changes/some-change"
printf '42' > "$tmp/cwd/openspec/changes/some-change/.github-issue"
out=$(cd "$tmp/cwd" && lane 42 '[]')
assert_equals "opsx" "$out"

it "the SAME body, on an issue NO sidecar names, is the plain lane"
setup
mkdir -p "$tmp/cwd/openspec/changes/some-change"
printf '42' > "$tmp/cwd/openspec/changes/some-change/.github-issue"
out=$(cd "$tmp/cwd" && lane 99 '[]')
assert_equals "plain" "$out"

# --- an opsx: phase label, with no binding at all -> opsx

it "an opsx: phase label with no sidecar binding is still the opsx lane"
setup
out=$(cd "$tmp/cwd" && lane 7 '[{"name":"opsx:applying"}]')
assert_equals "opsx" "$out"

it "an ordinary label with no sidecar binding is the plain lane"
setup
out=$(cd "$tmp/cwd" && lane 7 '[{"name":"bug"}]')
assert_equals "plain" "$out"

it "no labels at all, no sidecar: the plain lane"
setup
out=$(cd "$tmp/cwd" && lane 7 '[]')
assert_equals "plain" "$out"

# --- a binding for a DIFFERENT issue does not leak onto this one

it "a sidecar binding a DIFFERENT issue number does not make this issue opsx"
setup
mkdir -p "$tmp/cwd/openspec/changes/other-change"
printf '55' > "$tmp/cwd/openspec/changes/other-change/.github-issue"
out=$(cd "$tmp/cwd" && lane 56 '[]')
assert_equals "plain" "$out"

# --- the wording of the issue changes nothing: same body, two fixtures,
# different lanes purely from the binding/label.

it "identical issue text on both fixtures still splits by the binding alone"
setup
mkdir -p "$tmp/cwd/openspec/changes/worded-change"
printf '10' > "$tmp/cwd/openspec/changes/worded-change/.github-issue"
opsx_out=$(cd "$tmp/cwd" && lane 10 '[]')
plain_out=$(cd "$tmp/cwd" && lane 11 '[]')
assert_equals "opsx" "$opsx_out"
assert_equals "plain" "$plain_out"

summary
