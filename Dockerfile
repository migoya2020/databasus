# ========= BUILD FRONTEND =========
FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend-build

WORKDIR /frontend

ARG APP_VERSION=dev
ENV VITE_APP_VERSION=$APP_VERSION

COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile
COPY frontend/ ./

RUN if [ ! -f .env ]; then \
  if [ -f .env.production.example ]; then \
  cp .env.production.example .env; \
  fi; \
  fi

RUN pnpm build


# ========= BUILD BACKEND =========
FROM --platform=$BUILDPLATFORM golang:1.26.3 AS backend-build

ARG TARGETOS
ARG TARGETARCH

RUN git clone --depth 1 --branch v3.27.1 https://github.com/pressly/goose.git /tmp/goose && \
  cd /tmp/goose/cmd/goose && \
  GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
  go build -o /usr/local/bin/goose . && \
  rm -rf /tmp/goose
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.4

WORKDIR /app

COPY backend/go.mod backend/go.sum ./
RUN go mod download

RUN mkdir -p /app/ui/build
COPY --from=frontend-build /frontend/dist /app/ui/build

COPY backend/ ./
RUN swag init -d . -g cmd/main.go -o swagger

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
RUN CGO_ENABLED=0 \
  GOOS=$TARGETOS \
  GOARCH=$TARGETARCH \
  go build -o /app/main ./cmd/main.go


# ========= BUILD AGENT =========
FROM --platform=$BUILDPLATFORM golang:1.26.3 AS agent-build

ARG APP_VERSION=dev

WORKDIR /agent

COPY agent/backup/go.mod ./
RUN go mod download

COPY agent/backup/ ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags "-X main.Version=${APP_VERSION}" \
    -o /agent-binaries/databasus-agent-linux-amd64 ./cmd/main.go

RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
    go build -ldflags "-X main.Version=${APP_VERSION}" \
    -o /agent-binaries/databasus-agent-linux-arm64 ./cmd/main.go


# ========= RUNTIME =========
FROM debian:bookworm-slim

ARG APP_VERSION=dev
ARG TARGETARCH
LABEL org.opencontainers.image.version=$APP_VERSION
ENV APP_VERSION=$APP_VERSION
ENV CONTAINER_ARCH=$TARGETARCH
ENV ENV_MODE=production

# Install only what's needed — no postgresql-17, no postgres server
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
      wget ca-certificates gnupg lsb-release sudo gosu curl unzip xz-utils \
      libncurses5 libncurses6 rclone \
      libmariadb3 \
      valkey; \
    rm -rf /var/lib/apt/lists/*

# Pre-built DB client binaries (PG, MySQL, MariaDB, MongoDB)
ARG TARGETARCH
RUN --mount=type=bind,source=assets/tools,target=/ctx/tools,readonly \
    mkdir -p /app/assets/tools && \
    if [ "$TARGETARCH" = "amd64" ]; then \
      cp -r /ctx/tools/x64 /app/assets/tools/x64; \
    elif [ "$TARGETARCH" = "arm64" ]; then \
      cp -r /ctx/tools/arm /app/assets/tools/arm; \
    fi && \
    chmod +x /app/assets/tools/*/postgresql/*/bin/* \
             /app/assets/tools/*/mysql/*/bin/* \
             /app/assets/tools/*/mariadb/*/bin/* \
             /app/assets/tools/*/mongodb/bin/*

# Create non-root user for the main application process
RUN useradd -r -s /usr/sbin/nologin -u 65532 databasus

WORKDIR /app

COPY --from=backend-build /usr/local/bin/goose /usr/local/bin/goose
COPY --from=backend-build /app/main .
COPY backend/migrations ./migrations
COPY --from=backend-build /app/ui/build ./ui/build
COPY frontend/cloud-root-content.html /app/cloud-root-content.html
COPY --from=agent-build /agent-binaries ./agent-binaries
COPY .env.example /.env

# Create startup script
COPY <<EOF /app/start.sh
#!/bin/bash
set -e

# ========= Validate required external database config =========
if [ -z "\${DANGEROUS_EXTERNAL_DATABASE_DSN:-}" ]; then
    echo ""
    echo "=========================================="
    echo "ERROR: DANGEROUS_EXTERNAL_DATABASE_DSN is not set!"
    echo "=========================================="
    echo ""
    echo "This image requires an external PostgreSQL database."
    echo "Please set the following environment variable:"
    echo ""
    echo "  DANGEROUS_EXTERNAL_DATABASE_DSN=postgres://user:password@host:5432/databasus?sslmode=disable"
    echo ""
    echo "=========================================="
    exit 1
fi

echo "Using external PostgreSQL database."

# Generate runtime configuration for frontend
echo "Generating runtime configuration..."

# Detect if email is configured
if [ -n "\${SMTP_HOST:-}" ] && [ -n "\${DATABASUS_URL:-}" ]; then
  IS_EMAIL_CONFIGURED="true"
else
  IS_EMAIL_CONFIGURED="false"
fi

cat > /app/ui/build/runtime-config.js <<JSEOF
// Runtime configuration injected at container startup
window.__RUNTIME_CONFIG__ = {
  IS_CLOUD: '\${IS_CLOUD:-false}',
  GITHUB_CLIENT_ID: '\${GITHUB_CLIENT_ID:-}',
  GOOGLE_CLIENT_ID: '\${GOOGLE_CLIENT_ID:-}',
  IS_EMAIL_CONFIGURED: '\$IS_EMAIL_CONFIGURED',
  CLOUDFLARE_TURNSTILE_SITE_KEY: '\${CLOUDFLARE_TURNSTILE_SITE_KEY:-}',
  CONTAINER_ARCH: '\${CONTAINER_ARCH:-unknown}',
  CLOUD_PRICE_PER_GB: '\${CLOUD_PRICE_PER_GB:-}',
  CLOUD_PADDLE_CLIENT_TOKEN: '\${CLOUD_PADDLE_CLIENT_TOKEN:-}'
};
JSEOF

# Inject analytics script if provided
if [ -n "\${ANALYTICS_SCRIPT:-}" ]; then
  if ! grep -q "rybbit.databasus.com" /app/ui/build/index.html 2>/dev/null; then
    echo "Injecting analytics script..."
    sed -i "s#</head>#  \${ANALYTICS_SCRIPT}\\
  </head>#" /app/ui/build/index.html
  fi
fi

# Inject Paddle script if client token is provided
if [ -n "\${CLOUD_PADDLE_CLIENT_TOKEN:-}" ]; then
  if ! grep -q "cdn.paddle.com" /app/ui/build/index.html 2>/dev/null; then
    echo "Injecting Paddle script..."
    sed -i "s#</head>#  <script src=\"https://cdn.paddle.com/paddle/v2/paddle.js\"></script>\\
  </head>#" /app/ui/build/index.html
  fi
fi

# Inject static HTML into root div for cloud mode
if [ "\${IS_CLOUD:-false}" = "true" ]; then
  if ! grep -q "cloud-static-content" /app/ui/build/index.html 2>/dev/null; then
    echo "Injecting cloud static HTML content..."
    perl -i -pe '
      BEGIN {
        open my \$fh, "<", "/app/cloud-root-content.html" or die;
        local \$/;
        \$c = <\$fh>;
        close \$fh;
        \$c =~ s/\\n/ /g;
      }
      s/<div id="root"><\\/div>/<div id="root"><!-- cloud-static-content --><noscript>\$c<\\/noscript><\\/div>/
    ' /app/ui/build/index.html
  fi
fi

# Set up data directories
echo "Setting up data directory permissions..."
mkdir -p /databasus-data/temp
mkdir -p /databasus-data/backups
chown -R databasus:databasus /databasus-data/temp /databasus-data/backups
chown databasus:databasus /databasus-data/secret.key /databasus-data/instance.json 2>/dev/null || true
chmod 700 /databasus-data/temp

# ========= Start Valkey (internal cache) =========
echo "Configuring Valkey cache..."
cat > /tmp/valkey.conf << 'VALKEY_CONFIG'
port 6379
bind 127.0.0.1
protected-mode yes
save ""
maxmemory 256mb
maxmemory-policy allkeys-lru
VALKEY_CONFIG

echo "Starting Valkey..."
valkey-server /tmp/valkey.conf &
VALKEY_PID=\$!

echo "Waiting for Valkey to be ready..."
for i in {1..30}; do
    if valkey-cli ping >/dev/null 2>&1; then
        echo "Valkey is ready!"
        break
    fi
    sleep 1
done

# Start the main application
echo "Starting Databasus application..."
exec gosu databasus ./main
EOF

LABEL org.opencontainers.image.source="https://github.com/databasus/databasus"

RUN chmod +x /app/start.sh

EXPOSE 4005

VOLUME ["/databasus-data"]

ENTRYPOINT ["/app/start.sh"]
CMD []
