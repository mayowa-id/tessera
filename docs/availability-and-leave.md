# Availability and leave

Open **Availability & leave** from your workspace. Employees can see only their own requests and notes. Managers can switch to **Team requests** to review their team and record time for staff who do not have a login.

## Two ways to make time

- **Unavailable time** is a dated exception and blocks overlapping assignments immediately. Outside these exceptions, a person is available by default. Recurring availability is not part of this release.
- **Leave** starts as pending. A manager approves or rejects it; only approved leave blocks assignment. Check this screen for decisions—leave notification emails are not sent yet.

Enter start and end times in the organization's displayed timezone. Overnight periods are supported. Clock changes are handled explicitly: skipped local minutes are rejected and repeated minutes offer first/second occurrence choices. End times are exclusive, so a shift can start exactly when time off ends. Each period may span up to 366 elapsed days; lists show up to 93 elapsed days per date range.

Pending or approved records can be cancelled by their owner or a manager. Rejected and cancelled records are retained as history. Managers may review their own leave; this is an explicit v1 permission, not a substitute for a company's approval policy.

## Existing assignments

Unavailable time cannot be saved over an assigned shift. Leave can be requested over an assignment, but approval is blocked until the conflict is resolved. Managers receive affected shifts, locations, roster states, and weeks. Personal availability errors do not expose unpublished shift details; employees are asked to contact their manager. Draft shifts can be unassigned or reassigned through the linked roster.

Published schedules remain frozen in this release. An existing published assignment cannot be removed to approve conflicting leave yet; roster amendments need a separate workflow. Plan availability before publishing.

Assignments, assigned-shift edits, and publication all check approved leave and unavailable time. Approval and assignment use the same organization scheduling lock, followed by employee and request locks under READ COMMITTED isolation. If approval races with assignment, only one can succeed; the other receives a conflict. Pending leave and rejected/cancelled records do not block scheduling.

## Privacy and history

Reasons and decision notes are optional and limited to 500 characters. They are visible only to the owner and organization managers. Activity records the actor, employee, dates, kind, and state transition in the same transaction; reasons and decision notes are excluded from audit snapshots. The guest preview never reads real time-off records.

## Release

Apply **migration 7** before deploying this backend, then deploy the frontend. Both migration helpers apply pending migrations automatically:

```powershell
# Windows PowerShell, from the repository root
& "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File .\scripts\migrate-neon.ps1
```

```bash
# WSL/Linux, with Go installed
bash scripts/migrate-neon.sh
```

Use the private prompt for the direct Neon URL with TLS. Do not send it in chat or commit it. Keep the pooled connection URL in the deployed backend.

The browser smoke test covers employee request, manager approval, blocked assignment, employee cancellation, and restored assignment. PostgreSQL tests also cover ownership, CSRF, tenant boundaries, publication revalidation, audit privacy, and concurrent approval versus assignment.
