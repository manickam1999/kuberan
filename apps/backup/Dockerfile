FROM alpine:3.22

ARG TARGETARCH

# Use a stable Alpine mirror instead of CDN (fixes hanging APKINDEX issues)
RUN sed -i 's|dl-cdn.alpinelinux.org|mirrors.edge.kernel.org|g' /etc/apk/repositories

# Install dependencies
# rclone + curl power the off-host encrypted push to R2 (see plans/017 Phase 0).
RUN apk add --no-cache \
  postgresql17-client \
  ca-certificates \
  tzdata \
  rclone \
  curl \
  && update-ca-certificates

# Install supercronic (cron replacement for containers), verified against the
# SHA1SUMs published on the v0.2.33 release page. Bump version + sums together.
ARG SUPERCRONIC_VERSION=v0.2.33
ARG SUPERCRONIC_SHA1_AMD64=71b0d58cc53f6bd72cf2f293e09e294b79c666d8
ARG SUPERCRONIC_SHA1_ARM64=e0f0c06ebc5627e43b25475711e694450489ab00
RUN set -eu; \
  case "${TARGETARCH:-amd64}" in \
    amd64) sum="${SUPERCRONIC_SHA1_AMD64}" ;; \
    arm64) sum="${SUPERCRONIC_SHA1_ARM64}" ;; \
    *) echo "unsupported TARGETARCH ${TARGETARCH}" >&2; exit 1 ;; \
  esac; \
  wget -q "https://github.com/aptible/supercronic/releases/download/${SUPERCRONIC_VERSION}/supercronic-linux-${TARGETARCH:-amd64}" \
    -O /usr/local/bin/supercronic; \
  echo "${sum}  /usr/local/bin/supercronic" | sha1sum -c -; \
  chmod +x /usr/local/bin/supercronic

# Unprivileged runtime user. The host bind mount ./backups must be writable
# by this uid (deploy/deploy.sh chowns it); see docs/backup-and-dr.md.
ARG BACKUP_UID=10001
RUN addgroup -S -g "${BACKUP_UID}" backup \
  && adduser -S -D -H -u "${BACKUP_UID}" -G backup -s /sbin/nologin backup \
  && mkdir -p /backups \
  && chown backup:backup /backups \
  && chmod 0700 /backups

# Copy backup + restore-test scripts and their shared helpers
COPY lib.sh /usr/local/lib/kuberan-backup/lib.sh
COPY backup.sh /backup.sh
COPY restore-test.sh /restore-test.sh
RUN chmod 0755 /backup.sh /restore-test.sh \
  && chmod 0644 /usr/local/lib/kuberan-backup/lib.sh
ENV BACKUP_LIB=/usr/local/lib/kuberan-backup/lib.sh

# Copy cron definition
COPY crontab /etc/supercronic/crontab

# rclone/psql want a writable HOME; /tmp is a tmpfs in docker-compose.prod.yml.
ENV HOME=/tmp
USER backup

# Run initial backup then start cron
CMD ["/bin/sh", "-c", "/backup.sh || true; exec /usr/local/bin/supercronic /etc/supercronic/crontab"]
