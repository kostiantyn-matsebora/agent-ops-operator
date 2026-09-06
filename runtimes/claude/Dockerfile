# agentops claude agent runtime — the default AgentRuntime image.
# docker buildx build --platform linux/amd64,linux/arm64 \
#   -t ghcr.io/kostiantyn-matsebora/agentops-runtime-claude:<tag> ./runtimes/claude/
#
# PUBLISHED BY TAG, never by hand: `runtime-claude-v<semver>` runs .github/workflows/release.yml,
# which refuses a tag already in the registry and asserts the pushed manifest's
# architectures against the declaration in .github/components.sh. The command above
# builds locally; it does not publish.
FROM node:22-bookworm-slim
LABEL org.opencontainers.image.source=https://github.com/kostiantyn-matsebora/agent-ops-operator

# --ignore-scripts refuses every package's own lifecycle scripts, npm's
# transitive dependencies included; claude-code's native binary then needs
# its OWN installer run explicitly and nothing else's -- deliberate and
# scoped, rather than every postinstall in the tree running unexamined.
# Verified against node:22-bookworm-slim: the plain install fails at
# `claude --version` with "native binary not installed" until this second
# step runs.
#
# BOTH VERSIONS ARE PINNED (docker:S8543) -- an EXPLICIT, hand-bumped pair,
# not resolved implicitly at build time. Was `@latest` on both; check what
# `npm view npm version` / `npm view @anthropic-ai/claude-code version` now
# resolve to before bumping, same as any other dependency upgrade here.
#
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
    rm -rf /var/lib/apt/lists/* && \
    npm install -g --ignore-scripts npm@12.0.2 @anthropic-ai/claude-code@2.1.252 && \
    node "$(npm root -g)/@anthropic-ai/claude-code/install.cjs" && \
    npm cache clean --force

# NO kubectl, and no other domain tooling. This image is a VENDOR runtime: it
# checks out a repo and runs claude-code. What an agent may reach is wiring —
# MCPConfig + MCPToolset bound by a Pipeline — so a Kubernetes CLI here would be
# the same category error as bundling an MCP server. Need one? Derive an image
# (three lines, and its own version pin becomes yours to own against your
# cluster, same as this one) and point AgentRuntime.spec.image at it; see README.
COPY runtime.js tools.js spin.js /srv/

ENV HOME=/data/context
USER node
CMD ["node", "/srv/runtime.js"]
