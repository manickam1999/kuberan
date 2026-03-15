FROM alpine:3.21

# Install PostgreSQL 17 client (for pg_dump) and supercronic (cron for containers)
# Client version must be >= server version (Supabase runs PG 17)
RUN apk add --no-cache postgresql17-client ca-certificates tzdata \
    && wget -q https://github.com/aptible/supercronic/releases/download/v0.2.33/supercronic-linux-amd64 \
       -O /usr/local/bin/supercronic \
    && chmod +x /usr/local/bin/supercronic

COPY backup.sh /backup.sh
RUN chmod +x /backup.sh

COPY crontab /etc/supercronic/crontab

# Run an initial backup on start, then hand off to supercronic.
# Initial backup failure is non-fatal — supercronic will retry on schedule.
CMD ["/bin/sh", "-c", "/backup.sh || true; exec supercronic /etc/supercronic/crontab"]
