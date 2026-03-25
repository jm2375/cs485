# Backend — TripPlanner API Server

Go/Gin HTTP server that provides the REST API, WebSocket hub, and database layer for TripPlanner.

---

## Table of Contents

1. [Architecture overview](#architecture-overview)
2. [External dependencies](#external-dependencies)
3. [Databases](#databases)
4. [Environment variables](#environment-variables)
5. [Install](#install)
6. [Start](#start)
7. [Stop](#stop)
8. [Reset data](#reset-data)
9. [Running tests](#running-tests)
10. [Production (AWS Elastic Beanstalk)](#production-aws-elastic-beanstalk)

---

## Architecture overview

```
cmd/server/main.go          entry point — wires everything together
internal/config/            reads environment variables into a Config struct
internal/db/                PostgreSQL connection, schema migration, seed data
internal/cache/             in-memory TTL cache (rate limiting, invite tokens)
internal/models/            shared domain types (Trip, User, Role, …)
internal/middleware/        JWT authentication middleware
internal/services/          business logic (auth, trips, POIs, invitations, …)
internal/handlers/          Gin HTTP handlers (one file per resource)
internal/websocket/         WebSocket hub — broadcasts real-time events to trip rooms
```

The server listens on port **8080** (configurable via `PORT`). In production the compiled React frontend is served as static files from the same process; in development the Vite dev server runs separately and proxies API calls to `:8080`.

---

## External dependencies

### Required at runtime

| Dependency | Version | Purpose |
|---|---|---|
| **PostgreSQL** | 15 or 16 | Primary data store. All user, trip, collaborator, invitation, POI, and itinerary data is persisted here. The server will refuse to start if it cannot reach the database. |

### Optional at runtime

| Dependency | Purpose | Configured via |
|---|---|---|
| **Google Places API** | Searches for real points of interest when a user queries an unknown location. Without an API key the server falls back to returning only locally cached POIs from the database. | `API_KEY` env var |

### Go library dependencies (direct)

| Library | Version | Purpose |
|---|---|---|
| `github.com/gin-gonic/gin` | v1.9.1 | HTTP router and middleware framework |
| `github.com/golang-jwt/jwt/v5` | v5.2.0 | JWT generation and validation for session tokens |
| `github.com/google/uuid` | v1.6.0 | UUID generation for all primary keys |
| `github.com/gorilla/websocket` | v1.5.1 | WebSocket upgrade and connection management |
| `github.com/lib/pq` | v1.12.0 | PostgreSQL driver for `database/sql` |
| `golang.org/x/crypto` | v0.21.0 | bcrypt password hashing |
| `modernc.org/sqlite` | v1.29.0 | SQLite driver — used only by the integration test suite, not by the running server |

All indirect dependencies are pinned in `go.sum` and fetched automatically by `go mod download`.

### Build-time toolchain

| Tool | Minimum version | Purpose |
|---|---|---|
| **Go** | 1.21 | Compile the server binary |
| **Docker** | 24+ | Run PostgreSQL locally via `docker compose` |
| **Docker Compose** | v2 | Orchestrate the local PostgreSQL container |

---

## Databases

### PostgreSQL (primary store)

The server connects to a single PostgreSQL database. On startup it automatically runs `CREATE TABLE IF NOT EXISTS` migrations — no separate migration tool is required.

#### Tables created and owned by this service

| Table | Reads | Writes | Description |
|---|---|---|---|
| `users` | ✓ | ✓ | Registered accounts. Stores email, display name, and bcrypt password hash. |
| `trips` | ✓ | ✓ | Collaborative trip records. Each trip has a unique `invite_code` used for share links. |
| `trip_collaborators` | ✓ | ✓ | Join table between users and trips. Stores role (`OWNER`, `EDITOR`, `VIEWER`) and join timestamp. |
| `invitations` | ✓ | ✓ | Pending, accepted, and revoked email invitations. Stores a SHA-256 token hash (never the raw token), role, and expiry. |
| `points_of_interest` | ✓ | ✓ | POI catalogue. Seeded with Tokyo demo data; supplemented by Google Places API results cached on first fetch. |
| `itinerary_items` | ✓ | ✓ | POIs added to a trip's itinerary, with day number and display order. |

#### Indexes

```sql
idx_inv_trip_status    ON invitations(trip_id, status)
idx_inv_email_status   ON invitations(invitee_email, status)
idx_collab_trip        ON trip_collaborators(trip_id)
idx_itinerary_trip     ON itinerary_items(trip_id)
idx_poi_category       ON points_of_interest(category)
```

#### In-memory cache (not a persistent store)

An in-process TTL cache (`internal/cache`) is used for:
- Invitation rate limiting (max 20 invitations per trip per hour)

This cache is **not persisted** — it resets on every server restart. It does not require any external service.

---

## Environment variables

Create a file named `.env` in the `backend/` directory. The server reads it on startup and does not override variables already set in the process environment.

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | Yes | `postgres://tripplanner:tripplanner@localhost:5432/tripplanner?sslmode=disable` | Full PostgreSQL connection string. Format: `postgres://user:password@host:port/dbname?sslmode=disable\|require` |
| `JWT_SECRET` | Yes | `dev-secret-change-in-production` | Secret used to sign JWT tokens. Use a long random string in production. Rotating this value invalidates all existing sessions. |
| `FRONTEND_URL` | Yes | `http://localhost:5173` | Origin of the frontend. Used for CORS `Access-Control-Allow-Origin` and as the base URL in invitation email links. In production set this to the public domain. |
| `PORT` | No | `8080` | TCP port the HTTP server listens on. |
| `SEED_DATA` | No | `true` | When `true`, seeds a demo user set and Tokyo trip on first startup. The seed is idempotent — it checks for existing data before inserting. Set to `false` in production if you do not want demo data. |
| `API_KEY` | No | _(empty)_ | Google Places API key. Enables live POI search. Without this key, search returns only locally cached POIs. |
| `STATIC_DIR` | No | _(empty)_ | Path to a compiled React build (`dist/`). When set, the server serves the frontend from this directory and handles React Router with a catch-all route. Leave empty in local development (Vite serves the frontend separately). |

**Example `.env` for local development:**

```env
DATABASE_URL=postgres://tripplanner:tripplanner@localhost:5432/tripplanner?sslmode=disable
JWT_SECRET=dev-secret-change-in-production
FRONTEND_URL=http://localhost:5173
SEED_DATA=true
```

---

## Install

### 1. Install Go

Download and install Go 1.21 or later from https://go.dev/dl/.

Verify:
```bash
go version
# go version go1.21.x ...
```

### 2. Install Docker Desktop

Download from https://www.docker.com/products/docker-desktop/. Docker is used to run PostgreSQL locally.

### 3. Fetch Go dependencies

```bash
cd backend/
go mod download
```

---

## Start

### Local development

**Step 1 — Start PostgreSQL**

From the repository root (where `docker-compose.yml` lives):

```bash
docker compose up -d
```

This starts a PostgreSQL 16 container on `localhost:5432` with:
- Database: `tripplanner`
- Username: `tripplanner`
- Password: `tripplanner`

Wait a few seconds for the container health check to pass, then verify:

```bash
docker compose ps
# postgres   running (healthy)
```

**Step 2 — Configure environment**

Copy or create `backend/.env`:

```bash
cp backend/.env.example backend/.env   # if an example exists
# or create manually — see Environment variables section above
```

**Step 3 — Start the server**

```bash
cd backend/
go run ./cmd/server/
```

Expected startup output:

```
[db] demo seed complete
[main] server listening on :8080  (frontend: http://localhost:5173)
```

The server is ready when that line appears. The health check endpoint is available at:

```bash
curl http://localhost:8080/health
# {"status":"ok","time":"..."}
```

---

## Stop

**Stop the Go server:**
Press `Ctrl+C` in the terminal where it is running. The process exits cleanly.

**Stop PostgreSQL:**

```bash
# From the repository root
docker compose stop
```

This stops the container but preserves the data volume (`postgres_data`). Data survives the stop.

**Stop and remove the container (keeps data volume):**

```bash
docker compose down
```

---

## Reset data

### Soft reset — clear application data, keep schema

Connect to the database and truncate all tables:

```bash
docker compose exec postgres psql -U tripplanner -d tripplanner -c "
  TRUNCATE itinerary_items, invitations, trip_collaborators, trips, points_of_interest, users RESTART IDENTITY CASCADE;
"
```

Then restart the server. With `SEED_DATA=true`, the demo data will be re-inserted on startup.

### Hard reset — destroy the database entirely

```bash
# From the repository root
docker compose down -v
```

The `-v` flag removes the `postgres_data` named volume, permanently deleting all data.

Bring everything back up fresh:

```bash
docker compose up -d
cd backend/
go run ./cmd/server/
```

The server will recreate the schema and re-seed demo data on startup.

### Reset demo data only (keep real user accounts)

If you want to reset only the demo Tokyo trip and seed users without touching other accounts:

```bash
docker compose exec postgres psql -U tripplanner -d tripplanner -c "
  DELETE FROM users WHERE email IN (
    'sarah.chen@example.com',
    'david.lee@example.com',
    'emily.sato@example.com',
    'kenji.tanaka@example.com',
    'maria.r@example.com'
  );
"
```

Restarting the server with `SEED_DATA=true` will re-create the demo dataset.

---

## Running tests

The test suite uses an in-memory SQLite database — no PostgreSQL or Docker required.

```bash
cd backend/
go test ./...
```

Run with verbose output to see individual test names:

```bash
go test -v ./...
```

Run a specific test:

```bash
go test -v -run TestFullInvitationAcceptFlow ./...
```

> **Note:** The integration tests in `integration_test.go` are in package `main_test` and test all HTTP endpoints end-to-end, including auth, trips, collaborators, invitations, POI search, and itinerary management.

---

## Production (AWS Elastic Beanstalk)

The production deployment uses the multi-stage `Dockerfile` at the repository root. It builds the React frontend and Go binary in a single image, served by the Go process on port 8080 behind EB's nginx reverse proxy.

### Required environment properties (set in EB Console → Configuration → Environment Properties)

| Variable | Value |
|---|---|
| `DATABASE_URL` | Full RDS PostgreSQL connection string, e.g. `postgres://user:pass@host.rds.amazonaws.com:5432/dbname?sslmode=require` |
| `JWT_SECRET` | A long random secret — never the default dev value |
| `FRONTEND_URL` | The EB environment URL, e.g. `http://your-env.elasticbeanstalk.com` |
| `SEED_DATA` | `false` for production, `true` if demo data is desired |
| `API_KEY` | Google Places API key (optional) |

### RDS requirements

- Engine: **PostgreSQL 15 or 16**
- The RDS security group must allow inbound TCP on port **5432** from the EB EC2 instance security group
- An initial database must exist in the RDS instance (create with `CREATE DATABASE dbname;` if not set during RDS setup)
- The server runs schema migrations automatically on startup — no manual DDL required

### Deploying a new version

```bash
# From the repository root — exclude local-only files
zip -r tripplanner.zip . \
  -x "frontend/node_modules/*" \
  -x "backend/.env" \
  -x "**/*.db" \
  -x ".git/*" \
  -x "docker-compose.yml"
```

Upload `tripplanner.zip` via **EB Console → Application versions → Upload and deploy**.
