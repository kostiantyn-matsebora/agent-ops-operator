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
    useradd -u 1000 -m -d /home/runtime runtime

COPY --from=build /out/runtime /usr/local/bin/agentops-runtime-ollama

ENV HOME=/data/context
USER runtime
ENTRYPOINT ["/usr/local/bin/agentops-runtime-ollama"]
