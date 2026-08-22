# signal-cron adapter — multi-stage build
# docker buildx build --platform linux/amd64,linux/arm64 --push -t kmatsebora/agentops-signal-cron:<tag> ./signals/cron/
FROM --platform=$BUILDPLATFORM golang:1.23 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/signal-cron .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/signal-cron /signal-cron
USER 65532:65532
ENTRYPOINT ["/signal-cron"]
