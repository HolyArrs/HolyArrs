# Debian slim, for libwebp (below). Trailers, storyboards and the "most
# replayed" graph come from HolyArrs's own reader inside /metadata; no other
# program is installed for them.
FROM debian:12-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587
# libwebp itself: the Go WebP library loads it at start (a 500 px picture
# encodes in 56 ms on the VM instead of 205 ms with its built-in copy).
RUN apt-get update && apt-get install -y --no-install-recommends libwebp7 libwebpdemux2 \
 && rm -rf /var/lib/apt/lists/* \
 && ln -s libwebp.so.7 /usr/lib/aarch64-linux-gnu/libwebp.so \
 && ln -s libwebpdemux.so.2 /usr/lib/aarch64-linux-gnu/libwebpdemux.so
COPY --chown=10001:10001 data /data
COPY metadata /metadata
COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY config.json /config/config.json
USER 10001:10001
ENV GOMEMLIMIT=1200MiB
ENTRYPOINT ["/metadata"]
CMD ["serve", "--config", "/config/config.json"]
