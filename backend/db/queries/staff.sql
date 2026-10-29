-- name: PageEmployees :many
SELECT e.id::text AS id, e.display_name, e.active,
       ARRAY(SELECT er.job_role_id::text FROM employee_roles er
             WHERE er.org_id=e.org_id AND er.employee_id=e.id ORDER BY er.job_role_id)::text[] AS job_role_ids
FROM employees e
WHERE e.org_id=sqlc.arg(org_id)::text::uuid
  AND (sqlc.arg(include_inactive)::boolean OR e.active)
  AND strpos(lower(e.display_name),lower(sqlc.arg(search)::text))>0
ORDER BY lower(e.display_name), e.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: GetEmployee :one
SELECT e.id::text AS id, e.display_name, e.active,
       ARRAY(SELECT er.job_role_id::text FROM employee_roles er
             WHERE er.org_id=e.org_id AND er.employee_id=e.id ORDER BY er.job_role_id)::text[] AS job_role_ids
FROM employees e WHERE e.org_id=sqlc.arg(org_id)::text::uuid AND e.id=sqlc.arg(id)::text::uuid;

-- name: LockEmployee :one
SELECT id::text AS id FROM employees
WHERE org_id=sqlc.arg(org_id)::text::uuid AND id=sqlc.arg(id)::text::uuid FOR UPDATE;

-- name: CreateEmployee :one
INSERT INTO employees (org_id,display_name,active) VALUES (sqlc.arg(org_id)::text::uuid,$1,$2) RETURNING id::text;

-- name: UpdateEmployee :exec
UPDATE employees SET display_name=$1,active=$2
WHERE org_id=sqlc.arg(org_id)::text::uuid AND id=sqlc.arg(id)::text::uuid;

-- name: LockQualificationRoles :many
SELECT r.id::text AS id, r.active,
       EXISTS(SELECT 1 FROM employee_roles er WHERE er.org_id=r.org_id AND er.job_role_id=r.id
              AND er.employee_id=sqlc.arg(employee_id)::text::uuid) AS already_assigned
FROM job_roles r
WHERE r.org_id=sqlc.arg(org_id)::text::uuid AND r.id::text=ANY(sqlc.arg(role_ids)::text[])
ORDER BY r.id FOR UPDATE OF r;

-- name: ClearEmployeeRoles :exec
DELETE FROM employee_roles WHERE org_id=sqlc.arg(org_id)::text::uuid AND employee_id=sqlc.arg(employee_id)::text::uuid;

-- name: AddEmployeeRole :exec
INSERT INTO employee_roles (org_id,employee_id,job_role_id)
VALUES (sqlc.arg(org_id)::text::uuid,sqlc.arg(employee_id)::text::uuid,sqlc.arg(job_role_id)::text::uuid);

-- name: PageLocations :many
SELECT id::text AS id,name,active FROM locations
WHERE org_id=sqlc.arg(org_id)::text::uuid AND (sqlc.arg(include_inactive)::boolean OR active)
  AND strpos(lower(name),lower(sqlc.arg(search)::text))>0
ORDER BY lower(name),id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: GetLocation :one
SELECT id::text AS id,name,active FROM locations WHERE org_id=sqlc.arg(org_id)::text::uuid AND id=sqlc.arg(id)::text::uuid;

-- name: CreateLocation :one
INSERT INTO locations (org_id,name,active) VALUES (sqlc.arg(org_id)::text::uuid,$1,$2)
RETURNING id::text,name,active;

-- name: UpdateLocation :one
UPDATE locations SET name=$1,active=$2 WHERE org_id=sqlc.arg(org_id)::text::uuid AND id=sqlc.arg(id)::text::uuid
RETURNING id::text,name,active;

-- name: PageJobRoles :many
SELECT id::text AS id,name,active FROM job_roles
WHERE org_id=sqlc.arg(org_id)::text::uuid AND (sqlc.arg(include_inactive)::boolean OR active)
  AND strpos(lower(name),lower(sqlc.arg(search)::text))>0
ORDER BY lower(name),id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: GetJobRole :one
SELECT id::text AS id,name,active FROM job_roles WHERE org_id=sqlc.arg(org_id)::text::uuid AND id=sqlc.arg(id)::text::uuid;

-- name: CreateJobRole :one
INSERT INTO job_roles (org_id,name,active) VALUES (sqlc.arg(org_id)::text::uuid,$1,$2)
RETURNING id::text,name,active;

-- name: UpdateJobRole :one
UPDATE job_roles SET name=$1,active=$2 WHERE org_id=sqlc.arg(org_id)::text::uuid AND id=sqlc.arg(id)::text::uuid
RETURNING id::text,name,active;
