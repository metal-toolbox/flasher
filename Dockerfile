FROM alpine:latest

ENTRYPOINT ["/usr/sbin/flasher"]

COPY flasher /usr/sbin/flasher
RUN chmod +x /usr/sbin/flasher
