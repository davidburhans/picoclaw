# ============================================================
# Stage 1: Build the picoclaw binary
# ============================================================
FROM golang:1.26.0-alpine AS builder

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

RUN apk add --no-cache ca-certificates tzdata git make ffmpeg curl wget nodejs npm jq

# Install MCP CLI, httpx and uv (for uvx support)
RUN pip install --no-cache-dir "mcp[cli]" httpx uv

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q --spider http://localhost:18790/health || exit 1

# Copy binary
COPY --from=builder /src/build/picoclaw /usr/local/bin/picoclaw

# Create picoclaw home directory
RUN /usr/local/bin/picoclaw onboard

ENTRYPOINT ["picoclaw"]
CMD ["gateway"]
