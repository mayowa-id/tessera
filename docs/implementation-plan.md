# Tessera implementation plan

Build a shift roster application to learn the complete process of delivering a Go API and Angular frontend in one repository. Work through runnable milestones; the original six-to-eight-week estimate is a rough guide, not a deadline.

Current progress: foundation and identity code are implemented. Go unit tests/vet/build, Angular production build and eight frontend tests, sqlc generation, and migration-file validation have passed locally. The real PostgreSQL identity/tenant suite is configured for CI and can run with `TEST_DATABASE_URL`; it has not run locally because Docker/PostgreSQL is unavailable. Complete that database verification before treating milestones 0 and 1 as fully accepted. Staff administration is the next feature milestone.

## 1. Scope and decisions

The first release lets a manager manage employees, create a weekly roster, assign shifts safely, and publish it. Employees sign in and see their published shifts.

- Backend: Go, chi, PostgreSQL, pgx, sqlc, goose migrations.
- Frontend: Angular, router, reactive forms, signals, HttpClient. Add CDK drag-and-drop after ordinary assignment works.
- API: REST with an OpenAPI contract and generated Angular client.
- Architecture: one modular API, one frontend, one database. No microservices or monorepo orchestration framework initially.
- Logging: start with Go's standard structured logger.
- Local development: Docker Compose for PostgreSQL; run Go and Angular locally for fast feedback. Deploy frontend and backend as separate Vercel projects, each rooted in its own application folder.
- Persistence: write migrations and SQL, then generate Go query methods with sqlc. MikroORM targets TypeScript/Node.js and does not fit a Go backend.
- Pin tool versions during setup and commit dependency lockfiles. Record version requirements and Windows-compatible commands in the README.

Defer swaps, email jobs, recurring templates, PDF exports, calendar feeds, auto-scheduling, and advanced metrics until the core release works.

## 2. Repository shape

```text
backend/
  cmd/api/
  internal/
    auth/
    organization/
    employee/
    roster/
    scheduling/
    platform/       # configuration, database, HTTP plumbing
    db/             # generated sqlc package
  db/migrations/
  db/queries/
  go.mod
frontend/           # Angular application and its package lockfile
contracts/openapi.yaml
deploy/             # deployment instructions for both Vercel projects
docs/
compose.yaml
.env.example
README.md
.github/workflows/
```

Organize Go code around features. Within each feature separate HTTP parsing, application operations, and persistence as the feature grows. Business rules must be callable without HTTP or a live database. Introduce small interfaces at real boundaries; avoid generic repository frameworks.

## 3. Domain decisions before schema work

- An organization is the tenant and has an IANA timezone, initially Africa/Lagos by default. Users are login identities; memberships connect them to organizations with admin, manager, or employee permissions.
- Employees are organization-owned staff records with optional linked memberships. Staff can exist before receiving a login.
- Job roles are qualifications such as nurse or cashier. Name them `job_roles` to distinguish them from authorization roles.
- A roster belongs to one organization and one location, covers one local Monday-to-Monday week, and owns its shifts. Reject duplicate rosters for that location and week initially.
- A shift is one staffing slot requiring one job role. Multiple people at the same time are represented by multiple slots in v1. A slot has at most one assignment.
- Store actual start/end instants with timezone-aware database types; retain the organization's timezone separately. Use half-open intervals `[start, end)` so adjacent shifts do not overlap. Reject zero-length or reversed shifts.
- Allow overnight shifts. Split hours at local week boundaries when calculating weekly totals; load neighboring shifts when checking minimum rest.
- Draft assignments reserve employee time too, across all locations. Employees cannot read draft rosters.
- States are draft, published, and archived. Published and archived rosters are immutable in v1; validate the complete roster before publishing. Future amendments need an explicit revision workflow.
- Policy values such as minimum rest and maximum weekly hours are product configuration, not universal legal requirements. Begin with documented defaults; treat overlap and missing qualifications as hard failures, weekly hours as a warning, and minimum rest as a hard failure.
- Archiving is allowed only after the roster ends. Retain assignments for history and reporting.

## 4. Build milestones

### Milestone 0: A running skeleton

1. Initialize the Go module and Angular application.
2. Add PostgreSQL Compose configuration, environment examples, and migration tooling.
3. Add API configuration validation, request IDs, structured logs, graceful shutdown, liveness and database readiness endpoints.
4. Make Angular display API readiness and a useful failure message.
5. Add CI for Go formatting/vet/tests/build and frontend checks/build.

Done when a fresh checkout can start the database, apply a migration, run both applications, and display the API result using documented commands. Record prerequisites including Docker and supported tool versions. No business features yet.

### Milestone 1: Login and organization boundaries

1. Create organizations, users, memberships, employees, locations, job roles, and employee qualifications.
2. Seed a manager, an employee login, and a small demo organization through a repeatable development command.
3. Implement login, logout, and current-user endpoints. Prefer opaque server-side sessions in Secure/HttpOnly cookies for this browser application; document local cookie settings, CSRF protection, password hashing, expiry, revocation, and login rate limiting.
4. Resolve organization membership server-side. Never trust a submitted organization ID without membership validation.
5. Implement permission checks, Angular login, protected routes, and a role-aware application shell. Backend authorization remains authoritative.

Done when both users can log in, manager endpoints reject employee access, logout invalidates the session, and tests prove organization A cannot read or mutate organization B's records. Include lists, direct IDs, and relationship IDs in isolation checks.

### Milestone 2: Staff administration, end to end

1. Add employee, location, and job-role APIs with validation and pagination.
2. Add corresponding Angular forms, lists, loading/error/empty states, and qualification editing.
3. Establish the OpenAPI contract and client generation process before expanding the API further.
4. Use deactivation for employees and locations referenced by roster history.

Done when a manager can create staff, assign qualifications, and reload the page without losing changes. Database foreign keys reject cross-organization relationships.

### Milestone 3: Draft rosters and safe assignments

1. Add rosters, shifts, and assignments. Start with a list or basic weekly table and form-based assignment.
2. Implement create/read/update/delete for draft shifts, assign/unassign, and roster retrieval. Scope every lookup and write to the authorized organization.
3. Implement pure scheduling checks for qualifications, minimum rest, and weekly hours. Keep rule inputs explicit and return stable violation codes, severity, affected records, and messages.
4. Implement database overlap protection and transactional assignment immediately, before adding more workflows.
5. Make shift edits revalidate existing assignments. All operations affecting assignment validity must use the same locking protocol.

Done when a manager can build a weekly draft, adjacent shifts are allowed, invalid assignments return understandable errors, and concurrent requests cannot bypass scheduling rules.

### Milestone 4: Publish and employee access (core release)

1. Add publish and archive operations with allowed state transitions and complete publish validation.
2. Require every slot to be assigned before publishing in v1. Warnings are displayed but do not prevent publication.
3. Add employee upcoming-shifts view restricted to their published rosters.
4. Improve the weekly roster layout, then add optional drag-and-drop using the same assignment endpoint. Start with confirmed server updates; add optimism only if it improves usability.
5. Record actor, organization, operation, timestamp, and relevant changes in an audit table within each mutation transaction.

Done when a manager can create and publish a roster and an employee sees only their published shifts. Add a browser smoke test covering that complete journey. Verify keyboard-accessible assignment works without dragging.

### Milestone 5: Availability and leave

1. Begin with dated availability windows, then add recurring local-time availability only if needed.
2. Add leave requests and manager approval/rejection with explicit transitions.
3. Extend assignment validation to approved leave and availability. Absence of declared availability means unrestricted availability initially; approved leave blocks assignment.
4. Reject leave approval if it conflicts with an existing assignment and identify shifts the manager must resolve first.

Done when approvals and assignments cannot race into an invalid schedule. Add regression cases for midnight, local dates, week boundaries, and daylight-saving transitions in a timezone that observes them.

### Milestone 6: Delivery and optional workflows

Deploy two Vercel projects from the same repository: `frontend` with the Angular preset and `backend` with the Go preset. The Go server must listen on Vercel's `PORT`. Connect managed PostgreSQL, run migrations as a release step, and proxy frontend `/api` requests to the backend deployment to preserve a single browser origin. Verify backup/restore, readiness, and the end-to-end smoke path. Restrict development seed credentials to development or an intentionally isolated demo.

Then choose one extension at a time:

- Shift swaps with peer acceptance, manager approval, cancellation, expiry, and validation at approval. Lock all affected employees in stable ID order.
- Notifications using a transactional outbox/job table, retries, deduplication, and a development log notifier. For Vercel, process bounded batches through a protected scheduled invocation or a separate worker service; do not rely on a perpetual background goroutine in the API deployment.
- CSV export, calendar subscriptions with revocable private tokens, PDF export.
- Templates, metrics, and an auto-fill preview that reuses the scheduling rules. Auto-fill is a separate algorithm feature and may require substantial work.

Each extension must have an acceptance scenario and focused tests before starting the next.

## 5. Database integrity and concurrency

Organization-owned tables carry `org_id`; global users and the organizations table are exceptions. Use composite foreign keys such as `(org_id, employee_id)` so tenant isolation is enforced in relationships as well as queries. Add indexes matching tenant-scoped reads.

For v1 keep canonical shift times on `shifts` and copy the interval to `assignments` for the exclusion constraint. A database trigger derives assignment times from the referenced shift; a shift-update trigger synchronizes assigned intervals in the same transaction. Lock the shift before assignment to serialize against time edits. Generated columns cannot derive values from another table. Cover this synchronization with database integration tests.

Enable `btree_gist` and enforce non-overlap for `(org_id, employee_id, during)` using equality for IDs and overlap for the range. Add a unique assignment constraint per shift and checks for valid intervals. Map exclusion/uniqueness violations to structured conflicts without leaking SQL internals.

Use transactions and employee row locks for checks involving multiple rows, such as rest and weekly totals; an overlap constraint alone does not protect those rules. Under READ COMMITTED, acquire the locks before reading the current schedule, then validate and write. Every assignment, unassignment, shift edit, and later leave approval must honor this protocol. Lock affected employees in sorted order, then affected rosters, then shifts, with roster locks serializing mutations against publish. For edits whose affected employees are discovered first, re-read after locking and retry if the set changed. Bound retries for deadlocks or transient serialization failures.

Prove separately that concurrent overlapping assignments cannot both succeed and that non-overlapping assignments cannot together exceed a hard aggregate rule. Include simultaneous publication and assignment tests. For warning-only weekly hours, both assignments may succeed with warnings; use minimum rest or a hard-limit test policy for aggregate concurrency tests.

## 6. API and verification conventions

- Use one error envelope with a stable code, human message, optional field errors/violations, and request ID.
- Use 400 for malformed requests, 401/403 for authentication/authorization, 404 for missing or inaccessible tenant resources, 409 for state or scheduling conflicts, and 422 for invalid field/domain inputs.
- Require explicit date-range pagination or bounded ranges for schedule reads.
- Unit-test pure rules; integration-test real PostgreSQL constraints, migrations, transactions, authorization, and tenancy. Testcontainers is optional if local/CI PostgreSQL is already reliable.
- Include overnight shifts, adjacent intervals, schedule edits, rollback, publication visibility, and concurrent mutation cases.
- Generate clients reproducibly and check generated output is current in CI. Run required Go checks and Angular checks/build on every change; add a small browser suite once the core journey exists.
- Provide repeatable migrate, generate, seed, check, and run commands. Keep generated database code separate from hand-written business types.

## 7. How we work through the plan

For each milestone: explain the concept, implement one small vertical slice, run its checks, demonstrate it, and record the design tradeoff. Keep a short README with actual setup commands and an architecture diagram as the repository develops. Complete the core release before chasing the original plan's polish features.

Immediate next task: Milestone 0 only. Inspect installed Go, Node, Angular prerequisites, and Docker; choose compatible pinned versions; scaffold the applications and prove frontend-to-API connectivity.

## References

- Original supplied implementation plan: reviewed as source material; this document narrows its first release and makes unresolved choices explicit.
- MikroORM scope: https://mikro-orm.io/docs/guide
- sqlc workflow: https://docs.sqlc.dev/
- PostgreSQL exclusion constraints: https://www.postgresql.org/docs/18/sql-createtable.html
- btree_gist: https://www.postgresql.org/docs/17/btree-gist.html
