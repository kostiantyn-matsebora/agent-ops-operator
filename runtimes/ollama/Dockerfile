# agentops ollama agent runtime — a second AgentRuntime image, in which the
# RUNTIME is the harness and Ollama is only inference.
#   docker buildx build --platform linux/amd64,linux/arm64 \
#     -t ghcr.io/kostiantyn-matsebora/agentops-runtime-ollama:<tag> ./runtimes/ollama/
#
# PUBLISHED BY TAG, never by hand: `runtime-ollama-v<semver>` runs
# .github/workflows/release.yml. The command above builds locally.
#
# ITS OWN DOCKERFILE, not the shared .github/docker/go-module.Dockerfile, for
# two reasons that each suffice: the MCP SDK needs Go 1.25 where the shared
# recipe is 1.23, and the image needs git and a shell — a checkout to hold and
# a Bash tool to run — where the shared recipe is distroless.
FROM --platform=$BUILDPLATFORM golang:1.25 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/runtime .

FROM debian:bookworm-slim
LABEL org.opencontainers.image.source=https://github.com/kostiantyn-matsebora/agent-ops-operator

# git and openssh for the checkout, ca-certificates for https endpoints, and
# ordinary shell utilities for the Bash tool. NO kubectl and no other domain
# tooling: what an agent may reach is WIRING — MCPConfig + MCPToolset bound by
# a Pipeline — and a CLI here would be the same category error as bundling an
# MCP server. Need one? Derive an image and point AgentRuntime.spec.image at it.
#
# apt-get upgrade -y IS LOAD-BEARING, not defensive boilerplate: CI's build
# cache keys this RUN layer on its own TEXT, so an unchanged instruction
# reuses whatever it resolved to on an earlier build even after a CVE is
# published against a package pulled in as a transitive dependency here
# (libexpat1, CVE-2026-56408, via git/curl/jq) -- Trivy failed two straight
# builds on the SAME cached 2.5.0-1+deb12u2 while a genuinely fresh
# `apt-get install` on this exact base already resolved the patched
# 2.5.0-1+deb12u3, confirmed locally. Removing this line silently reopens
# that cache trap the next time any package here gets a security fix.
RUN apt-get update -qq && \
    apt-get upgrade -y -qq && \
    apt-get install -y -qq --no-install-recommends \
      git openssh-client curl jq ca-certificates procps && \
    rm -rf /var/lib/apt/lists/* && \
    useradd -u 1000 -m -d /home/runtime runtime

COPY --from=build /out/runtime /usr/local/bin/agentops-runtime-ollama

ENV HOME=/data/context
USER runtime
ENTRYPOINT ["/usr/local/bin/agentops-runtime-ollama"]
