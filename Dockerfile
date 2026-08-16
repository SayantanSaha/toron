# ==============================================================================
# 👑 Toron Web Server & Edge Gateway — Multi-Stage Docker Container Build
# Stage 1: Build static Go binary with CGO disabled
# Stage 2: Minimal, secure production execution appliance (~15 MB)
# ==============================================================================

# ------------------------------------------------------------------------------
# Stage 1: Builder
# ------------------------------------------------------------------------------
FROM golang:alpine AS builder

WORKDIR /src

# Copy go.mod
COPY go.mod ./

# Copy full source tree
COPY . .

# Build static Linux binary (CGO_ENABLED=0)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /src/bin/toron ./cmd/toron

# ------------------------------------------------------------------------------
# Stage 2: Production Appliance
# ------------------------------------------------------------------------------
FROM alpine:latest AS runner

# Install TLS CA certificates and timezone data
RUN apk --no-cache add ca-certificates tzdata

# Set working directory for configuration and static assets
WORKDIR /etc/toron

# Copy compiled binary from builder
COPY --from=builder /src/bin/toron /usr/local/bin/toron

# Copy default config, routes, and dashboard static assets
COPY config.yaml /etc/toron/config.yaml
COPY routes.yaml /etc/toron/routes.yaml
COPY public/ /etc/toron/public/

# Expose standard Toron Edge Gateway ports
# 8080: HTTP Gateway & Web Control Center Dashboard
# 8443: HTTPS Gateway (mTLS / ACME)
# 8090: Layer 4 TCP Proxy Stream
# 8091: Layer 4 UDP Proxy Stream
EXPOSE 8080 8443 8090 8091

# Declare configuration and socket volume mount points
VOLUME ["/etc/toron", "/var/run/docker.sock"]

# Default entrypoint
ENTRYPOINT ["/usr/local/bin/toron", "-config", "/etc/toron/config.yaml"]
