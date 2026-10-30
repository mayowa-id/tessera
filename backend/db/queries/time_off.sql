-- name: LinkedEmployee :one
SELECT id::text AS id, display_name, active FROM employees
WHERE org_id=sqlc.arg(org_id)::text::uuid AND user_id=sqlc.arg(user_id)::text::uuid;

-- name: TimeOffActor :one
SELECT m.role, o.timezone FROM memberships m JOIN organizations o ON o.id=m.org_id
WHERE m.org_id=sqlc.arg(org_id)::text::uuid AND m.user_id=sqlc.arg(user_id)::text::uuid;

-- name: CreateTimeOff :one
INSERT INTO time_off(org_id,employee_id,kind,status,starts_at,ends_at,reason,created_by)
VALUES(sqlc.arg(org_id)::text::uuid,sqlc.arg(employee_id)::text::uuid,$1,$2,$3,$4,$5,sqlc.arg(created_by)::text::uuid)
RETURNING id::text;

-- name: GetTimeOff :one
SELECT t.id::text AS id,t.employee_id::text AS employee_id,e.display_name AS employee_name,
       coalesce(e.user_id::text,'')::text AS user_id,t.kind,t.status,t.starts_at,t.ends_at,t.reason,t.decision_note,t.created_at,t.updated_at
FROM time_off t JOIN employees e ON e.org_id=t.org_id AND e.id=t.employee_id
WHERE t.org_id=sqlc.arg(org_id)::text::uuid AND t.id=sqlc.arg(id)::text::uuid;

-- name: LockTimeOff :exec
SELECT id FROM time_off WHERE org_id=sqlc.arg(org_id)::text::uuid AND id=sqlc.arg(id)::text::uuid FOR UPDATE;

-- name: DecideTimeOff :exec
UPDATE time_off SET status=$1,decision_note=$2,reviewed_by=sqlc.arg(user_id)::text::uuid,updated_at=now()
WHERE org_id=sqlc.arg(org_id)::text::uuid AND id=sqlc.arg(id)::text::uuid;

-- name: PageTimeOff :many
SELECT t.id::text AS id,t.employee_id::text AS employee_id,e.display_name AS employee_name,
       coalesce(e.user_id::text,'')::text AS user_id,t.kind,t.status,t.starts_at,t.ends_at,t.reason,t.decision_note,t.created_at,t.updated_at
FROM time_off t JOIN employees e ON e.org_id=t.org_id AND e.id=t.employee_id
WHERE t.org_id=sqlc.arg(org_id)::text::uuid
AND (sqlc.arg(owner_id)::text='' OR e.user_id::text=sqlc.arg(owner_id)::text)
AND (sqlc.arg(employee_id)::text='' OR t.employee_id::text=sqlc.arg(employee_id)::text)
AND (sqlc.arg(status_filter)::text='' OR t.status=sqlc.arg(status_filter)::text)
AND t.ends_at>sqlc.arg(from_time)::timestamptz AND t.starts_at<sqlc.arg(to_time)::timestamptz
ORDER BY t.starts_at,t.id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: TimeOffAssignmentConflicts :many
SELECT s.id::text AS id,r.id::text AS roster_id,r.state,r.week_start,l.name AS location_name,s.starts_at,s.ends_at
FROM assignments a JOIN shifts s ON s.org_id=a.org_id AND s.id=a.shift_id
JOIN rosters r ON r.org_id=s.org_id AND r.id=s.roster_id
JOIN locations l ON l.org_id=r.org_id AND l.id=r.location_id
WHERE a.org_id=sqlc.arg(org_id)::text::uuid AND a.employee_id=sqlc.arg(employee_id)::text::uuid
AND a.during && tstzrange(sqlc.arg(from_time)::timestamptz,sqlc.arg(to_time)::timestamptz,'[)')
ORDER BY s.starts_at,s.id LIMIT 51;

-- name: SchedulingTimeOff :many
SELECT kind,starts_at,ends_at FROM time_off
WHERE org_id=sqlc.arg(org_id)::text::uuid AND employee_id=sqlc.arg(employee_id)::text::uuid AND status='approved'
AND ends_at>sqlc.arg(from_time)::timestamptz AND starts_at<sqlc.arg(to_time)::timestamptz;
