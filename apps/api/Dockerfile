# Requires BuildKit (default for `docker compose build` / Docker 23+) for the
# cache mounts below.

# ---- Builder ----
FROM golang:1.26-alpine3.22 AS builder
WORKDIR /app

# Download dependencies first (cached layer + persistent module cache)
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source and build all binaries (module + build caches survive rebuilds)
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mcp ./cmd/mcp

# ---- Runtime ----
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 kuberan \
    && adduser -S -D -H -u 10001 -G kuberan -s /sbin/nologin kuberan

COPY --from=builder /out/api /api
COPY --from=builder /out/migrate /migrate
COPY --from=builder /out/mcp /mcp
COPY --from=builder /app/migrations /migrations

# api, mcp and migrate all run unprivileged; they only listen on high ports
# and read /migrations. The migrate binary resolves file://migrations
# relative to the working directory, so keep WORKDIR at /.
WORKDIR /
USER 10001:10001

EXPOSE 8080
ENTRYPOINT ["/api"]
