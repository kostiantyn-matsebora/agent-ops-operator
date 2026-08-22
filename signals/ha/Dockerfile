# signal-ha adapter — multi-stage build
# docker buildx build --platform linux/amd64,linux/arm64 --push -t kmatsebora/agentops-signal-ha:<tag> ./signals/ha/
FROM --platform=$BUILDPLATFORM golang:1.23 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/signal-ha .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/signal-ha /signal-ha
USER 65532:65532
ENTRYPOINT ["/signal-ha"]
