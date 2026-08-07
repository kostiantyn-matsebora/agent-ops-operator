# channel-telegram adapter — multi-stage, multi-arch build
# Adapter pods carry no node constraints, so they must run on every node
# architecture in the cluster (mixed amd64/arm64 clusters are common):
#   docker buildx build --platform linux/amd64,linux/arm64 \
#     -t kmatsebora/agentops-channel-telegram:<tag> --push ./channel-telegram/
FROM --platform=$BUILDPLATFORM golang:1.23 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/channel-telegram .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/channel-telegram /channel-telegram
USER 65532:65532
ENTRYPOINT ["/channel-telegram"]
