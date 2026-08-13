# housekeeping job — multi-stage build
# docker buildx build --platform linux/amd64,linux/arm64 --push -t kmatsebora/agentops-housekeeping:<tag> ./housekeeping/
FROM --platform=$BUILDPLATFORM golang:1.23 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/housekeeping .

# No agent code and no shell: this workload mounts the claim ROOTS, which is
# exactly the reach subPath isolation denies runtime pods.
FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/housekeeping /housekeeping
USER 65532:65532
ENTRYPOINT ["/housekeeping"]
