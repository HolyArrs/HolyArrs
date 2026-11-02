FROM scratch
COPY metadata /metadata
COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
USER 10001:10001
ENV GOMEMLIMIT=768MiB
ENTRYPOINT ["/metadata"]
