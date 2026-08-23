# agent-ops console — multi-stage, multi-arch build.
#
# Stage 1 builds the SPA with npm; stage 2 embeds it into the Go binary. npm
# exists HERE and nowhere else: no other module and not the manager gains a
# build step, and the shipped artifact stays one distroless Go image serving one
# port with nothing fetched at runtime.
#
#   docker buildx build --platform linux/amd64,linux/arm64 \
#     -t ghcr.io/kostiantyn-matsebora/agentops-console:<tag> ./platform/console/
# Never overwrite a pushed tag — bump it on every change.
#
# PUBLISHED BY TAG, never by hand: `console-v<semver>` runs .github/workflows/release.yml,
# which refuses a tag already in the registry and asserts the pushed manifest's
# architectures against the declaration in .github/components.sh. The command above
# builds locally; it does not publish.

FROM --platform=$BUILDPLATFORM node:22-alpine AS ui
WORKDIR /ui
# package files first so a source-only edit reuses the install layer
COPY ui/package.json ui/package-lock.json* ./
# `npm ci` when a lockfile is present (reproducible), `npm install` otherwise
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi
COPY ui/ ./
RUN npm run build
# PRE-COMPRESS, once, here. The assets never change after this line, so gzipping
# them per request would spend CPU on a fixed answer — and a build can afford
# -9 where a request handler cannot. `-k` keeps the original for clients that
# do not offer gzip. Roughly 2.6MB of CSS and JS becomes about 400KB.
RUN find dist -type f \( -name '*.js' -o -name '*.css' -o -name '*.html' -o -name '*.json' -o -name '*.svg' -o -name '*.map' \) \
      -exec gzip -9 -k -f {} \; \
 && echo "compressed:" && du -sh dist

FROM --platform=$BUILDPLATFORM golang:1.23 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY *.go ./
# go:embed all:ui/dist needs the built assets present at compile time
COPY --from=ui /ui/dist ./ui/dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/console .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/console /console
USER 65532:65532
ENTRYPOINT ["/console"]
