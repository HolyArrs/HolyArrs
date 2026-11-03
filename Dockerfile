# Build stage: runs on the build machine and cross-compiles for the target,
# so multi-arch builds (amd64, arm64, armv7) need no emulation.
FROM --platform=$BUILDPLATFORM golang:1.27.2@sha256:007a316059246276cb214df93950be77837545dc78007a73e0291a4963d5d3bf AS build
ARG TARGETOS=linux
ARG TARGETARCH
ARG TARGETVARIANT
ARG VERSION
# Optional build-only resource controls; omitted values keep Go's defaults.
ARG GOMAXPROCS
ARG GOFLAGS
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN set -eu; \
    ldflags="-s -w"; \
    if [ -n "${VERSION:-}" ]; then ldflags="$ldflags -X main.version=$VERSION"; fi; \
    GOARM="${TARGETVARIANT#v}" CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="$ldflags" -o /out/holyarrs ./cmd/holyarrs
RUN mkdir -p /out/config && chown 1000:1000 /out/config
# yt-dlp for trailers: the official pinned release, checked against its
# published SHA-256 (yt-dlp's SHA2-256SUMS for 2026.08.19). It sits next to
# /holyarrs, where HolyArrs looks after HOLYARRS_YTDLP; the weekly update
# writes newer releases to /config/bin. No armv7 build exists: those images
# play trailers through the YouTube embed instead.
ARG YTDLP_VERSION=2026.08.19
RUN set -eu; mkdir -p /out/ytdlp;     case "$TARGETARCH" in       amd64) asset=yt-dlp_linux; sum=58162f9bfdc27458ea47bfcb311cf47028f17d8154a8bf7d689861d46399230a ;;       arm64) asset=yt-dlp_linux_aarch64; sum=b16e4dab368a816cd05d477d698a605a6ae87ccee1c8ffd38fa21d7254141fcc ;;       *) exit 0 ;;     esac;     curl -fsSL -o /out/ytdlp/yt-dlp "https://github.com/yt-dlp/yt-dlp/releases/download/$YTDLP_VERSION/$asset";     echo "$sum  /out/ytdlp/yt-dlp" | sha256sum -c -;     chmod 755 /out/ytdlp/yt-dlp

# Final stage: Debian slim, because the yt-dlp release program needs glibc
# and zlib (a scratch image cannot run it).
FROM debian:12-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587
ARG VERSION=dev
LABEL org.opencontainers.image.title="HolyArrs" \
      org.opencontainers.image.source="https://github.com/CoderLuii/holyarrs" \
      org.opencontainers.image.licenses="GPL-3.0-only" \
      org.opencontainers.image.version="$VERSION"
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/holyarrs /holyarrs
COPY --from=build /out/ytdlp/ /
COPY --from=build --chown=1000:1000 /out/config /config
COPY LICENSE /LICENSE
# No GOMEMLIMIT here: the program aims at 384 MiB by itself, and on a small
# board or a container limited to about 2.5 GiB or less it turns on its
# low-memory mode and aims lower (a quarter of the memory, 192-384 MiB; see
# docs/parts-and-safe-mode.md). SQLite's page caches and the driver's own heap
# sit outside that target (measured up to about 69 MiB with a 12,800-Item
# Library), so about 20 percent of a 500 MB limit stays headroom
# (releases/3.7-fault-and-memory.md). Set GOMEMLIMIT to choose the target.
USER 1000:1000
EXPOSE 18730
VOLUME ["/config"]
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 CMD ["/holyarrs", "-healthcheck"]
ENTRYPOINT ["/holyarrs"]
CMD ["-config", "/config", "-addr", ":18730"]
