# Web stage: builds the app's pages from this commit's source and lockfile,
# so an image from any commit always shows that commit's pages. It runs on
# the build machine for every target; Node never reaches the final image.
FROM --platform=$BUILDPLATFORM node:24.22.1-trixie-slim@sha256:43b9ba1142b8dcf33fa8f922ca1be85714adc444e0586c7cf7726daaeabc1bea AS web
WORKDIR /src/app/frontend
COPY app/frontend/package.json app/frontend/package-lock.json ./
RUN npm ci --ignore-scripts --no-audit --no-fund
COPY app/frontend/ ./
# The language files stay plain here; the build stage packs them with bzip2.
RUN HOLYARRS_LANGUAGES_PLAIN=1 npm run build

# Build stage: runs on the build machine and cross-compiles for the target,
# so multi-arch builds (amd64, arm64, armv7) need no emulation.
FROM --platform=$BUILDPLATFORM golang:1.27.4@sha256:08bcb99a38f85a265f9a93d1d3323e2440eec333380f3ea26ee0dec9264bb41c AS build
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
# The pages just built are what the program embeds.
RUN rm -rf app/frontend/dist
COPY --from=web /src/app/frontend/dist ./app/frontend/dist
# bzip2 is smaller than brotli for every language file (Python is part of the
# Go image; the program unpacks with Go's own bzip2 reader).
RUN python3 deploy/pack_languages.py app/frontend/dist
# Inlining is off in HolyArrs's own packages and in the parts of Go's library
# and the sign-in libraries that are not speed-critical (about 1.3 MB less to
# download, same measured speed). The database, text handling, unpacking and
# database/sql keep it, where it matters for speed.
RUN set -eu; \
    ldflags="-s -w"; \
    if [ -n "${VERSION:-}" ]; then ldflags="$ldflags -X main.version=$VERSION"; fi; \
    noinline=""; \
    for p in github.com/HolyArrs/holyarrs/... \
        net/http/... crypto/tls/... crypto/x509/... net net/url net/textproto net/mail net/netip net/smtp mime/... \
        os os/exec os/signal os/user log/... encoding/asn1 encoding/pem encoding/csv archive/... vendor/... \
        io/fs path/filepath html/... text/... flag embed \
        github.com/go-jose/... github.com/coreos/... golang.org/x/oauth2/... github.com/spf13/... go4.org/... \
        github.com/google/... github.com/dustin/... github.com/hashicorp/...; do \
        noinline="$noinline -gcflags=$p=-l"; \
    done; \
    GOARM="${TARGETVARIANT#v}" CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath $noinline -ldflags="$ldflags" -o /out/holyarrs ./app/backend/cmd/holyarrs
# The folders the bare image needs: /config for the owner (1000:1000) and a
# /tmp anyone may write (Move's working copies, temporary files).
RUN mkdir -p /out/config /out/tmp && chown 1000:1000 /out/config && chmod 1777 /out/tmp

# Final stage: an empty image with only HolyArrs, CA certificates and the
# licences. No shell and no system tools: HolyArrs is one static Go program
# with its own time zone database, RAR/7z/zip reader and healthcheck.
FROM scratch
ARG VERSION=dev
LABEL org.opencontainers.image.title="HolyArrs" \
      org.opencontainers.image.source="https://github.com/HolyArrs/HolyArrs" \
      org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.version="$VERSION"
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/holyarrs /holyarrs
COPY --from=build --chmod=1777 /out/tmp /tmp
COPY --from=build --chown=1000:1000 /out/config /config
COPY LICENSE /LICENSE
# No GOMEMLIMIT here: the program aims at 384 MiB by itself, and on a small
# machine or a container limited to about 2.5 GiB or less it turns on its
# low-memory mode and aims lower (a quarter of the memory, 192-384 MiB; see
# docs/run/parts-safe-mode.md). SQLite's page caches and the driver's own heap
# sit outside that target (measured up to about 69 MiB with a 12,800-Item
# Library), so about 20 percent of a 500 MB limit stays headroom.
# Set GOMEMLIMIT to choose the target.
# No USER line: the image starts as root, gives /config to PUID:PGID (default
# 1000:1000) when its owner differs, then restarts itself as that user and
# never keeps running as root. Run with user: "<uid>:<gid>" to skip the switch.
# container=oci marks this as a container on every runtime: Docker and Podman
# leave marker files, but containerd (Kubernetes, k3s, nerdctl) does not, and
# outside a container a root start without PUID/PGID is refused.
ENV container=oci
EXPOSE 6262
VOLUME ["/config"]
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 CMD ["/holyarrs", "-healthcheck"]
ENTRYPOINT ["/holyarrs"]
# No -addr here, so HOLYARRS_ADDR or HOLYARRS_PORT can move the port (default 6262).
CMD ["-config", "/config"]
