# egress-proxy — the enforcing proxy AND the redirect installer, one image.
# docker buildx build --platform linux/amd64,linux/arm64 --push -t kmatsebora/agentops-egress-proxy:<tag> ./platform/egress-proxy/
#
# NOT distroless, unlike every other component here, and the reason is the one
# thing this image does that Go cannot: writing the redirect rules.
#
# `iptables` on a modern distribution is a shim over nftables, and which backend
# it selects depends on what the HOST kernel already has. Getting that wrong
# does not fail loudly — the rules apply to a table nothing consults, and the
# agent's traffic sails past unmediated while `iptables -L` looks correct. So
# the image ships the real tool from a distribution that handles the
# legacy/nft selection, rather than a hand-rolled netlink client that would have
# to reimplement that choice.
#
# The privilege this needs is confined to the init container, which exits before
# the agent container starts. The PROXY runs from the same image as an ordinary
# unprivileged user.
FROM --platform=$BUILDPLATFORM golang:1.23 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/egress-proxy .

FROM debian:bookworm-slim
RUN apt-get update -qq && \
    apt-get install -y -qq --no-install-recommends iptables && \
    rm -rf /var/lib/apt/lists/*
COPY --from=build /out/egress-proxy /egress-proxy
# The proxy's uid. It MUST differ from the agent's (1000): the redirect excludes
# this uid, and that exclusion is the only thing separating the two.
USER 1337:1337
ENTRYPOINT ["/egress-proxy"]
