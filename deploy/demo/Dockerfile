# Demo reset job: psql + the migrate binary + curl/jq, scheduled with
# supercronic (same pattern and pinned release as apps/backup).
# Build with the api source as the "api" named context (docker-compose.demo.yml).

FROM golang:1.26-alpine3.22 AS migrate
WORKDIR /app
COPY --from=api go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY --from=api . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM alpine:3.22
ARG TARGETARCH
RUN apk add --no-cache postgresql17-client curl jq ca-certificates tzdata

# Same supercronic release and checksums as apps/backup/Dockerfile. Bump together.
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

RUN addgroup -S -g 10002 demoreset && adduser -S -D -H -u 10002 -G demoreset -s /sbin/nologin demoreset

COPY --from=migrate /out/migrate /migrate
COPY --from=api migrations /migrations
COPY reset.sh /reset.sh
COPY crontab /etc/supercronic/crontab
RUN chmod 0755 /reset.sh /migrate

# The migrate binary resolves file://migrations relative to the working directory.
WORKDIR /
ENV HOME=/tmp
USER demoreset

# Reset once at start (the database is empty on first boot), then nightly.
CMD ["/bin/sh", "-c", "/reset.sh || true; exec /usr/local/bin/supercronic /etc/supercronic/crontab"]
