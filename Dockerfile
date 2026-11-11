# syntax=docker/dockerfile:1.7
# ---------- 1. build the web UI ----------
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---------- 2. build the Go binary ----------
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
COPY third_party/pocket-core ./third_party/pocket-core
RUN cd third_party/pocket-core && sha256sum -c SHA256SUMS
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/pocket-karaoke ./cmd/pocket-karaoke

# ---------- 3. runtime ----------
FROM alpine:3.21
# Song files: ffmpeg/ffprobe (length, tags, pictures); keeping copies of YouTube songs: yt-dlp
# (python3; our own copy in /data/bin is updated daily) with node for YouTube's signature puzzles.
# All optional: without them the app still runs (DOWNLOADS=off skips yt-dlp altogether).
RUN apk add --no-cache ca-certificates tzdata curl ffmpeg python3 nodejs \
 && addgroup -g 1000 app && adduser -u 1000 -G app -D -h /data app \
 && mkdir -p /usr/local/share/pocket-karaoke \
 && curl -fsSL -o /usr/local/share/pocket-karaoke/yt-dlp https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp \
 && chmod 755 /usr/local/share/pocket-karaoke/yt-dlp
COPY --from=build /out/pocket-karaoke /usr/local/bin/pocket-karaoke
ENV DATA_DIR=/data PORT=8080 HOST=0.0.0.0
VOLUME ["/data"]
EXPOSE 8080
USER app
WORKDIR /data
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD curl -fsS http://localhost:8080/healthz || exit 1
LABEL org.opencontainers.image.title="Pocket Karaoke" \
      org.opencontainers.image.description="Karaoke at home: songs from uploads (MP3, MP4, MP3+CDG), YouTube and Pocket Music with timed words, key change, less voice, scores and recordings; karaoke nights with a big screen and guests on their phones. One Go binary, one SQLite file, one volume." \
      org.opencontainers.image.source="https://github.com/abjelosevic88/pocket-homelab-karaoke" \
      org.opencontainers.image.licenses="MIT"
ENTRYPOINT ["pocket-karaoke"]
