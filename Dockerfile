# TapeBackarr Dockerfile
# Multi-stage build for minimal production image

# ============================================================================
# Stage 1: Build the Go backend
# ============================================================================
FROM golang:1.24-bookworm AS backend-builder

WORKDIR /app

# Copy go module files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/

# Build the binary
ARG VERSION=dev
ARG BUILD_TIME=""
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}" \
    -o tapebackarr ./cmd/tapebackarr

# ============================================================================
# Stage 2: Build the frontend
# ============================================================================
FROM node:20-bookworm-slim AS frontend-builder

WORKDIR /app

# Copy package files first for better caching
COPY web/frontend/package*.json ./
RUN npm ci

# Copy frontend source
COPY web/frontend/ ./

# Build the frontend
RUN npm run build

# ============================================================================
# Stage 3: Production image
# ============================================================================
FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    mt-st \
    tar \
    mbuffer \
    sg3-utils \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user (but note: tape access may require root or tape group)
RUN groupadd -r tapebackarr && useradd -r -g tapebackarr tapebackarr

# Create necessary directories
RUN mkdir -p /opt/tapebackarr \
    /etc/tapebackarr \
    /var/lib/tapebackarr \
    /var/log/tapebackarr \
    && chown -R tapebackarr:tapebackarr \
    /opt/tapebackarr \
    /var/lib/tapebackarr \
    /var/log/tapebackarr

# Copy binary from builder
COPY --from=backend-builder /app/tapebackarr /opt/tapebackarr/tapebackarr

# Copy frontend build from builder
COPY --from=frontend-builder /app/build /opt/tapebackarr/static

# Copy documentation
COPY docs/ /opt/tapebackarr/docs/

# Copy default config (will be overridden by volume mount)
COPY deploy/config.example.json /etc/tapebackarr/config.json

WORKDIR /opt/tapebackarr

# Expose the web interface port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/v1/health || exit 1

# Note: Container typically needs to run as root for tape device access
# unless proper device permissions are configured on the host
USER root

# Volume for persistent data
VOLUME ["/var/lib/tapebackarr", "/etc/tapebackarr"]

# Default command
ENTRYPOINT ["/opt/tapebackarr/tapebackarr"]
CMD ["-config", "/etc/tapebackarr/config.json"]
