# agentops copilot agent runtime — the third AgentRuntime image, driving GitHub
# Copilot through its SDK.
# docker buildx build --platform linux/amd64,linux/arm64 \
#   -t ghcr.io/kostiantyn-matsebora/agentops-runtime-copilot:<tag> ./runtimes/copilot/
#
# PUBLISHED BY TAG, never by hand: `runtime-copilot-v<semver>` runs
# .github/workflows/release.yml, which refuses a tag already in the registry and
# asserts the pushed manifest's architectures against .github/components.sh.
# The command above builds locally; it does not publish.
FROM node:22-bookworm-slim
LABEL org.opencontainers.image.source=https://github.com/kostiantyn-matsebora/agent-ops-operator

# THE APT LAYER IS REBUILT ONCE A DAY, AND `apt-get upgrade -y` IS WHAT THAT
# REBUILD BUYS. CI's build cache keys this RUN layer on its instruction text
# plus the build arguments it reads, so an unchanged instruction reuses
# whatever it resolved to on an earlier build even after a CVE is published
# against a package pulled in here. Twice now: libexpat1 (CVE-2026-56408, via
# git/curl/jq) failed two straight scans on a cached 2.5.0-1+deb12u2 while a
# fresh install already resolved deb12u3; then libssh2 (CVE-2026-7598,
# CVE-2026-58050) on a layer cached BEFORE deb12u1 existed -- and by then the
# upgrade line was already here, so it patched nothing: a cached layer runs
# no command. `APT_REFRESH` is the date, passed by `ci.yml` and
# `build-image.yml`, and reading it below is what makes the key change daily.
# Removing either half silently reopens the trap the next time any package
# here gets a security fix, and the release path shares the same cache scope,
# so a stale layer would reach a PUBLISHED image.
ARG APT_REFRESH=unset
RUN echo "apt refresh: ${APT_REFRESH}" && \
    apt-get update -qq && \
    apt-get upgrade -y -qq && \
    apt-get install -y -qq --no-install-recommends \
      git openssh-client curl jq ca-certificates procps && \
    rm -rf /var/lib/apt/lists/*

# NO kubectl, and no other domain tooling. This image is a VENDOR runtime: it
# checks out a repo and runs Copilot. What an agent may reach is wiring —
# MCPConfig + MCPToolset bound by a Pipeline — so a Kubernetes CLI here would be
# the same category error as bundling an MCP server. Need one? Derive an image
# (three lines, and its own version pin becomes yours to own against your
# cluster, same as this one) and point AgentRuntime.spec.image at it; see
# docs/concepts.md.
WORKDIR /srv
COPY package.json package-lock.json /srv/
# `npm ci` from the lockfile: the SDK and the CLI it drives are one artifact
# released as two packages, and a floating range once installed a pair that
# threw before any session existed (SDK 1.0.11 against CLI 1.0.81).
# npm FIRST: the image scan gates on fixable CRITICAL/HIGH, and the npm the
# base image ships carries old `brace-expansion`, `undici`, `tar` and
# `ip-address` under /usr/local/lib/node_modules/npm — the same fix
# runtimes/claude applies. PINNED, not @latest (docker:S8543) -- an
# EXPLICIT, hand-bumped version; check `npm view npm version` before
# bumping, and re-verify against a Trivy scan of the built image that the
# CVE this upgrade exists for stays fixed.
#
# THE PIN ALONE STOPPED BEING ENOUGH: npm 12.2.0 (the latest release as of
# this fix) still bundles versions of all four below their fixed ones.
# There is no npm release yet that clears the scan, so each vendored package
# is overwritten in place with a same-major bump that satisfies npm's own
# range on it (minimatch wants brace-expansion ^5.0.5, node-gyp wants undici
# ^6.25.0, node-gyp/pacote/libnpmdiff want tar ^7.4.3 or newer, socks wants
# ip-address ^10.1.1) -- `npm pack` pulls the fixed tarball straight from the
# registry, no extra file to commit. Re-check each range before ever bumping
# npm past this pin, since a new npm version may resolve a different
# transitive version, and re-verify against a Trivy scan that the CVE each
# entry exists for stays fixed.
# THEN drop the CLI's bundled foundry-local-sdk: a Windows/macOS local-model
# host this Linux pod can never use, whose vendored adm-zip is the one finding
# npm cannot reach (it sits inside the platform package, not in the lockfile).
# --ignore-scripts on both: verified against this lockfile that neither step
# needs a lifecycle script -- the SDK's prebuilt copilot-linux-x64 binary
# ships as plain files, not a postinstall download, so skipping every
# package's scripts (npm's own and the ~20 in this tree) costs nothing here,
# unlike runtimes/claude where the CLI needs its own installer run after.
RUN npm install -g --ignore-scripts npm@12.0.2 && \
    npm ci --omit=dev --no-audit --no-fund --ignore-scripts && \
    rm -rf /srv/node_modules/@github/copilot-linux-*/foundry-local-sdk && \
    NPM_ROOT="$(npm root -g)/npm/node_modules" && \
    cd /tmp && \
    for entry in brace-expansion@5.0.12 undici@6.28.1 tar@7.5.22 ip-address@10.7.2; do \
      pkg="$(echo "$entry" | cut -d@ -f1)" && \
      tgz="$(npm pack --silent "$entry")" && \
      rm -rf "$NPM_ROOT/$pkg" && tar xzf "$tgz" && mv package "$NPM_ROOT/$pkg" && \
      rm -f "$tgz"; \
    done && \
    npm cache clean --force
COPY runtime.js tools.js vocabulary.js mcp.js continuity.js report.js /srv/

ENV HOME=/data/context
USER node
CMD ["node", "/srv/runtime.js"]
