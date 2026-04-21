# TripPlanner

A collaborative trip-planning web app. Create trips, invite collaborators, search points of interest, and build shared itineraries — all in real time.

---

## Table of Contents

1. [Running the app locally](#running-the-app-locally)
2. [Running tests](#running-tests)
3. [Deploying to AWS (fork setup guide)](#deploying-to-aws-fork-setup-guide)

---

## Running the app locally

### Prerequisites

| Tool | Minimum version | Download |
|---|---|---|
| Node.js | 18 | https://nodejs.org |
| Go | 1.21 | https://go.dev/dl/ |
| Docker Desktop | 24 | https://www.docker.com/products/docker-desktop/ |

### 1. Clone the repository

```bash
git clone https://github.com/<your-org>/cs485.git
cd cs485
```

### 2. Start the database

```bash
docker compose up -d
```

This starts a PostgreSQL 16 container on `localhost:5432`. Wait a few seconds, then verify it is healthy:

```bash
docker compose ps
# postgres   running (healthy)
```

### 3. Configure the backend

Create `backend/.env`:

```env
DATABASE_URL=postgres://tripplanner:tripplanner@localhost:5432/tripplanner?sslmode=disable
JWT_SECRET=dev-secret-change-in-production
FRONTEND_URL=http://localhost:5173
SEED_DATA=true
```

Optionally add a Google Places API key to enable live POI search:

```env
API_KEY=your-google-places-api-key
```

Without `API_KEY`, the app falls back to the built-in Tokyo demo dataset.

### 4. Start the backend

```bash
cd backend
go run ./cmd/server/
```

You should see:

```
[db] demo seed complete
[main] server listening on :8080  (frontend: http://localhost:5173)
```

### 5. Start the frontend

Open a second terminal:

```bash
cd frontend
npm install
npm run dev
```

### 6. Open the app

Visit **http://localhost:5173** in your browser.

A demo account is pre-seeded:

| Email | Password |
|---|---|
| `sarah.chen@example.com` | `password123` |

Or register a new account from the sign-up page.

### Stopping the app

- Press `Ctrl+C` in both terminals to stop the frontend and backend.
- Stop the database: `docker compose stop` (preserves data) or `docker compose down` (stops container).

---

## Running tests

### Frontend unit tests

```bash
cd frontend
npm install
npm test
```

### Backend unit tests

No database or Docker required — the suite uses an in-memory SQLite database.

```bash
cd backend
go test -v ./...
```

### Frontend integration tests

Requires a running backend. Point `BASE_URL` at it before running:

```bash
cd frontend
BASE_URL=http://localhost:8080 npm run test:integration
```

All integration tests are silently skipped when `BASE_URL` is unset, so `npm test` always stays clean in CI without a live server.

---

## Deploying to AWS (fork setup guide)

The application ships as a single Docker image (multi-stage `Dockerfile`) that serves both the compiled React frontend and the Go API on port 8080. The recommended hosting target is **AWS Elastic Beanstalk** (Docker platform) backed by **AWS RDS** (PostgreSQL).

### Prerequisites

- An AWS account with permissions to create EB applications, RDS instances, EC2 security groups, and IAM roles.
- [AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2.html) installed and configured (`aws configure`).
- [EB CLI](https://docs.aws.amazon.com/elasticbeanstalk/latest/dg/eb-cli3-install.html) installed (`pip install awsebcli`).

---

### Step 1 — Create an RDS PostgreSQL database

1. Open the [RDS Console](https://console.aws.amazon.com/rds/) and click **Create database**.
2. Choose:
   - Engine: **PostgreSQL**
   - Version: **15** or **16**
   - Template: **Free tier** (for testing) or **Production**
3. Set credentials:
   - DB instance identifier: `tripplanner-db`
   - Master username: `tripplanner`
   - Master password: choose a strong password and save it
4. Under **Connectivity**, note the **VPC** and select **No** for public access (EB will access it privately).
5. Under **Additional configuration → Initial database name**, enter `tripplanner`.
6. Click **Create database** and wait for the status to become **Available**.
7. Copy the **Endpoint** from the database details page — you will need it in Step 3.

---

### Step 2 — Create the Elastic Beanstalk application

```bash
# From the repository root
eb init tripplanner --platform "Docker" --region us-east-1
```

When prompted, choose to set up SSH access if you want shell access to the EC2 instance.

---

### Step 3 — Create the environment

```bash
eb create tripplanner-prod \
  --instance-type t3.small \
  --elb-type application \
  --envvars \
    DATABASE_URL=postgres://tripplanner:<PASSWORD>@<RDS_ENDPOINT>:5432/tripplanner?sslmode=require,\
    JWT_SECRET=<LONG_RANDOM_SECRET>,\
    FRONTEND_URL=http://tripplanner-prod.<region>.elasticbeanstalk.com,\
    SEED_DATA=false,\
    API_KEY=<GOOGLE_PLACES_API_KEY>
```

Replace the placeholders:

| Placeholder | Value |
|---|---|
| `<PASSWORD>` | The RDS master password from Step 1 |
| `<RDS_ENDPOINT>` | The RDS endpoint hostname from Step 1 |
| `<LONG_RANDOM_SECRET>` | A random 40+ character string (e.g. `openssl rand -hex 32`) |
| `<region>` | The AWS region you chose (e.g. `us-east-1`) |
| `<GOOGLE_PLACES_API_KEY>` | Optional — omit the variable entirely if you don't have one |

> **`FRONTEND_URL`**: Once the environment is created you will get a permanent EB URL. Update this variable to match that URL exactly, or to your custom domain if you configure one.

---

### Step 4 — Allow the EB instances to reach RDS

The EC2 instances created by EB need network access to the RDS instance on port 5432.

1. In the [EC2 Console → Security Groups](https://console.aws.amazon.com/ec2/#SecurityGroups), find the security group attached to your EB environment (named something like `awseb-…`).
2. Open the **RDS security group** (find it in the RDS Console under your database → **Connectivity & security → VPC security groups**).
3. Add an **inbound rule** to the RDS security group:
   - Type: **PostgreSQL**
   - Port: **5432**
   - Source: the EB EC2 security group ID (starts with `sg-`)

---

### Step 5 — Deploy

```bash
# From the repository root
eb deploy
```

The EB CLI builds and uploads a zip of the repository, EB runs the multi-stage `Dockerfile`, and the environment updates in place. The first deploy takes 5–10 minutes; subsequent deploys are faster.

Monitor progress:

```bash
eb logs --all
```

When the deploy succeeds, get the public URL:

```bash
eb open
```

---

### Step 6 — Update `FRONTEND_URL`

After the first deploy you know the permanent EB URL (e.g. `http://tripplanner-prod.us-east-1.elasticbeanstalk.com`). Update the environment variable so CORS and invitation links work correctly:

```bash
eb setenv FRONTEND_URL=http://tripplanner-prod.us-east-1.elasticbeanstalk.com
```

EB will perform a rolling update to pick up the change.

---

### Step 7 — (Optional) Set the GitHub Actions secret for integration tests

To run the integration test suite against the deployed environment in CI:

1. Go to your GitHub repository → **Settings → Secrets and variables → Actions**.
2. Click **New repository secret**.
3. Name: `BACKEND_BASE_URL`
4. Value: your EB environment URL (e.g. `http://tripplanner-prod.us-east-1.elasticbeanstalk.com`)

The workflow in `.github/workflows/run-integration-tests.yml` will automatically run on every push.

---

### Environment variables reference

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | Full PostgreSQL connection string. Use `sslmode=require` for RDS. |
| `JWT_SECRET` | Yes | Secret for signing JWT tokens. Use a long random value — rotating it logs out all users. |
| `FRONTEND_URL` | Yes | Public URL of the app. Used for CORS and invitation email links. |
| `SEED_DATA` | No | Set `true` to seed demo accounts and a Tokyo trip on startup. Default `true` in the Docker image; set `false` for production. |
| `API_KEY` | No | Google Places API key. Enables live POI search worldwide. Without it, only the seeded Tokyo POIs are available. |
| `PORT` | No | Port the server listens on. Defaults to `8080`. Do not change on Elastic Beanstalk. |

---

### Deploying a new version

After making code changes:

```bash
eb deploy
```

Or upload manually via the EB Console: build a zip (excluding `frontend/node_modules`, `backend/.env`, `.git`) and upload it under **Application versions → Upload and deploy**.

To run frontend tests:
<ul>
  <li>Move to the frontend directory (<code>cd frontend</code>)</li>
  <li>Install Jest framework (<code>npm install -D jest @types/jest jest-environment-jsdom @testing-library/react @testing-library/jest-dom ts-jest</code>)</li>
  <li>Run <code>npm test</code> to run both test files</li>
</ul>

To run backend tests:
<ul>
  <li>Move to the backend directory (<code>cd backend</code>)</li>
  <li>Run <code>go test -v ./...</code> to run all backend tests</li>
</ul>
