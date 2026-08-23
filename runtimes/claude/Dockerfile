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

RUN apt-get update -qq && \
    apt-get install -y -qq --no-install-recommends \
      git openssh-client curl jq ca-certificates procps && \
    rm -rf /var/lib/apt/lists/* && \
    npm install -g @anthropic-ai/claude-code && \
    npm cache clean --force

# NO kubectl, and no other domain tooling. This image is a VENDOR runtime: it
# checks out a repo and runs claude-code. What an agent may reach is wiring —
# MCPConfig + MCPToolset bound by a Pipeline — so a Kubernetes CLI here would be
# the same category error as bundling an MCP server. Need one? Derive an image
# (three lines, and the version pin becomes yours to own against your cluster)
# and point AgentRuntime.spec.image at it; see README.
COPY runtime.js tools.js spin.js /srv/

ENV HOME=/data/home
USER node
CMD ["node", "/srv/runtime.js"]
