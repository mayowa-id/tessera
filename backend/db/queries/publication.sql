-- name: SetRosterState :exec
UPDATE rosters SET state=$1 WHERE org_id=sqlc.arg(org_id)::text::uuid AND id=sqlc.arg(id)::text::uuid;

-- name: RecordAudit :exec
INSERT INTO audit_events(org_id,actor_user_id,entity_type,entity_id,entity_label,action,before_data,after_data)
VALUES(sqlc.arg(org_id)::text::uuid,sqlc.arg(actor_user_id)::text::uuid,$1,sqlc.arg(entity_id)::text::uuid,$2,$3,$4,$5);

-- name: PageActivity :many
SELECT a.id::text AS id,a.action,a.entity_type,a.entity_id::text AS entity_id,a.entity_label,
       a.actor_user_id::text AS actor_user_id,u.display_name AS actor_name,a.created_at,a.before_data,a.after_data
FROM audit_events a JOIN users u ON u.id=a.actor_user_id
WHERE a.org_id=sqlc.arg(org_id)::text::uuid ORDER BY a.created_at DESC,a.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: MyPublishedShifts :many
SELECT s.id::text AS id,r.id::text AS roster_id,l.name AS location_name,j.name AS job_role_name,
       s.starts_at,s.ends_at,r.timezone,r.state
FROM assignments a JOIN shifts s ON s.org_id=a.org_id AND s.id=a.shift_id
JOIN rosters r ON r.org_id=s.org_id AND r.id=s.roster_id
JOIN employees e ON e.org_id=a.org_id AND e.id=a.employee_id
JOIN locations l ON l.org_id=r.org_id AND l.id=r.location_id
JOIN job_roles j ON j.org_id=s.org_id AND j.id=s.job_role_id
WHERE a.org_id=sqlc.arg(org_id)::text::uuid AND e.user_id=sqlc.arg(user_id)::text::uuid
AND r.state IN ('published','archived') AND s.ends_at>sqlc.arg(from_time)::timestamptz AND s.starts_at<sqlc.arg(to_time)::timestamptz
ORDER BY s.starts_at,s.id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: PublishedEmployeeRequirements :many
SELECT DISTINCT s.job_role_id::text AS job_role_id
FROM assignments a JOIN shifts s ON s.org_id=a.org_id AND s.id=a.shift_id
JOIN rosters r ON r.org_id=s.org_id AND r.id=s.roster_id
WHERE a.org_id=sqlc.arg(org_id)::text::uuid AND a.employee_id=sqlc.arg(employee_id)::text::uuid
AND r.state='published' AND s.ends_at>now();

-- name: CatalogHasPublishedShifts :one
SELECT EXISTS(SELECT 1 FROM shifts s JOIN rosters r ON r.org_id=s.org_id AND r.id=s.roster_id
WHERE s.org_id=sqlc.arg(org_id)::text::uuid AND r.state='published' AND s.ends_at>now()
AND ((sqlc.arg(kind)::text='locations' AND r.location_id=sqlc.arg(id)::text::uuid)
OR (sqlc.arg(kind)::text='job-roles' AND s.job_role_id=sqlc.arg(id)::text::uuid))) AS has_future;

-- name: LockStaffLocation :one
SELECT id::text AS id,name,active FROM locations WHERE org_id=sqlc.arg(org_id)::text::uuid AND id=sqlc.arg(id)::text::uuid FOR UPDATE;

-- name: LockStaffJobRole :one
SELECT id::text AS id,name,active FROM job_roles WHERE org_id=sqlc.arg(org_id)::text::uuid AND id=sqlc.arg(id)::text::uuid FOR UPDATE;
