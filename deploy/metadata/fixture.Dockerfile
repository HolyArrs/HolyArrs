FROM scratch
COPY fixture /fixture
USER 10001:10001
ENTRYPOINT ["/fixture"]
