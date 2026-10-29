-- name: DatabaseTime :one
SELECT NOW()::timestamptz AS database_time;
