# agentops manager — multi-stage build
# docker buildx build --platform linux/amd64,linux/arm64 -t ghcr.io/kostiantyn-matsebora/agentops-manager:<tag> ./platform/manager/
#
# PUBLISHED BY TAG, never by hand: `manager-v<semver>` runs .github/workflows/release.yml,
# which refuses a tag already in the registry and asserts the pushed manifest's
# architectures against the declaration in .github/components.sh. The command above
# builds locally; it does not publish.
FROM --platform=$BUILDPLATFORM golang:1.23 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY api/ api/
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/manager ./cmd/manager

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/manager /manager
USER 65532:65532
ENTRYPOINT ["/manager"]
