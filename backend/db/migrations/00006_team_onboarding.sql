-- +goose Up
-- Existing provisioned accounts remain usable; public registrations must verify ownership.
ALTER TABLE users ADD COLUMN email_verified boolean NOT NULL DEFAULT true;
ALTER TABLE users ALTER COLUMN email_verified SET DEFAULT false;
CREATE TABLE auth_tokens (
  token_hash bytea PRIMARY KEY CHECK(octet_length(token_hash)=32),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  purpose text NOT NULL CHECK(purpose IN ('verify','reset')),
  expires_at timestamptz NOT NULL,
  UNIQUE(user_id,purpose)
);
CREATE TABLE onboarding_limits (
  key_hash bytea PRIMARY KEY,
  attempts integer NOT NULL,
  window_end timestamptz NOT NULL
);
CREATE TABLE team_invitations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id uuid NOT NULL REFERENCES organizations(id),
  employee_id uuid NOT NULL,
  email text NOT NULL CHECK(email=lower(email)),
  display_name text NOT NULL,
  role text NOT NULL CHECK(role IN ('manager','employee')),
  token_hash bytea NOT NULL UNIQUE CHECK(octet_length(token_hash)=32),
  invited_by uuid NOT NULL REFERENCES users(id),
  expires_at timestamptz NOT NULL,
  accepted_at timestamptz,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(org_id,email),
  FOREIGN KEY(org_id,employee_id) REFERENCES employees(org_id,id)
);
CREATE INDEX invitations_org_idx ON team_invitations(org_id,created_at DESC);
-- +goose Down
DROP TABLE team_invitations,onboarding_limits,auth_tokens;
ALTER TABLE users DROP COLUMN email_verified;
