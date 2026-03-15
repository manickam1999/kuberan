FROM alpine:3.21

# Install PostgreSQL 16 client (for pg_dump) and supercronic (cron for containers)
RUN apk add --no-cache postgresql16-client ca-certificates tzdata \
    && wget -q https://github.com/aptible/supercronic/releases/download/v0.2.33/supercronic-linux-amd64 \
       -O /usr/local/bin/supercronic \
    && chmod +x /usr/local/bin/supercronic

COPY backup.sh /backup.sh
RUN chmod +x /backup.sh

COPY crontab /etc/supercronic/crontab

# Run an initial backup on start, then hand off to supercronic
CMD ["/bin/sh", "-c", "/backup.sh; exec supercronic /etc/supercronic/crontab"]
