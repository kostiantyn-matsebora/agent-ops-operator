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
#   k3d       the system pack's cluster, with kubectl beside it. The testing
#   kubectl   station runs that tier ITSELF, so a session without them can only
#             dispatch it.
#   docker    (dockerd) is not installed here: the image has it. It is STARTED, detached,
#             when nothing answers, and reported by --verify like the rest.
#   playwright  the e2e tier: the console UI's node_modules and Chromium with its
#             system libraries. Without them `npm run test:e2e` cannot start.
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
K3D_VERSION="${K3D_VERSION:-v5.9.0}"
KUBECTL_VERSION="${KUBECTL_VERSION:-v1.31.4}"

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
  printf 'k3d %s\n'       "$(have k3d && echo present || echo missing)"
  printf 'kubectl %s\n'   "$(have kubectl && echo present || echo missing)"
  printf 'playwright %s\n' "$(playwright_ready && echo present || echo missing)"
  printf 'docker %s\n'    "$(docker_answers && echo running || echo "not running")"
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

## k3d and kubectl — the system pack creates its cluster with them.
install_k3d() {
  have k3d && { note "k3d $(k3d version 2>/dev/null | head -1 || echo present)"; return; }
  note "installing k3d $K3D_VERSION"
  if curl -fsSL "https://github.com/k3d-io/k3d/releases/download/${K3D_VERSION}/k3d-linux-amd64" -o "$HOME/.local/bin/k3d" \
     && chmod +x "$HOME/.local/bin/k3d"; then
    note "k3d installed"
  else
    rm -f "$HOME/.local/bin/k3d"; missing k3d
  fi
}

install_kubectl() {
  have kubectl && { note "kubectl present"; return; }
  note "installing kubectl $KUBECTL_VERSION"
  if curl -fsSL "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" -o "$HOME/.local/bin/kubectl" \
     && chmod +x "$HOME/.local/bin/kubectl"; then
    note "kubectl installed"
  else
    rm -f "$HOME/.local/bin/kubectl"; missing kubectl
  fi
}

# `docker` or `nerdctl`: either answers when its engine runs.
# BOUNDED: a daemon that is starting can hang `info`, and this runs inside a 10s hook budget. A dockerd started
# through sudo answers a root caller only until the socket's group is open, so the same check is tried as root too.
docker_answers() {
  local c
  for c in docker nerdctl; do
    have "$c" || continue
    timeout 3 "$c" info >/dev/null 2>&1 && return 0
    have sudo && timeout 3 sudo -n "$c" info >/dev/null 2>&1 && return 0
  done
  return 1
}

## dockerd — the image HAS it, but a session starts with none running. Started, by `--verify`, in the
## background when nothing answers, and waited on briefly. A sandbox that forbids it
## (no privileges) leaves it named as missing, and the station dispatches the tier instead.
start_docker() {
  docker_answers && { note "docker running"; return; }
  have dockerd || { missing docker; return; }
  note "starting dockerd"
  # DETACHED (setsid), because the SessionStart hook that runs this is killed at its own timeout (10s), and
  # a plain background child would be killed with it. The wait is short for the same reason: the routine
  # waits for the daemon itself before it needs one.
  if have sudo; then setsid nohup sudo dockerd >/dev/null 2>&1 < /dev/null & else setsid nohup dockerd >/dev/null 2>&1 < /dev/null & fi
  local i
  for i in 1 2 3; do
    docker_answers && { note "docker running"; return; }
    sleep 2
  done
  missing "docker (dockerd was started and is not answering yet)"
}

## playwright — the e2e tier. Dependencies come from the lockfile (`npm ci`), then the
## one browser the specs use. `--with-deps` also installs the system libraries, which
## needs root: a sandbox without it leaves the browser installed but unable to launch,
## and the verify line says so by trying to launch it.
UI_DIR="${UI_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/platform/console/ui}"

playwright_ready() {
  [ -x "$UI_DIR/node_modules/.bin/playwright" ] || return 1
  (cd "$UI_DIR" && npx --no-install playwright --version >/dev/null 2>&1) || return 1
  [ -n "$(find "${PLAYWRIGHT_BROWSERS_PATH:-$HOME/.cache/ms-playwright}" -maxdepth 1 -name 'chromium*' 2>/dev/null | head -1)" ]
}

install_playwright() {
  playwright_ready && { note "playwright present"; return; }
  [ -f "$UI_DIR/package-lock.json" ] || { missing playwright; return; }
  note "installing the console UI's dependencies and chromium"
  if (cd "$UI_DIR" && npm ci --ignore-scripts >/dev/null 2>&1) \
     && (cd "$UI_DIR" && { if have sudo; then sudo -E env "PATH=$PATH" npx --no-install playwright install --with-deps chromium; \
                          else npx --no-install playwright install --with-deps chromium; fi; } >/dev/null 2>&1 \
         || npx --no-install playwright install chromium >/dev/null 2>&1) \
     && playwright_ready; then
    note "playwright installed"
  else
    missing playwright
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
  # NOT `-C $HOME`. The tarball unpacks to `./go`, and `$HOME/go` is GOPATH's
  # DEFAULT — so extracting there drops the toolchain's own bin/pkg/src into
  # the module cache's directory. It works until something writes the other's
  # tree, and then it fails as a build error naming neither.
  if curl -fsSL "https://go.dev/dl/go${GO_FLOOR}.0.linux-amd64.tar.gz" -o "$tmp/go.tgz" \
     && rm -rf "$HOME/.local/go" && mkdir -p "$HOME/.local" \
     && tar -xzf "$tmp/go.tgz" -C "$HOME/.local" \
     && ln -sf "$HOME/.local/go/bin/go" "$HOME/.local/bin/go" \
     && ln -sf "$HOME/.local/go/bin/gofmt" "$HOME/.local/bin/gofmt"; then
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
  # PREPENDED UNCONDITIONALLY, and the "already there" case is exactly why. If
  # the image puts `~/.local/bin` on PATH but AFTER `/usr/local/go/bin`, a
  # skipped prepend leaves the image's older `go` winning every lookup — so the
  # toolchain this script just installed would be invisible, which is the one
  # failure `install_go` exists to prevent. A duplicate entry costs nothing.
  export PATH="$HOME/.local/bin:$PATH"

  if [ "$VERIFY_ONLY" = 1 ]; then
    # A RUNNING PROCESS IS NOT SNAPSHOTTED. The setup script's filesystem is cached, a daemon is
    # not, so dockerd is started here, at every session start, and never in the setup run.
    start_docker >/dev/null 2>&1
    verify
    exit 0
  fi

  # GO FIRST. `install_envtest` runs `go run` to fetch the assets, so with an
  # image older than the floor it would build them with the toolchain this
  # script is about to replace — and the ORDER is the only thing that decides
  # which one it used.
  install_go
  install_helm
  install_openspec
  install_pyyaml
  install_envtest
  install_serena
  install_k3d
  install_kubectl
  install_playwright

  if [ ${#MISSING[@]} -gt 0 ]; then
    # NAMED, NEVER FATAL. The setup must exit 0 or the platform reports a
    # failed session; the verify hook repeats this in the model's context.
    printf 'cloud-bootstrap: could not install: %s\n' "${MISSING[*]}" >&2
  fi
  exit 0
}

main "$@"
