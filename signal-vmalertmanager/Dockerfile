# signal-vmalertmanager adapter — multi-stage build
# docker buildx build --platform linux/amd64,linux/arm64 --push -t kmatsebora/agentops-signal-vmalertmanager:<tag> ./signal-vmalertmanager/
FROM --platform=$BUILDPLATFORM golang:1.23 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/signal-vmalertmanager .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/signal-vmalertmanager /signal-vmalertmanager
USER 65532:65532
ENTRYPOINT ["/signal-vmalertmanager"]
