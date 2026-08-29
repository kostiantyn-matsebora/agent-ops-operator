# TEST IMAGE — the stub runtime. Built by the e2e pack and imported into its
# cluster under a name that says what it is: agentops-test-stub-runtime.
# Published by no release tag (.github/components.sh excludes test/), referenced
# by no chart default and no sample. If you see it in a real deployment,
# something is wrong.
FROM --platform=$BUILDPLATFORM golang:1.25 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/app .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/app /app
USER 65532:65532
ENTRYPOINT ["/app"]
