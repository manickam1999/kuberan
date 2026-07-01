.PHONY: dev build build-mcp test test-cover test-race lint check check-fast fmt migrate-up migrate-down migrate-version swagger clean

# Development
dev:
	air

build:
	go build -o bin/api ./cmd/api

build-mcp:
	go build -o bin/mcp ./cmd/mcp

# Testing
test:
	go test ./... -v -count=1 -timeout 120s

test-cover:
	go test ./... -coverprofile=coverage.out -count=1 -timeout 120s
	go tool cover -html=coverage.out -o coverage.html

test-race:
	go test ./... -race -count=1 -timeout 120s

# Code quality
lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	goimports -w .

# Verification (primary feedback loop for agents and developers)
check-fast:
	go build ./...
	go vet ./...
	golangci-lint run ./...

check:
	./scripts/check.sh

# Database
migrate-up:
	go run cmd/migrate/main.go up

migrate-down:
	go run cmd/migrate/main.go down 1

migrate-version:
	go run cmd/migrate/main.go version

# Documentation
swagger:
	swag init -g cmd/api/main.go -d . --output internal/docs --parseDependency

clean:
	rm -rf bin/ tmp/ coverage.out coverage.html
