-- +goose Up
CREATE TABLE time_off (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id uuid NOT NULL REFERENCES organizations(id),
  employee_id uuid NOT NULL,
  kind text NOT NULL CHECK (kind IN ('unavailable', 'leave')),
  status text NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
  starts_at timestamptz NOT NULL,
  ends_at timestamptz NOT NULL,
  reason text NOT NULL DEFAULT '' CHECK (length(reason) <= 500),
  decision_note text NOT NULL DEFAULT '' CHECK (length(decision_note) <= 500),
  created_by uuid NOT NULL REFERENCES users(id),
  reviewed_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, id),
  FOREIGN KEY (org_id, employee_id) REFERENCES employees(org_id, id),
  CHECK (ends_at > starts_at AND ends_at - starts_at <= interval '366 days'),
  CHECK (kind = 'leave' OR status IN ('approved', 'cancelled'))
);
CREATE INDEX time_off_org_range ON time_off(org_id, starts_at, id);
CREATE INDEX time_off_employee_range ON time_off(org_id, employee_id, starts_at);
CREATE INDEX time_off_approved_range ON time_off USING gist(org_id, employee_id, tstzrange(starts_at, ends_at, '[)')) WHERE status = 'approved';

-- +goose Down
DROP TABLE time_off;
