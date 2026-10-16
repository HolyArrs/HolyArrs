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

# Final stage
FROM scratch
ARG VERSION=dev
LABEL org.opencontainers.image.title="HolyArrs" \
      org.opencontainers.image.source="https://github.com/CoderLuii/holyarrs" \
      org.opencontainers.image.licenses="GPL-3.0-only" \
      org.opencontainers.image.version="$VERSION"
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/holyarrs /holyarrs
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
