FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/ergo /usr/local/bin/ergo
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/ergo"]
