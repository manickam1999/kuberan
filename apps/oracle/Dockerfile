FROM golang:1.26-alpine3.22 AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /oracle ./main.go

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=builder /oracle /oracle
ENTRYPOINT ["/oracle"]
