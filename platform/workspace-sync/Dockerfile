# workspace-sync — the init container that clones every repository a
# runtime pod needs before the agent container starts.
# docker buildx build --platform linux/amd64,linux/arm64 -t ghcr.io/kostiantyn-matsebora/agentops-workspace-sync:<tag> ./platform/workspace-sync/
#
# NOT distroless, unlike most components here, because this one's whole job
# is running `git` against an https or ssh remote — the shared
# `.github/docker/go-module.Dockerfile` recipe ships a static binary and
# nothing else, which is right for a component with no process dependency,
# and wrong for the one component in this repository that actually needs
# git, a certificate store for https and an ssh client for ssh (design D4/D5).
#
# PUBLISHED BY TAG, never by hand: `workspace-sync-v<semver>` runs
# .github/workflows/release.yml, which refuses a tag already in the registry
# and asserts the pushed manifest's architectures against the declaration in
# .github/components.sh. The command above builds locally; it does not
# publish.
FROM --platform=$BUILDPLATFORM golang:1.27 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/workspace-sync .

FROM debian:bookworm-slim
LABEL org.opencontainers.image.source=https://github.com/kostiantyn-matsebora/agent-ops-operator
# The same daily rebuild the runtimes carry (their Dockerfiles state why):
# the base's own packages get security fixes too, and a cached layer applies
# none of them.
ARG APT_REFRESH=unset
RUN echo "apt refresh: ${APT_REFRESH}" && \
    apt-get update -qq && \
    apt-get upgrade -y -qq && \
    apt-get install -y -qq --no-install-recommends git ca-certificates openssh-client && \
    rm -rf /var/lib/apt/lists/*
COPY --from=build /out/workspace-sync /workspace-sync
# Runs as an ordinary unprivileged user — this container writes only under
# the workspace mount and its own /tmp scratch space, never a privileged
# path.
USER 65532:65532
ENTRYPOINT ["/workspace-sync"]
