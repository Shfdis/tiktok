# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY *.go ./

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o bot .

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Default to docker-compose service DNS name; can be overridden.
ENV BOT_BASE_URL=http://backend:8080

COPY --from=builder /app/bot /app/bot

CMD ["/app/bot"]


