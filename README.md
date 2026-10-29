# Tessera

Every shift in its place. A learning project for building a Go + Angular + PostgreSQL roster application in a monorepo.

Current stage: foundation. Scheduling and authentication are not implemented yet. Follow [the implementation plan](docs/implementation-plan.md) for the milestones.

The two primary application folders are `frontend/` (Angular) and `backend/` (Go). Each is independently deployable as its own Vercel project. Root-level documentation, contracts, and development infrastructure are shared.

## Development

Requirements: Go 1.25.1+, Node 22.13.1 (Angular 21 compatible), npm 11.6.2, and Docker Compose v2. Commands below use PowerShell from the repository root. The API reads process environment variables; it does not automatically load `.env`.

```powershell
Copy-Item .env.example .env
docker compose up -d --wait
Set-Location backend
$env:DATABASE_URL='postgres://tessera:tessera_local@localhost:5432/tessera?sslmode=disable'
go run github.com/pressly/goose/v3/cmd/goose@v3.24.3 -dir db/migrations postgres $env:DATABASE_URL up
go run ./cmd/api
```

In another terminal from the root:

```powershell
Set-Location frontend
npx --yes npm@11.6.2 ci
npm start
```

Open http://localhost:4200. Angular proxies `/api` to http://localhost:8080 during development. `/api/healthz` confirms the process is alive; `/api/readyz` returns 200 when PostgreSQL is reachable and 503 otherwise. Readiness does not yet assert schema version.

Port 8080 was occupied on the development machine during setup. If it is still in use, set `$env:HTTP_ADDR=':8081'` before starting Go and update the target port in `frontend/proxy.conf.json` to match. Do not stop another service just to free the default port.

If you change database credentials in `.env`, update `DATABASE_URL` too. Compose initialization credentials apply only when its database volume is first created.

## Checks and generation

From `backend/`:

```powershell
go test ./...
go vet ./...
go build -o bin/tessera-api ./cmd/api
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate
```

From `frontend/`: `npm test -- --watch=false` and `npm run build`. Angular tests check successful readiness and a database failure followed by a successful retry. The foundation's Go tests cover independent liveness, successful readiness, failed readiness, and request IDs. CI also applies the migration against PostgreSQL and checks the real readiness endpoint.

## Architecture

```mermaid
flowchart LR
  Browser --> Angular
  Angular -->|/api| Go[Go API / chi]
  Go -->|pgx / generated sqlc queries| PostgreSQL
```

Feature modules will be added as we build them. Database constraints protect stored invariants; Go application services coordinate transactions and business rules. OpenAPI will define the application API before the first business features.

## Foundation verification

Verified locally: Go tests, vet, API build, Angular production build, two Angular connection tests, migration-file validation, and requests through Angular's development proxy to the compiled Go API. Temporary smoke-test servers were stopped after verification.

Docker was unavailable in this environment. Applying the migration and observing successful readiness against a real PostgreSQL instance remain unverified locally; CI is configured to check those steps. Browser visual inspection has not been performed. Milestone 0 is not fully validated until the real database check succeeds.

## Hosting

Create two Vercel projects from this repository: **Root Directory `frontend`** for Angular and **Root Directory `backend`** for Go. Each folder includes its own `vercel.json`. PostgreSQL is hosted separately through a managed provider.

Before deploying the frontend, configure a Vercel rewrite for `/api/:path*` to your real API origin, before the SPA fallback. The development proxy is not used in production. See [the deployment guide](deploy/README.md).

The backend uses Vercel's Go preset with the existing `cmd/api/main.go` entry point. It listens on Vercel's `PORT` when supplied, otherwise the local `HTTP_ADDR`. Configure a production `DATABASE_URL` on the backend project and run migrations separately. Future background jobs will use scheduled invocations or a separate worker. No deployment has been performed.
