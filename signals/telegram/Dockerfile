# signal-telegram — multi-stage, multi-arch build
# Adapter pods carry no node constraints, so they must run on every node
# architecture in the cluster (mixed amd64/arm64 clusters are common):
#   docker buildx build --platform linux/amd64,linux/arm64 \
#     -t kmatsebora/agentops-signal-telegram:<tag> --push ./signal-telegram/
FROM --platform=$BUILDPLATFORM golang:1.23 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/signal-telegram .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/signal-telegram /signal-telegram
USER 65532:65532
ENTRYPOINT ["/signal-telegram"]
