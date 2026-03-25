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
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server/

# ── Stage 3: Final image ───────────────────────────────────────────────────────
FROM alpine:3.19
WORKDIR /app

# ca-certificates is needed for outbound HTTPS (Google Places API)
RUN apk add --no-cache ca-certificates

COPY --from=backend /app/server ./server
COPY --from=frontend /app/dist  ./dist

# EB routes external traffic → port 8080 via its nginx reverse proxy
EXPOSE 8080

# DATABASE_URL and REDIS_URL must be set as environment variables at runtime
# (EB environment properties, or docker run -e).
ENV STATIC_DIR=./dist \
    SEED_DATA=true

CMD ["./server"]
