FROM --platform=$BUILDPLATFORM golang:1.27.2@sha256:007a316059246276cb214df93950be77837545dc78007a73e0291a4963d5d3bf AS build
ARG TARGETOS=linux
ARG TARGETARCH
WORKDIR /src
COPY app/backend/cmd/holyarrs-companion ./cmd/holyarrs-companion
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /companion ./cmd/holyarrs-companion/main.go
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /companion /companion
USER 1000:1000
ENTRYPOINT ["/companion"]
