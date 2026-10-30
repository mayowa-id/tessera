-- name: SerializeScheduling :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(org_id)::text,0));

-- name: RosterLocation :one
SELECT l.id::text AS id,o.timezone FROM locations l JOIN organizations o ON o.id=l.org_id
WHERE l.org_id=sqlc.arg(org_id)::text::uuid AND l.id=sqlc.arg(id)::text::uuid AND l.active FOR SHARE OF l;

-- name: CreateRoster :one
INSERT INTO rosters (org_id,location_id,week_start,timezone)
VALUES (sqlc.arg(org_id)::text::uuid,sqlc.arg(location_id)::text::uuid,$1,$2) RETURNING id::text;

-- name: PageRosters :many
SELECT r.id::text AS id,r.location_id::text AS location_id,l.name AS location_name,r.week_start,r.timezone,r.state
FROM rosters r JOIN locations l ON l.org_id=r.org_id AND l.id=r.location_id
WHERE r.org_id=sqlc.arg(org_id)::text::uuid AND r.week_start=$1
ORDER BY lower(l.name),r.id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: GetRoster :one
SELECT r.id::text AS id,r.location_id::text AS location_id,l.name AS location_name,r.week_start,r.timezone,r.state
FROM rosters r JOIN locations l ON l.org_id=r.org_id AND l.id=r.location_id
WHERE r.org_id=sqlc.arg(org_id)::text::uuid AND r.id=sqlc.arg(id)::text::uuid;

-- name: LockRoster :one
SELECT state FROM rosters WHERE org_id=sqlc.arg(org_id)::text::uuid AND id=sqlc.arg(id)::text::uuid FOR UPDATE;

-- name: RosterShifts :many
SELECT s.id::text AS id,s.job_role_id::text AS job_role_id,j.name AS job_role_name,s.starts_at,s.ends_at,
       coalesce(a.employee_id::text,'')::text AS employee_id,coalesce(e.display_name,'')::text AS employee_name
FROM shifts s JOIN job_roles j ON j.org_id=s.org_id AND j.id=s.job_role_id
LEFT JOIN assignments a ON a.org_id=s.org_id AND a.shift_id=s.id
LEFT JOIN employees e ON e.org_id=a.org_id AND e.id=a.employee_id
WHERE s.org_id=sqlc.arg(org_id)::text::uuid AND s.roster_id=sqlc.arg(roster_id)::text::uuid ORDER BY s.starts_at,s.id;

-- name: RosterShift :one
SELECT s.id::text AS id,s.job_role_id::text AS job_role_id,j.name AS job_role_name,s.starts_at,s.ends_at,
       coalesce(a.employee_id::text,'')::text AS employee_id,coalesce(e.display_name,'')::text AS employee_name
FROM shifts s JOIN job_roles j ON j.org_id=s.org_id AND j.id=s.job_role_id
LEFT JOIN assignments a ON a.org_id=s.org_id AND a.shift_id=s.id
LEFT JOIN employees e ON e.org_id=a.org_id AND e.id=a.employee_id
WHERE s.org_id=sqlc.arg(org_id)::text::uuid AND s.roster_id=sqlc.arg(roster_id)::text::uuid AND s.id=sqlc.arg(id)::text::uuid;

-- name: LockShift :exec
SELECT id FROM shifts WHERE org_id=sqlc.arg(org_id)::text::uuid AND id=sqlc.arg(id)::text::uuid FOR UPDATE;

-- name: SchedulingEmployee :one
SELECT active FROM employees WHERE org_id=sqlc.arg(org_id)::text::uuid AND id=sqlc.arg(id)::text::uuid FOR UPDATE;

-- name: SchedulingRole :one
SELECT active FROM job_roles WHERE org_id=sqlc.arg(org_id)::text::uuid AND id=sqlc.arg(id)::text::uuid FOR SHARE;

-- name: EmployeeQualified :one
SELECT EXISTS(SELECT 1 FROM employee_roles WHERE org_id=sqlc.arg(org_id)::text::uuid
AND employee_id=sqlc.arg(employee_id)::text::uuid AND job_role_id=sqlc.arg(job_role_id)::text::uuid) AS qualified;

-- name: SchedulingHistory :many
SELECT s.id::text AS id,s.starts_at,s.ends_at FROM assignments a JOIN shifts s ON s.org_id=a.org_id AND s.id=a.shift_id
WHERE a.org_id=sqlc.arg(org_id)::text::uuid AND a.employee_id=sqlc.arg(employee_id)::text::uuid
AND s.id<>sqlc.arg(exclude_id)::text::uuid
AND a.during && tstzrange(sqlc.arg(window_start)::timestamptz,sqlc.arg(window_end)::timestamptz,'[)') ORDER BY s.starts_at;

-- name: CreateShift :one
INSERT INTO shifts (org_id,roster_id,job_role_id,starts_at,ends_at)
VALUES (sqlc.arg(org_id)::text::uuid,sqlc.arg(roster_id)::text::uuid,sqlc.arg(job_role_id)::text::uuid,$1,$2) RETURNING id::text;

-- name: UpdateShift :exec
UPDATE shifts SET job_role_id=sqlc.arg(job_role_id)::text::uuid,starts_at=$1,ends_at=$2
WHERE org_id=sqlc.arg(org_id)::text::uuid AND id=sqlc.arg(id)::text::uuid;

-- name: DeleteShift :exec
DELETE FROM shifts WHERE org_id=sqlc.arg(org_id)::text::uuid AND id=sqlc.arg(id)::text::uuid;

-- name: AssignShift :exec
INSERT INTO assignments (org_id,shift_id,employee_id)
VALUES (sqlc.arg(org_id)::text::uuid,sqlc.arg(shift_id)::text::uuid,sqlc.arg(employee_id)::text::uuid)
ON CONFLICT (org_id,shift_id) DO UPDATE SET employee_id=EXCLUDED.employee_id;

-- name: UnassignShift :exec
DELETE FROM assignments WHERE org_id=sqlc.arg(org_id)::text::uuid AND shift_id=sqlc.arg(shift_id)::text::uuid;
