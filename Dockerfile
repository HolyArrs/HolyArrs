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

# 7-Zip for RAR sets the built-in decoder cannot read (#54, Luis's yes,
# 2026-11-03): the official static build (7zzs, no libraries needed) of
# 7-Zip 26.04, checked against SHA-256 sums taken from its release files,
# put on the PATH as 7zz with its licence beside it (xz unpacks the release
# file here in the build stage only). armv7 images rely on
# the built-in decoder alone.
ARG SEVENZIP_VERSION=26.04
RUN set -eu; mkdir -p /out/7zip/usr/local/bin /out/7zip/usr/share/doc/7zip;     case "$TARGETARCH" in       amd64) asset=linux-x64; sum=fc0327ba27e89bd086cf426dff17d77de582953cdbbc10a6576540a06853ffcd ;;       arm64) asset=linux-arm64; sum=5b0ac3aa91c1e3499f011d6af69d7f6b4de3ef2f011d9e2ff44c58ebd04aa388 ;;       *) exit 0 ;;     esac;     command -v xz >/dev/null || { apt-get update -qq && apt-get install -y -qq --no-install-recommends xz-utils >/dev/null; };     v=$(echo "$SEVENZIP_VERSION" | tr -d .);     curl -fsSL -o /tmp/7z.tar.xz "https://github.com/ip7z/7zip/releases/download/$SEVENZIP_VERSION/7z$v-$asset.tar.xz";     echo "$sum  /tmp/7z.tar.xz" | sha256sum -c -;     tar -xJf /tmp/7z.tar.xz -C /tmp 7zzs License.txt;     install -m 755 /tmp/7zzs /out/7zip/usr/local/bin/7zz;     install -m 644 /tmp/License.txt /out/7zip/usr/share/doc/7zip/License.txt;     rm -f /tmp/7z.tar.xz /tmp/7zzs /tmp/License.txt

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
COPY --from=build /out/7zip/ /
COPY --from=build --chown=1000:1000 /out/config /config
COPY LICENSE /LICENSE
# Under a 500 MB (476.8 MiB) container limit: SQLite's page caches (up to
# 8 connections x 8000 KiB) and the driver's own heap sit outside what
# GOMEMLIMIT sees (measured up to about 69 MiB with a 12,800-Item Library).
# That is not a hard ceiling: SQLite's sorter and temporary memory are not
# capped, so about 20 percent is left as headroom
# (releases/3.7-fault-and-memory.md).
ENV GOMEMLIMIT=384MiB
USER 1000:1000
EXPOSE 18730
VOLUME ["/config"]
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 CMD ["/holyarrs", "-healthcheck"]
ENTRYPOINT ["/holyarrs"]
CMD ["-config", "/config", "-addr", ":18730"]
