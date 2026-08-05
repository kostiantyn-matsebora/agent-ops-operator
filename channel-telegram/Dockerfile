# channel-telegram adapter — multi-stage build
# docker build --platform linux/amd64 -t kmatsebora/agentops-channel-telegram:<tag> ./channel-telegram/
FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/channel-telegram .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/channel-telegram /channel-telegram
USER 65532:65532
ENTRYPOINT ["/channel-telegram"]
