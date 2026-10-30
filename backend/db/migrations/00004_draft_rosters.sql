-- +goose Up
CREATE TABLE rosters (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id uuid NOT NULL REFERENCES organizations(id),
  location_id uuid NOT NULL,
  week_start date NOT NULL CHECK (extract(isodow FROM week_start)=1),
  timezone text NOT NULL,
  state text NOT NULL DEFAULT 'draft' CHECK (state IN ('draft','published','archived')),
  UNIQUE (org_id,id),
  UNIQUE (org_id,location_id,week_start),
  FOREIGN KEY (org_id,location_id) REFERENCES locations(org_id,id)
);
CREATE TABLE shifts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id uuid NOT NULL REFERENCES organizations(id),
  roster_id uuid NOT NULL,
  job_role_id uuid NOT NULL,
  starts_at timestamptz NOT NULL,
  ends_at timestamptz NOT NULL,
  during tstzrange GENERATED ALWAYS AS (tstzrange(starts_at,ends_at,'[)')) STORED,
  CHECK (ends_at>starts_at AND ends_at<=starts_at+interval '24 hours'),
  UNIQUE (org_id,id),
  FOREIGN KEY (org_id,roster_id) REFERENCES rosters(org_id,id),
  FOREIGN KEY (org_id,job_role_id) REFERENCES job_roles(org_id,id)
);
CREATE INDEX shifts_roster_idx ON shifts(org_id,roster_id,starts_at);
CREATE TABLE assignments (
  org_id uuid NOT NULL REFERENCES organizations(id),
  shift_id uuid NOT NULL,
  employee_id uuid NOT NULL,
  during tstzrange NOT NULL,
  PRIMARY KEY (org_id,shift_id),
  FOREIGN KEY (org_id,shift_id) REFERENCES shifts(org_id,id) ON DELETE CASCADE,
  FOREIGN KEY (org_id,employee_id) REFERENCES employees(org_id,id),
  EXCLUDE USING gist (org_id WITH =, employee_id WITH =, during WITH &&)
);

-- +goose StatementBegin
CREATE FUNCTION derive_assignment_interval() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  SELECT s.during INTO NEW.during FROM shifts s
  WHERE s.org_id=NEW.org_id AND s.id=NEW.shift_id FOR SHARE;
  IF NOT FOUND THEN RAISE EXCEPTION 'shift not found' USING ERRCODE='23503'; END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER assignment_interval BEFORE INSERT OR UPDATE ON assignments
FOR EACH ROW EXECUTE FUNCTION derive_assignment_interval();

-- +goose StatementBegin
CREATE FUNCTION synchronize_assignment_interval() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  UPDATE assignments SET during=NEW.during WHERE org_id=NEW.org_id AND shift_id=NEW.id;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER shift_interval AFTER UPDATE OF starts_at,ends_at ON shifts
FOR EACH ROW EXECUTE FUNCTION synchronize_assignment_interval();

-- +goose Down
DROP TABLE assignments;
DROP TABLE shifts;
DROP FUNCTION synchronize_assignment_interval();
DROP FUNCTION derive_assignment_interval();
DROP TABLE rosters;
