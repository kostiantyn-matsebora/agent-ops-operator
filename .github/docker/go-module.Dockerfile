# THE recipe for every self-contained Go module in this repository.
#
# Nine components built from byte-identical Dockerfiles — the five signal
# adapters, channel-telegram, gateway-telegram, context-sync and housekeeping.
# Nine copies of one file is nine places for a base-image bump to be applied in
# eight of, and the tell was an edit that had to be SCRIPTED across all of them.
#
# THE CONTEXT IS STILL THE MODULE'S OWN DIRECTORY. `docker build -f` separates
# the recipe from the context, so nothing copies across a component boundary and
# `.claude/rules/structure.md`'s "a directory holds everything its container
# builds from" is intact — that rule constrains the CONTEXT, which has not
# moved.
#
#   docker build -f .github/docker/go-module.Dockerfile ./signals/cron
#
# `.github/components.sh` hands this path to every module without a Dockerfile
# of its own. A component needing something different gets a Dockerfile in its
# directory and is served by that instead — manager, console, egress-proxy and
# runtime-claude are the four.
#
# THE BINARY IS `/app`, AND THAT IS FORCED RATHER THAN CHOSEN. An exec-form
# ENTRYPOINT does not expand build arguments, and distroless carries no shell
# for the shell form — so a per-component `ENTRYPOINT ["/${BINARY}"]` cannot
# work. A fixed path is the only shape that does.
FROM --platform=$BUILDPLATFORM golang:1.23 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/app .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/app /app
USER 65532:65532
ENTRYPOINT ["/app"]
