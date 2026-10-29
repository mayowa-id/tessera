# Tessera

Every shift in its place. A learning project for building a Go + Angular + PostgreSQL roster application in a monorepo.

Current stage: login and organization access. Manager and employee sessions, a role-aware workspace, and the initial organization schema are implemented. Scheduling and staff editing come next. Follow [the implementation plan](docs/implementation-plan.md) for the milestones.

The two primary application folders are `frontend/` (Angular) and `backend/` (Go). Each is independently deployable as its own Vercel project. Root-level documentation, contracts, and development infrastructure are shared.

## Development

Requirements: Go 1.25.1+, Node 22.13.1 (Angular 21 compatible), npm 11.6.2, and Docker Compose v2. Commands below use PowerShell from the repository root. The API reads process environment variables; it does not automatically load `.env`.

```powershell
Copy-Item .env.example .env
docker compose up -d --wait
Set-Location backend
$env:DATABASE_URL='postgres://tessera:tessera_local@localhost:5432/tessera?sslmode=disable'
$env:APP_ORIGIN='http://localhost:4200'
$env:SESSION_COOKIE_SECURE='false'
go run github.com/pressly/goose/v3/cmd/goose@v3.24.3 -dir db/migrations postgres $env:DATABASE_URL up
go run ./cmd/api
```

In another terminal from the root:

```powershell
Set-Location frontend
npx --yes npm@11.6.2 ci
npm start
```

Open http://localhost:4200 to sign in; `/system` shows connection diagnostics. Angular proxies `/api` to http://localhost:8080 during development. `/api/healthz` confirms the process is alive; `/api/readyz` returns 200 when PostgreSQL is reachable and 503 otherwise. Readiness does not yet assert schema version. Use the exact configured frontend origin; `localhost` and `127.0.0.1` are different origins.

Port 8080 was occupied on the development machine during setup. If it is still in use, set `$env:HTTP_ADDR=':8081'` before starting Go and update the target port in `frontend/proxy.conf.json` to match. Do not stop another service just to free the default port.

If you change database credentials in `.env`, update `DATABASE_URL` too. Compose initialization credentials apply only when its database volume is first created.

## Development accounts

After applying migrations, run the repeatable seed from `backend/` against a development database. Supply a password of 12 to 72 bytes. The password is not stored in the repository or printed by the command:

```powershell
$env:TESSERA_ALLOW_DEMO_SEED='true'
$demoSecret=Read-Host 'Demo password' -AsSecureString
$env:DEMO_PASSWORD=[System.Net.NetworkCredential]::new('', $demoSecret).Password
go run ./cmd/seed
Remove-Item Env:DEMO_PASSWORD
```

Sign in as `manager@tessera.local` or `employee@tessera.local` with that password. Seeding updates these demo accounts, revokes their sessions, and resets their login throttles. It is explicitly disabled on Vercel. There is no public registration or invitation flow yet.

## Authentication and organization boundaries

Sessions last 12 hours and are stored in PostgreSQL using SHA-256 hashes of random 256-bit cookie tokens. Cookies are host-only, HttpOnly, SameSite=Lax, scoped to `/api`, and Secure by default. Set `SESSION_COOKIE_SECURE=false` only for local development. Passwords use bcrypt cost 12; the seed validates its 72-byte limit.

All browser mutations must have the exact configured `APP_ORIGIN` and `X-Tessera-Request: 1`; logout also requires the session-bound `X-CSRF-Token` returned by login or `/api/auth/me`. Angular keeps identity and CSRF state in memory, restores it from the session, and adds the required headers. No CORS allowlist is opened: production uses the frontend's same-origin API proxy. See [the authentication decisions](docs/authentication.md).

The manager summary endpoint checks current membership and role on every request. Nonmembers receive 404, employees receive 403 for manager-only access, and unauthenticated requests receive 401. Organization-scoped foreign keys prevent cross-tenant employee qualifications and membership links.

## Checks and generation

From `backend/`:

```powershell
go test ./...
go vet ./...
go build -o bin/tessera-api ./cmd/api
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate
```

To run the real PostgreSQL integration test against a development database:

```powershell
$env:TEST_DATABASE_URL=$env:DATABASE_URL
go test ./internal/auth -run TestPostgresIdentity -v
```

The test creates and removes a unique schema and requires schema/extension privileges. Use a development or dedicated test database. It applies migrations, seeds twice, checks login/expiry/throttling, and verifies tenant restrictions in both HTTP and database constraints. Without `TEST_DATABASE_URL` it is explicitly skipped. CI supplies it automatically.

From `frontend/`: `npm test -- --watch=false` and `npm run build`. Frontend tests cover readiness, login validation, expired sessions, logout CSRF headers, manager counts, and employee restrictions. Backend tests cover session rotation/revocation/expiry, origin and CSRF checks, throttling, role enforcement, and tenant isolation. CI also applies migrations against PostgreSQL and checks readiness and generated SQL freshness.

## Architecture

```mermaid
flowchart LR
  Browser --> Angular
  Angular -->|/api| Go[Go API / chi]
  Go -->|pgx / generated sqlc queries| PostgreSQL
```

Feature modules will be added as we build them. Database constraints protect stored invariants; Go application services coordinate transactions and business rules. The OpenAPI contract describes the implemented health and authentication endpoints. Database queries are generated with sqlc; the frontend currently uses typed HttpClient services, with client generation planned before staff administration.

## Foundation verification

The foundation was verified with Go tests/vet/build, Angular build and connection tests, migration-file validation, and requests through Angular's development proxy. The identity milestone adds the tests described above. Temporary smoke-test servers are stopped after verification.

Docker was unavailable in this environment. Applying the migration and observing successful readiness against a real PostgreSQL instance remain unverified locally; CI is configured to check those steps. Browser visual inspection has not been performed. Milestone 0 is not fully validated until the real database check succeeds.

## Hosting

Create two Vercel projects from this repository: **Root Directory `frontend`** for Angular and **Root Directory `backend`** for Go. Each folder includes its own `vercel.json`. PostgreSQL is hosted separately through a managed provider.

Before deploying the frontend, configure a Vercel rewrite for `/api/:path*` to your real API origin, before the SPA fallback. The development proxy is not used in production. See [the deployment guide](deploy/README.md).

The backend uses Vercel's Go preset with the existing `cmd/api/main.go` entry point. It listens on Vercel's `PORT` when supplied, otherwise the local `HTTP_ADDR`. Configure a production `DATABASE_URL` and the frontend's HTTPS `APP_ORIGIN` on the backend project; leave secure cookies enabled. Run migrations separately. Future background jobs will use scheduled invocations or a separate worker. No deployment has been performed.
