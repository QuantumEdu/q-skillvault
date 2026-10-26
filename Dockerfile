# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary (CGO disabled for zero-dependency portability)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/skillvault ./cmd/skillvault

# Minimal runtime image
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata && \
    mkdir -p /root/.skillvault

WORKDIR /app
COPY --from=builder /build/skillvault /app/skillvault

# Environment defaults
ENV HOME=/root
ENV SKILLVAULT_HOME=/root/.skillvault

# Stdio MCP server entrypoint
ENTRYPOINT ["/app/skillvault", "mcp"]
