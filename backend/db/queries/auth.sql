-- name: FindLoginUser :one
SELECT id::text AS id, email, display_name, password_hash FROM users WHERE email = $1;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, expires_at)
VALUES ($1, sqlc.arg(user_id)::text::uuid, $2);

-- name: SessionUser :one
SELECT u.id::text AS id, u.email, u.display_name
FROM sessions s JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: UserMemberships :many
SELECT o.id::text AS org_id, o.name, o.timezone, m.role
FROM memberships m JOIN organizations o ON o.id = m.org_id
WHERE m.user_id = sqlc.arg(user_id)::text::uuid
ORDER BY o.name, o.id;

-- name: ConsumeLoginAttempt :one
INSERT INTO login_limits (key_hash, attempts, window_end)
VALUES ($1, 1, now() + interval '15 minutes')
ON CONFLICT (key_hash) DO UPDATE SET
  attempts = CASE WHEN login_limits.window_end <= now() THEN 1 ELSE login_limits.attempts + 1 END,
  window_end = CASE WHEN login_limits.window_end <= now() THEN now() + interval '15 minutes' ELSE login_limits.window_end END
RETURNING attempts;

-- name: OrganizationSummary :one
SELECT (SELECT count(*) FROM employees WHERE org_id = sqlc.arg(org_id)::text::uuid AND active) AS employees,
       (SELECT count(*) FROM locations WHERE org_id = sqlc.arg(org_id)::text::uuid AND active) AS locations,
       (SELECT count(*) FROM job_roles WHERE org_id = sqlc.arg(org_id)::text::uuid) AS job_roles;
