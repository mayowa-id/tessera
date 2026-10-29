-- +goose Up
CREATE TABLE organizations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  slug text NOT NULL UNIQUE,
  name text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
  timezone text NOT NULL DEFAULT 'Africa/Lagos'
);

CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email text NOT NULL UNIQUE CHECK (email = lower(email)),
  display_name text NOT NULL,
  password_hash text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
  org_id uuid NOT NULL REFERENCES organizations(id),
  user_id uuid NOT NULL REFERENCES users(id),
  role text NOT NULL CHECK (role IN ('admin', 'manager', 'employee')),
  PRIMARY KEY (org_id, user_id)
);
CREATE INDEX memberships_user_idx ON memberships(user_id);

CREATE TABLE locations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id uuid NOT NULL REFERENCES organizations(id),
  name text NOT NULL,
  active boolean NOT NULL DEFAULT true,
  UNIQUE (org_id, id),
  UNIQUE (org_id, name)
);

CREATE TABLE job_roles (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id uuid NOT NULL REFERENCES organizations(id),
  name text NOT NULL,
  UNIQUE (org_id, id),
  UNIQUE (org_id, name)
);

CREATE TABLE employees (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id uuid NOT NULL REFERENCES organizations(id),
  user_id uuid,
  display_name text NOT NULL,
  active boolean NOT NULL DEFAULT true,
  UNIQUE (org_id, id),
  UNIQUE (org_id, user_id),
  FOREIGN KEY (org_id, user_id) REFERENCES memberships(org_id, user_id)
);

CREATE TABLE employee_roles (
  org_id uuid NOT NULL REFERENCES organizations(id),
  employee_id uuid NOT NULL,
  job_role_id uuid NOT NULL,
  PRIMARY KEY (org_id, employee_id, job_role_id),
  FOREIGN KEY (org_id, employee_id) REFERENCES employees(org_id, id),
  FOREIGN KEY (org_id, job_role_id) REFERENCES job_roles(org_id, id)
);

CREATE TABLE sessions (
  token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_expiry_idx ON sessions(expires_at);

-- Database-backed throttling works across independent Vercel instances.
CREATE TABLE login_limits (
  key_hash bytea PRIMARY KEY,
  attempts integer NOT NULL,
  window_end timestamptz NOT NULL
);

-- +goose Down
DROP TABLE login_limits, sessions, employee_roles, employees, job_roles, locations, memberships, users, organizations;
