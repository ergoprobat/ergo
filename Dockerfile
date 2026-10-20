FROM gcr.io/distroless/static-debian12:nonroot
COPY ergo /usr/local/bin/ergo
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/ergo"]
