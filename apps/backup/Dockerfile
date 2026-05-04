FROM alpine:3.21

ARG TARGETARCH

# Use a stable Alpine mirror instead of CDN (fixes hanging APKINDEX issues)
RUN sed -i 's|dl-cdn.alpinelinux.org|mirrors.edge.kernel.org|g' /etc/apk/repositories

# Install dependencies
RUN apk add --no-cache \
  postgresql17-client \
  ca-certificates \
  tzdata \
  && update-ca-certificates

# Install supercronic (cron replacement for containers)
RUN wget -q https://github.com/aptible/supercronic/releases/download/v0.2.33/supercronic-linux-${TARGETARCH} \
  -O /usr/local/bin/supercronic \
  && chmod +x /usr/local/bin/supercronic

# Copy backup script
COPY backup.sh /backup.sh
RUN chmod +x /backup.sh

# Copy cron definition
COPY crontab /etc/supercronic/crontab

# Run initial backup then start cron
CMD ["/bin/sh", "-c", "/backup.sh || true; exec /usr/local/bin/supercronic /etc/supercronic/crontab"]
