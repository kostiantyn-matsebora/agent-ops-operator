# agentops claude agent runtime — the default AgentRuntime image.
# docker build --platform linux/amd64 -t kmatsebora/agentops-runtime-claude:<tag> .
FROM node:22-bookworm-slim

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
COPY runtime.js tools.js /srv/

ENV HOME=/data/home
USER node
CMD ["node", "/srv/runtime.js"]
