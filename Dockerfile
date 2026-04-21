# ── Stage 1: Build React frontend ─────────────────────────────────────────────
FROM node:22-alpine AS frontend
WORKDIR /app
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ── Stage 2: Build Go backend ──────────────────────────────────────────────────
FROM golang:1.21-alpine AS backend
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY backend/ ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server/

# ── Stage 3: Final image ───────────────────────────────────────────────────────
FROM alpine:3.19
WORKDIR /app

# ca-certificates is needed for outbound HTTPS (Google Places API)
RUN apk add --no-cache ca-certificates && \
    addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=backend /app/server ./server
COPY --from=frontend /app/dist  ./dist

RUN chown -R appuser:appgroup /app
USER appuser

# EB routes external traffic → port 8080 via its nginx reverse proxy
EXPOSE 8080

# DATABASE_URL must be set as an environment variable at runtime
# (EB environment properties, or docker run -e).
# SEED_DATA defaults to false; set to true only for local dev/demo environments.
ENV STATIC_DIR=./dist \
    SEED_DATA=false

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

CMD ["./server"]
