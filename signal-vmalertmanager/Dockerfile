# signal-vmalertmanager adapter — multi-stage build
# docker build --platform linux/amd64 -t kmatsebora/agentops-signal-vmalertmanager:<tag> ./signal-vmalertmanager/
FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/signal-vmalertmanager .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/signal-vmalertmanager /signal-vmalertmanager
USER 65532:65532
ENTRYPOINT ["/signal-vmalertmanager"]
