#!/usr/bin/env bash
# WHAT A CLOUD SESSION OF THIS REPOSITORY NEEDS, INSTALLED FROM THE TREE.
#
# The cloud environment's setup field is ONE line handing off to this file:
#
#   export CLAUDE_CODE_REMOTE=1
#   cd /home/user/agent-ops-operator && bash .github/scripts/cloud-bootstrap.sh
#
# WHY A FILE AND NOT THE FORM. A setup script typed into the environment's form
# is unreviewed, unversioned and invisible to anyone reading this repository —
# and it drifts from the tree the first time a check gains a dependency. The
# line in the form names a file; what the environment installs is then a pull
# request like anything else.
#
# THE IMAGE ALREADY HAS Go, Node 22, Python 3, dockerd, `gh` and jq. What it
# does NOT have is everything below, and the failure each absence produces is
# the reason it is here rather than a preference:
#
#   helm      the chart render tests call exec.LookPath("helm") and SKIP
#             without it. `go test` is GREEN either way — the silent skip is
#             the whole reason the verify pass exists.
#   openspec  every opsx command, at the version ci.yml installs.
#   pyyaml    docs-generate.py and docs-task-guard.py import it.
#   envtest   the manager's integration suite needs KUBEBUILDER_ASSETS.
#   serena    the .mcp.json stdio server.
#   Go 1.25   platform/manager and runtimes/ollama declare it; the image's own
#             version is unstated, so it is checked rather than assumed.
#
# IT ALWAYS EXITS 0. The platform treats a failed setup as a failed session,
# and a session with one tool missing is more useful than no session at all —
# so each section is independent, a failure is NAMED on stderr at the end, and
# the verify pass tells the model what it lacks before any work begins.
#
# IT IS IDEMPOTENT. The environment's cache rebuilds about weekly and this
# re-runs unchanged; every section checks before it installs.
set -uo pipefail

HELM_VERSION="${HELM_VERSION:-v3.16.3}"
OPENSPEC_VERSION="${OPENSPEC_VERSION:-1.10.0}"
ENVTEST_K8S="${ENVTEST_K8S:-1.31.x}"
ENVTEST_DIR="${ENVTEST_DIR:-$HOME/.envtest}"
GO_FLOOR="${GO_FLOOR:-1.25}"

MISSING=()
VERIFY_ONLY=0
[ "${1:-}" = "--verify" ] && VERIFY_ONLY=1

# THE MARKER IS THE WHOLE OF THE REMOTE TEST. Unset, `0` or `false` means a
# workstation, where every one of these installs would be an uninvited change
# to somebody's machine.
remote() {
  case "${CLAUDE_CODE_REMOTE:-}" in
    1|true|TRUE|True) return 0 ;;
    *) return 1 ;;
  esac
}

have() { command -v "$1" >/dev/null 2>&1; }

note() { printf '%s\n' "$*"; }

# A tool that could not be installed is named, once, at the end. Never fatal:
# the next section still runs.
missing() { MISSING+=("$1"); }

# --- verify: what is here, what is not --------------------------------------
#
# One line per tool. This is the SessionStart hook, and its output is the
# session's context — so it states presence rather than explaining anything.
verify() {
  local go_version="absent"
  have go && go_version=$(go version 2>/dev/null | awk '{print $3}' | sed 's/^go//')

  printf 'helm %s\n'      "$(have helm && echo present || echo missing)"
  printf 'openspec %s\n'  "$(have openspec && echo present || echo missing)"
  printf 'pyyaml %s\n'    "$(python3 -c 'import yaml' 2>/dev/null && echo present || echo missing)"
  printf 'envtest %s\n'   "$([ -n "$(find "$ENVTEST_DIR" -name kube-apiserver -type f 2>/dev/null | head -1)" ] && echo present || echo missing)"
  printf 'serena %s\n'    "$(have serena && echo present || echo missing)"
  printf 'go %s\n'        "$(go_at_least "$GO_FLOOR" && echo "present ($go_version)" || echo "missing (need >= $GO_FLOOR, have $go_version)")"
}

# `sort -V` decides it, so 1.10 is not read as older than 1.9.
go_at_least() {  # go_at_least <floor>
  have go || return 1
  local have_v; have_v=$(go version 2>/dev/null | awk '{print $3}' | sed 's/^go//')
  [ -n "$have_v" ] || return 1
  [ "$(printf '%s\n%s\n' "$1" "$have_v" | sort -V | head -1)" = "$1" ]
}

## helm — the chart render tests skip silently without it, and docs-generate.py
## renders the chart to fill its generated blocks.
install_helm() {
  have helm && { note "helm $(helm version --short 2>/dev/null || echo present)"; return; }
  note "installing helm $HELM_VERSION"
  local tmp; tmp=$(mktemp -d)
  if curl -fsSL "https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz" -o "$tmp/helm.tgz" \
     && tar -xzf "$tmp/helm.tgz" -C "$tmp" \
     && install -m 0755 "$tmp/linux-amd64/helm" "$HOME/.local/bin/helm" 2>/dev/null; then
    note "helm $(helm version --short 2>/dev/null || echo installed)"
  else
    missing helm
  fi
  rm -rf "$tmp"
}

## openspec — every opsx command, pinned to the version ci.yml installs so a
## remote session and CI agree on what a change's shape is.
install_openspec() {
  have openspec && { note "openspec $(openspec --version 2>/dev/null || echo present)"; return; }
  note "installing @fission-ai/openspec@$OPENSPEC_VERSION"
  if npm install -g "@fission-ai/openspec@$OPENSPEC_VERSION" >/dev/null 2>&1 && have openspec; then
    note "openspec $(openspec --version 2>/dev/null || echo installed)"
  else
    missing openspec
  fi
}

## pyyaml — docs-generate.py, docs-task-guard.py and the script suite import it.
install_pyyaml() {
  python3 -c 'import yaml' 2>/dev/null && { note "pyyaml present"; return; }
  note "installing pyyaml"
  if pip install --user pyyaml >/dev/null 2>&1 && python3 -c 'import yaml' 2>/dev/null; then
    note "pyyaml installed"
  else
    missing pyyaml
  fi
}

## envtest — the manager's integration suite runs against a real API server and
## needs KUBEBUILDER_ASSETS pointed at these binaries.
install_envtest() {
  local found
  found=$(find "$ENVTEST_DIR" -name kube-apiserver -type f 2>/dev/null | head -1)
  if [ -n "$found" ]; then
    note "envtest present: KUBEBUILDER_ASSETS=$(dirname "$found")"
    return
  fi
  have go || { missing envtest; return; }
  note "installing the envtest assets for $ENVTEST_K8S"
  local path
  if path=$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.19 \
              use "$ENVTEST_K8S" --bin-dir "$ENVTEST_DIR" -p path 2>/dev/null) && [ -n "$path" ]; then
    note "envtest installed: KUBEBUILDER_ASSETS=$path"
  else
    missing envtest
  fi
}

## serena — the .mcp.json stdio server. `.mcp.json` waits for this remotely,
## which is why a failure here is a slow MCP start rather than a broken one.
install_serena() {
  have serena && { note "serena present"; return; }
  note "installing serena"
  if have uv && uv tool install --quiet git+https://github.com/oraios/serena >/dev/null 2>&1 && have serena; then
    note "serena installed"
  else
    missing serena
  fi
}

## Go — platform/manager and runtimes/ollama declare 1.25. The image's version
## is unstated, so this VERIFIES and only installs when what is there is older.
install_go() {
  if go_at_least "$GO_FLOOR"; then
    note "go $(go version | awk '{print $3}') (floor go$GO_FLOOR)"
    return
  fi
  note "installing go$GO_FLOOR.0 (the image's is older than the floor)"
  local tmp; tmp=$(mktemp -d)
  if curl -fsSL "https://go.dev/dl/go${GO_FLOOR}.0.linux-amd64.tar.gz" -o "$tmp/go.tgz" \
     && tar -xzf "$tmp/go.tgz" -C "$HOME" \
     && ln -sf "$HOME/go/bin/go" "$HOME/.local/bin/go" \
     && ln -sf "$HOME/go/bin/gofmt" "$HOME/.local/bin/gofmt"; then
    note "go $(go version 2>/dev/null | awk '{print $3}' || echo installed)"
  else
    missing go
  fi
  rm -rf "$tmp"
}

main() {
  if ! remote; then
    # A WORKSTATION. Nothing is installed and nothing is printed: this file is
    # on the PreToolUse-free path of every local session through the verify
    # hook, and a hook that speaks on a workstation is a hook that gets
    # switched off.
    exit 0
  fi

  mkdir -p "$HOME/.local/bin"
  case ":$PATH:" in *":$HOME/.local/bin:"*) ;; *) export PATH="$HOME/.local/bin:$PATH" ;; esac

  if [ "$VERIFY_ONLY" = 1 ]; then
    verify
    exit 0
  fi

  install_helm
  install_openspec
  install_pyyaml
  install_envtest
  install_serena
  install_go

  if [ ${#MISSING[@]} -gt 0 ]; then
    # NAMED, NEVER FATAL. The setup must exit 0 or the platform reports a
    # failed session; the verify hook repeats this in the model's context.
    printf 'cloud-bootstrap: could not install: %s\n' "${MISSING[*]}" >&2
  fi
  exit 0
}

main "$@"
