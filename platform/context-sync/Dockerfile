# context-sync sidecar — multi-stage build
# docker buildx build --platform linux/amd64,linux/arm64 --push -t kmatsebora/agentops-context-sync:<tag> ./platform/context-sync/
#
# distroless/static, like every other component here: the incremental copy is
# implemented in Go rather than by shelling out to rsync, so the image needs no
# shell and no external tool that would have to be present and correct.
FROM --platform=$BUILDPLATFORM golang:1.23 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/context-sync .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/context-sync /context-sync
USER 65532:65532
ENTRYPOINT ["/context-sync"]
