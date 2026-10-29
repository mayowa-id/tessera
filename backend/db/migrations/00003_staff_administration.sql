-- +goose Up
ALTER TABLE job_roles ADD COLUMN active boolean NOT NULL DEFAULT true;
ALTER TABLE employees ADD CONSTRAINT employees_name_length CHECK (char_length(display_name) BETWEEN 1 AND 120);
ALTER TABLE locations ADD CONSTRAINT locations_name_length CHECK (char_length(name) BETWEEN 1 AND 120);
ALTER TABLE job_roles ADD CONSTRAINT job_roles_name_length CHECK (char_length(name) BETWEEN 1 AND 120);
CREATE UNIQUE INDEX locations_name_normalized ON locations (org_id, lower(name));
CREATE UNIQUE INDEX job_roles_name_normalized ON job_roles (org_id, lower(name));

-- +goose Down
DROP INDEX job_roles_name_normalized, locations_name_normalized;
ALTER TABLE job_roles DROP CONSTRAINT job_roles_name_length;
ALTER TABLE locations DROP CONSTRAINT locations_name_length;
ALTER TABLE employees DROP CONSTRAINT employees_name_length;
ALTER TABLE job_roles DROP COLUMN active;
