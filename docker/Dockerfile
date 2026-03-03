# ============================================================
# Stage 1: Build the picoclaw binary
# ============================================================
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN make build

# ============================================================
# Stage 2: Minimal runtime image
# ============================================================
FROM python:3.14-alpine
RUN apk add --no-cache bitcoin

RUN apk add --no-cache ca-certificates tzdata git make ffmpeg curl wget nodejs npm jq github-cli go gcc musl-dev openssl-dev pkgconfig rust cargo ripgrep podman crun doas fuse-overlayfs

# Configure storage driver for rootless podman inside containers
RUN mkdir -p /etc/containers && \
    printf '[storage]\ndriver = "vfs"\n' > /etc/containers/storage.conf

# Install MCP CLI, httpx and uv (for uvx support)
RUN pip install --no-cache-dir "mcp[cli]" httpx uv

# Health check (port can be overridden per-container via env)
ENV HEALTH_PORT=18790
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q --spider http://localhost:${HEALTH_PORT}/health || exit 1

# Copy binary
COPY --from=builder /src/build/picoclaw /usr/local/bin/picoclaw

# Create non-root user and group
RUN addgroup -g 1000 picoclaw && addgroup docker && \
    adduser -D -u 1000 -G picoclaw picoclaw && adduser picoclaw docker

# Configure doas access (nopass for non-interactive container use)
RUN mkdir -p /etc/doas.d && \
    echo 'permit nopass :picoclaw' > /etc/doas.d/20-picoclaw.conf && \
    chmod 400 /etc/doas.d/20-picoclaw.conf

# Configure subuid/subgid for rootless podman user namespaces
RUN echo 'picoclaw:100000:65536' > /etc/subuid && \
    echo 'picoclaw:100000:65536' > /etc/subgid


# Copy entrypoint wrapper (handles podman-in-container setup)
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN sed -i 's/\r$//' /usr/local/bin/entrypoint.sh && chmod +x /usr/local/bin/entrypoint.sh

# Switch to non-root user
USER picoclaw

ENV USE_BUILTIN_RIPGREP=0

# Run onboard to create initial directories and config
RUN /usr/local/bin/picoclaw onboard

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
