# agentops claude agent runtime — the default AgentRuntime image.
# docker build --platform linux/amd64 -t kmatsebora/agentops-runtime-claude:<tag> .
FROM node:22-bookworm-slim

RUN apt-get update -qq && \
    apt-get install -y -qq --no-install-recommends \
      git openssh-client curl jq ca-certificates procps && \
    rm -rf /var/lib/apt/lists/* && \
    npm install -g @anthropic-ai/claude-code && \
    npm cache clean --force

# kubectl for the bounded cluster-apply lane (RBAC of the worker SA is the wall)
RUN curl -fsSL -o /usr/local/bin/kubectl \
      "https://dl.k8s.io/release/v1.34.3/bin/linux/amd64/kubectl" && \
    chmod 0755 /usr/local/bin/kubectl

COPY runtime.js /srv/runtime.js

ENV HOME=/data/home
USER node
CMD ["node", "/srv/runtime.js"]
