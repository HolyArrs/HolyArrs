# Debian slim, because the trailer job runs yt-dlp's release program, which
# needs glibc and zlib (a scratch image cannot run it).
FROM debian:12-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587
# libwebp itself: the Go WebP library loads it at start (a 500 px picture
# encodes in 56 ms on the VM instead of 205 ms with its built-in copy).
RUN apt-get update && apt-get install -y --no-install-recommends libwebp7 libwebpdemux2 \
 && rm -rf /var/lib/apt/lists/* \
 && ln -s libwebp.so.7 /usr/lib/aarch64-linux-gnu/libwebp.so \
 && ln -s libwebpdemux.so.2 /usr/lib/aarch64-linux-gnu/libwebpdemux.so
# yt-dlp 2026.08.19 for arm64, checked against its published SHA-256. The
# trailer job updates it weekly into /data/bin (the root is read-only).
ADD --checksum=sha256:b16e4dab368a816cd05d477d698a605a6ae87ccee1c8ffd38fa21d7254141fcc --chmod=755 https://github.com/yt-dlp/yt-dlp/releases/download/2026.08.19/yt-dlp_linux_aarch64 /usr/local/bin/yt-dlp
COPY --chown=10001:10001 data /data
COPY metadata /metadata
COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY config.json /config/config.json
USER 10001:10001
ENV GOMEMLIMIT=1200MiB
ENTRYPOINT ["/metadata"]
CMD ["serve", "--config", "/config/config.json"]
