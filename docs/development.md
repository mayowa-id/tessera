# Local development

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

Open http://localhost:4200 to explore the sample workspace. Health checks are internal endpoints used by development tools and monitoring; they do not have a customer-facing page. Angular proxies `/api` to http://localhost:8080 during development. `/api/healthz` confirms the process is alive; `/api/readyz` returns 200 when PostgreSQL is reachable and 503 otherwise. Readiness does not yet assert schema version. Use the exact configured frontend origin; `localhost` and `127.0.0.1` are different origins.

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

Sign in as `manager@tessera.local` or `employee@tessera.local` with that password. Seeding updates these demo accounts, revokes their sessions, and resets their login throttles. It is explicitly disabled on Vercel. For self-service signup and invitations, configure Resend as described in [production onboarding](production-onboarding.md).

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

From `frontend/`: `npm test -- --watch=false` and `npm run build`. Frontend tests cover identity/staff behavior, DST conversion, roster operations, confirmed assignment rendering, publication/archival, personal schedules, and activity. Backend tests include pure rules, real concurrency, interval triggers, publication privacy, and transaction/audit invariants. CI checks migrations, generated clients/SQL, unit/integration tests, and the browser workflow.
