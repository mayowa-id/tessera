# Tessera

Every shift in its place. A learning project for building a Go + Angular + PostgreSQL roster application in a monorepo.

Current stage: draft rosters and safe assignments. Managers can build weekly rosters, add/edit/remove shift slots, and assign qualified employees with scheduling checks. Publishing and employee schedule access come next. Follow [the implementation plan](docs/implementation-plan.md) for the milestones.

Public browsing and team onboarding are available: explore the illustrative roster without signing in, create a team, verify the owner's email, then invite teammates. Email delivery uses Resend. See [production onboarding](docs/production-onboarding.md) for the deployed URLs, backend secrets, migration 5, and release checks.

The two primary application folders are `frontend/` (Angular) and `backend/` (Go). Each is independently deployable as its own Vercel project. Root-level documentation, contracts, and development infrastructure are shared.

## Development

Requirements: Go 1.25.1+, Node 22.13.1 (Angular 21 compatible), npm 11.6.2, and PostgreSQL 17+ through Docker Compose v2 or a native installation. Commands below use PowerShell from the repository root. The API reads process environment variables; it does not automatically load `.env`.

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

If you already use native or managed development PostgreSQL, skip `docker compose` and set `DATABASE_URL` to that development database before running migrations. The Windows test helper below uses an entirely separate disposable cluster.

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

## Team and location management

After login, managers open **Team & locations** from the workspace. Use the tabs to manage employees, locations, and job roles. Each list supports literal name search, pagination, and showing inactive records. Edit a record and clear **Active** to deactivate it; no hard-delete endpoint is exposed.

Employee saves update the name, active status, and complete qualification set atomically. New qualifications must reference active roles in the same organization. An existing inactive qualification may be retained or removed, but cannot be newly assigned or re-added after removal. Location and role names are case-insensitively unique within an organization. New staff records do not automatically create login accounts.

Staff endpoints share manager authorization, session validation, and mutation CSRF checks. Every collection, direct lookup, and update includes the authorized organization ID. The API uses bounded `limit`/`offset` pagination with `has_more`; defaults are 25 records and a maximum of 100.

## Checks and generation

## Draft rosters

Open **Rosters** from a manager workspace. Choose a Monday and location, create a draft, then add shift slots with a required qualification and local start/end time. Each slot accepts one employee; the assignment selector offers active qualified staff. Edits are shown only after server confirmation, and rejected edits retain the previous assignment.

- One roster per organization/location/week. The roster snapshots the organization's IANA timezone.
- Shift start must fall inside the local roster week. Shifts last more than zero and at most 24 elapsed hours; Sunday overnight shifts may end on Monday.
- Start and end controls use the roster timezone, regardless of the browser timezone. Skipped daylight-saving times are rejected; repeated times offer first/second occurrence choices.
- Draft assignments reserve employee time across all locations. Overlap, inactive employees, missing qualifications, and less than 11 hours of rest are hard conflicts.
- More than 40 actual elapsed hours in a local Monday-based week is a warning by default. Overnight hours are clipped into each affected week; daylight-saving changes use actual elapsed time.
- These starter policies are configured in `backend/internal/scheduling/rules.go`. Weekly hard-limit mode is exercised by concurrency tests; it is not exposed as a settings screen yet.

PostgreSQL exclusion constraints reject overlaps independently of application checks. Half-open `[start,end)` intervals allow database adjacency, but the separate rest rule may still reject adjacent assignments. Database triggers derive assignment intervals from shifts and synchronize time edits; clients cannot submit an independent reservation interval.

Scheduling mutations acquire an organization-wide transaction advisory lock under explicit READ COMMITTED isolation, then lock affected employees in sorted order, the roster, and the shift. This deliberately serializes scheduling writes within each organization in v1. Employee locks coordinate with staff qualification/status edits; deadlocks/serialization failures receive at most three attempts. Future publish operations must use the same protocol.

Changing staff qualifications or status can make an existing draft assignment unavailable. It stays visible for resolution, and subsequent assigned-shift edits revalidate it. Publishing and its complete validation pass are the next milestone.

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

On Windows, Docker is optional if native PostgreSQL binaries are installed. From the repository root, this helper creates a disposable loopback database, runs the complete backend suite, and stops/removes only its own generated test cluster:

```powershell
.\scripts\test-backend-postgres.ps1 -PostgresBin 'C:\Program Files\PostgreSQL\18\bin'
```

It does not use the Windows PostgreSQL service or its data directory. Use `-Port` to choose another test port if 15432 is occupied.

Add `-CheckMigrations` to validate and apply all migrations, roll back the latest migration, re-apply it, and then run the suite in the same disposable instance.

From `frontend/`: `npm test -- --watch=false` and `npm run build`. The 23 frontend tests cover readiness, identity, staff administration, local/DST time conversion, and draft creation/assignment/conflict/unassignment/removal. Backend tests include pure scheduling rules and real database concurrency, interval triggers, transactions, and tenant boundaries. CI applies migrations against PostgreSQL and checks readiness plus generated SQL/client freshness.

## Architecture

```mermaid
flowchart LR
  Browser --> Angular
  Angular -->|/api| Go[Go API / chi]
  Go -->|pgx / generated sqlc queries| PostgreSQL
```

Feature modules will be added as we build them. Database constraints protect stored invariants; Go application services coordinate transactions and business rules. The OpenAPI contract describes health, identity, staff, and draft scheduling endpoints. Database queries are generated with sqlc; staff and roster screens use generated Angular HTTP services/models. Client generation uses LF line endings for reproducibility across Windows and Linux.

Run `npm run generate:api` from `frontend/` after changing `contracts/openapi.yaml`. Generated clients are committed so each Vercel project can build independently without reading sibling folders. CI regenerates and checks both SQL and Angular client output for drift. Edit the contract or SQL source rather than generated files.

## Verification

Passed locally: the full Go suite including real PostgreSQL identity/staff/roster integration, vet and API build; Angular production build and all 23 frontend tests; SQL and Angular client generation; and Goose migration checks. Native PostgreSQL 18 is used for isolated local verification, while CI targets PostgreSQL 17.

Database tests cover identity/staff behavior, concurrent overlapping assignment (one winner), concurrent non-overlapping assignments against a hard weekly cap, cross-week rest, warnings, conflicting shift-edit rollback, trigger synchronization, forged interval rejection, tenant boundaries, and immutable non-draft state. Temporary test servers are stopped after verification. Browser visual inspection has not been performed.

## Hosting

Create two Vercel projects from this repository: **Root Directory `frontend`** for Angular and **Root Directory `backend`** for Go. Each folder includes its own `vercel.json`. PostgreSQL is hosted separately through a managed provider.

Before deploying the frontend, configure a Vercel rewrite for `/api/:path*` to your real API origin, before the SPA fallback. The development proxy is not used in production. See [the deployment guide](deploy/README.md).

The backend uses Vercel's Go preset with the existing `cmd/api/main.go` entry point. It listens on Vercel's `PORT` when supplied, otherwise the local `HTTP_ADDR`. Configure a production `DATABASE_URL` and the frontend's HTTPS `APP_ORIGIN` on the backend project; leave secure cookies enabled. Run migrations separately. Future background jobs will use scheduled invocations or a separate worker. No deployment has been performed.
