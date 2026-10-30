# Tessera

Every shift in its place. A learning project for building a Go + Angular + PostgreSQL roster application in a monorepo.

Current stage: the core roster workflow is implemented. Managers can manage staff, build drafts, publish validated rosters, archive ended weeks, and review activity. Employees can view their own published shifts and archived history. Availability and leave are next. Follow [the implementation plan](docs/implementation-plan.md) for the milestones.

Public browsing and team onboarding are available: explore the illustrative roster without signing in, create a team, verify the owner's email, then invite teammates. Email delivery uses Resend. See [production onboarding](docs/production-onboarding.md) for the deployed URLs, backend secrets, migration 6, and release checks.

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

All browser mutations must have the exact configured `APP_ORIGIN` and `X-Tessera-Request: 1`; authenticated mutations also require the session-bound `X-CSRF-Token` returned by login or `/api/auth/me`. Angular keeps identity and CSRF state in memory, restores it from the session, and adds the required headers. No CORS allowlist is opened: production uses the frontend's same-origin API proxy. See [the authentication decisions](docs/authentication.md).

The manager summary endpoint checks current membership and role on every request. Nonmembers receive 404, employees receive 403 for manager-only access, and unauthenticated requests receive 401. Organization-scoped foreign keys prevent cross-tenant employee qualifications and membership links.

## Team and location management

After login, managers open **Team & locations** from the workspace. Use the tabs to manage employees, locations, and job roles. Each list supports literal name search, pagination, and showing inactive records. Edit a record and clear **Active** to deactivate it; no hard-delete endpoint is exposed.

Employee saves update the name, active status, and complete qualification set atomically. New qualifications must reference active roles in the same organization. An existing inactive qualification may be retained or removed, but cannot be newly assigned or re-added after removal. Location and role names are case-insensitively unique within an organization. New staff records do not automatically create login accounts.

Staff endpoints share manager authorization, session validation, and mutation CSRF checks. Every collection, direct lookup, and update includes the authorized organization ID. The API uses bounded `limit`/`offset` pagination with `has_more`; defaults are 25 records and a maximum of 100.

## Draft rosters

Open **Rosters** from a manager workspace. Choose a Monday and location, create a draft, then add shift slots with a required qualification and local start/end time. Each slot accepts one employee; the assignment selector offers active qualified staff. Edits are shown only after server confirmation, and rejected edits retain the previous assignment.

- One roster per organization/location/week. The roster snapshots the organization's IANA timezone.
- Shift start must fall inside the local roster week. Shifts last more than zero and at most 24 elapsed hours; Sunday overnight shifts may end on Monday.
- Start and end controls use the roster timezone, regardless of the browser timezone. Skipped daylight-saving times are rejected; repeated times offer first/second occurrence choices.
- Draft assignments reserve employee time across all locations. Overlap, inactive employees, missing qualifications, and less than 11 hours of rest are hard conflicts.
- More than 40 actual elapsed hours in a local Monday-based week is a warning by default. Overnight hours are clipped into each affected week; daylight-saving changes use actual elapsed time.
- These starter policies are configured in `backend/internal/scheduling/rules.go`. Weekly hard-limit mode is exercised by concurrency tests; it is not exposed as a settings screen yet.

PostgreSQL exclusion constraints reject overlaps independently of application checks. Half-open `[start,end)` intervals allow database adjacency, but the separate rest rule may still reject adjacent assignments. Database triggers derive assignment intervals from shifts and synchronize time edits; clients cannot submit an independent reservation interval.

Scheduling mutations acquire an organization-wide transaction advisory lock under explicit READ COMMITTED isolation, then lock affected employees in sorted order, the roster, and the shift. This deliberately serializes scheduling writes within each organization in v1. Employee locks coordinate with staff qualification/status edits; deadlocks/serialization failures receive at most three attempts. Publishing and archiving use the same protocol.

Changing staff qualifications or status can make an existing draft assignment unavailable. It stays visible for resolution, and assigned-shift edits and publication revalidate it.

## Publish, employee schedules, and activity

Managers publish from **Rosters** after assigning every slot and saving or cancelling any open edit. Confirmation triggers a complete validation of the latest stored roster under the scheduling lock. Empty rosters, missing assignments, inactive records, and hard rule violations block publication. Weekly-hour warnings are returned without blocking. The response contains the validated snapshot, so concurrent additions cannot leave the screen showing an older draft.

Published rosters are frozen in v1. Staff edits cannot deactivate employees, roles, or locations, or remove qualifications needed by future published shifts. Archive is available only after both the local roster week and every shift (including overnight spillovers) have ended. There is no published-roster amendment flow yet.

**My shifts** is available to any signed-in organization member. It reads the user's linked employee record from the session, never from a submitted employee ID, and shows only their own published assignments and archived history. Date ranges are bounded to 62 elapsed days and support pagination. Drafts are excluded.

Managers use **Activity** to see staff and roster changes with actor names and timestamps. Before/after snapshots and the business mutation commit in one transaction. Audit-write failure rolls back the mutation; failed business operations produce no event. PostgreSQL rejects updates, deletes, and truncation of audit events. The explicit local seed is a maintenance operation and does not impersonate an authenticated user in the activity log.

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

For the complete browser workflow on Windows with Microsoft Edge installed:

```powershell
.\scripts\test-backend-postgres.ps1 -BrowserSmoke
```

This also starts temporary API/Angular servers on available loopback ports, seeds random-password demo accounts, and runs the manager-to-employee publication smoke test in headless Edge. It shuts down and removes its test services afterward. The last screenshot is retained at `.cache/publication-smoke.png`, and server diagnostics at `.cache/publication-browser/`. CI runs the same browser test with Chromium. `frontend/e2e/publication.smoke.mjs` accepts only a local `E2E_BASE_URL` and takes the seeded password through `E2E_PASSWORD`.

From `frontend/`: `npm test -- --watch=false` and `npm run build`. The 30 frontend tests cover identity/staff behavior, DST conversion, roster operations, confirmed assignment rendering, publication/archival, personal schedules, and activity. Backend tests include pure rules, real concurrency, interval triggers, publication privacy, and transaction/audit invariants. CI checks migrations, generated clients/SQL, unit/integration tests, and the browser workflow.

## Architecture

```mermaid
flowchart LR
  Browser --> Angular
  Angular -->|/api| Go[Go API / chi]
  Go -->|pgx / generated sqlc queries| PostgreSQL
```

Feature modules will be added as we build them. Database constraints protect stored invariants; Go application services coordinate transactions and business rules. OpenAPI describes the implemented health, identity, staff, roster lifecycle, personal-schedule, and activity endpoints. Database queries use sqlc; feature screens use generated Angular HTTP services/models. Client generation uses LF line endings for reproducibility across Windows and Linux.

Run `npm run generate:api` from `frontend/` after changing `contracts/openapi.yaml`. Generated clients are committed so each Vercel project can build independently without reading sibling folders. CI regenerates and checks both SQL and Angular client output for drift. Edit the contract or SQL source rather than generated files.

## Verification

Passed locally: the full Go suite with real PostgreSQL identity/staff/roster/publication/audit integration; vet and API build; Angular production build and all 30 frontend tests; SQL/Angular client generation; migration apply/rollback/re-apply; and the headless Edge browser smoke. Native PostgreSQL 18 is used locally; CI targets PostgreSQL 17 and Chromium.

Database tests cover assignment races, aggregate rules, interval synchronization, forged interval rejection, publish-time revalidation, publication versus assignment, employee identity scoping, archived history, future published-shift protections, append-only audit enforcement, and audit-failure rollback. The browser smoke exercises manager create/assign/publish/activity/logout, employee schedule/session restoration, and manager-route denial. The rendered employee schedule has also been visually inspected. Temporary servers are stopped after verification.

## Hosting

Create two Vercel projects from this repository: **Root Directory `frontend`** for Angular and **Root Directory `backend`** for Go. Each folder includes its own `vercel.json`. PostgreSQL is hosted separately through a managed provider.

Before deploying the frontend, configure a Vercel rewrite for `/api/:path*` to your real API origin, before the SPA fallback. The development proxy is not used in production. See [the deployment guide](deploy/README.md).

The backend uses Vercel's Go preset with the existing `cmd/api/main.go` entry point. It listens on Vercel's `PORT` when supplied, otherwise the local `HTTP_ADDR`. Configure a production `DATABASE_URL` and the frontend's HTTPS `APP_ORIGIN` on the backend project; leave secure cookies enabled. Run migrations separately. Future background jobs will use scheduled invocations or a separate worker. No deployment has been performed.
