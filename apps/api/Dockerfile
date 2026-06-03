# ---- Builder ----
FROM golang:1.24-alpine AS builder
WORKDIR /app

# Download dependencies first (cached layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build all binaries
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /migrate ./cmd/migrate
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /mcp ./cmd/mcp

# ---- Runtime ----
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /api /api
COPY --from=builder /migrate /migrate
COPY --from=builder /mcp /mcp
COPY --from=builder /app/migrations /migrations

EXPOSE 8080
ENTRYPOINT ["/api"]
