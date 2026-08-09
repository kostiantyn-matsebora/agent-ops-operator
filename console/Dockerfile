# agent-ops console — multi-stage, multi-arch build
# Adapter pods carry no node constraints, so they must run on every node
# architecture in the cluster (mixed amd64/arm64 clusters are common):
#   docker buildx build --platform linux/amd64,linux/arm64 \
#     -t kmatsebora/agentops-console:<tag> --push ./console/
# Never overwrite a pushed tag — bump it on every change.
FROM --platform=$BUILDPLATFORM golang:1.23 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY *.go ./
# the SPA is embedded (go:embed ui) — there is no asset pipeline to run
COPY ui/ ./ui/
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/console .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/console /console
USER 65532:65532
ENTRYPOINT ["/console"]
