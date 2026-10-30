package staff

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"tessera/backend/internal/audit"
	"tessera/backend/internal/db"
)

type PostgresStore struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool, queries: db.New(pool)}
}
func storeError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" {
			return ErrNameInUse
		}
		if pgErr.Code == "23503" {
			return ErrRole
		}
	}
	return err
}
func (s *PostgresStore) Employees(ctx context.Context, org string, page Page) ([]Employee, error) {
	rows, err := s.queries.PageEmployees(ctx, db.PageEmployeesParams{OrgID: org, Search: page.Search, IncludeInactive: page.IncludeInactive, PageLimit: page.Limit + 1, PageOffset: page.Offset})
	result := make([]Employee, 0, len(rows))
	for _, row := range rows {
		result = append(result, Employee{row.ID, row.DisplayName, row.Active, row.JobRoleIds})
	}
	return result, storeError(err)
}
func employee(ctx context.Context, q *db.Queries, org, id string) (Employee, error) {
	row, err := q.GetEmployee(ctx, db.GetEmployeeParams{OrgID: org, ID: id})
	return Employee{row.ID, row.DisplayName, row.Active, row.JobRoleIds}, storeError(err)
}
func (s *PostgresStore) Employee(ctx context.Context, org, id string) (Employee, error) {
	return employee(ctx, s.queries, org, id)
}

func (s *PostgresStore) SaveEmployee(ctx context.Context, org, id string, input EmployeeInput) (Employee, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Employee{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries.WithTx(tx)
	creating := id == ""
	var previous any
	if !creating {
		if _, err = q.LockEmployee(ctx, db.LockEmployeeParams{OrgID: org, ID: id}); err != nil {
			return Employee{}, storeError(err)
		}
		before, err := employee(ctx, q, org, id)
		if err != nil {
			return Employee{}, err
		}
		previous = before
		required, err := q.PublishedEmployeeRequirements(ctx, db.PublishedEmployeeRequirementsParams{OrgID: org, EmployeeID: id})
		if err != nil {
			return Employee{}, storeError(err)
		}
		roles := map[string]bool{}
		for _, roleID := range input.JobRoleIDs {
			roles[roleID] = true
		}
		for _, roleID := range required {
			if !input.Active || !roles[roleID] {
				return Employee{}, ErrPublishedAssignment
			}
		}
	}
	// Use a placeholder UUID for new records; only existing records can retain inactive roles.
	qualificationEmployee := id
	if creating {
		qualificationEmployee = "00000000-0000-0000-0000-000000000000"
	}
	if len(input.JobRoleIDs) > 0 {
		roles, err := q.LockQualificationRoles(ctx, db.LockQualificationRolesParams{OrgID: org, EmployeeID: qualificationEmployee, RoleIds: input.JobRoleIDs})
		if err != nil {
			return Employee{}, storeError(err)
		}
		if len(roles) != len(input.JobRoleIDs) {
			return Employee{}, ErrRole
		}
		for _, role := range roles {
			if !role.Active && (creating || !role.AlreadyAssigned) {
				return Employee{}, ErrRole
			}
		}
	}
	if creating {
		id, err = q.CreateEmployee(ctx, db.CreateEmployeeParams{OrgID: org, DisplayName: input.DisplayName, Active: input.Active})
	} else {
		err = q.UpdateEmployee(ctx, db.UpdateEmployeeParams{OrgID: org, ID: id, DisplayName: input.DisplayName, Active: input.Active})
	}
	if err != nil {
		return Employee{}, storeError(err)
	}
	if err = q.ClearEmployeeRoles(ctx, db.ClearEmployeeRolesParams{OrgID: org, EmployeeID: id}); err != nil {
		return Employee{}, storeError(err)
	}
	for _, roleID := range input.JobRoleIDs {
		if err = q.AddEmployeeRole(ctx, db.AddEmployeeRoleParams{OrgID: org, EmployeeID: id, JobRoleID: roleID}); err != nil {
			return Employee{}, storeError(err)
		}
	}
	result, err := employee(ctx, q, org, id)
	if err != nil {
		return Employee{}, err
	}
	action := "employee.updated"
	if creating {
		action = "employee.created"
	}
	if err = audit.Record(ctx, q, org, "employee", id, result.DisplayName, action, previous, result); err != nil {
		return Employee{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Employee{}, storeError(err)
	}
	return result, nil
}

func (s *PostgresStore) Catalog(ctx context.Context, org, kind string, page Page) ([]CatalogItem, error) {
	result := []CatalogItem{}
	switch kind {
	case "locations":
		rows, err := s.queries.PageLocations(ctx, db.PageLocationsParams{OrgID: org, Search: page.Search, IncludeInactive: page.IncludeInactive, PageLimit: page.Limit + 1, PageOffset: page.Offset})
		if err != nil {
			return nil, storeError(err)
		}
		for _, row := range rows {
			result = append(result, CatalogItem{row.ID, row.Name, row.Active})
		}
	case "job-roles":
		rows, err := s.queries.PageJobRoles(ctx, db.PageJobRolesParams{OrgID: org, Search: page.Search, IncludeInactive: page.IncludeInactive, PageLimit: page.Limit + 1, PageOffset: page.Offset})
		if err != nil {
			return nil, storeError(err)
		}
		for _, row := range rows {
			result = append(result, CatalogItem{row.ID, row.Name, row.Active})
		}
	default:
		return nil, ErrNotFound
	}
	return result, nil
}
func (s *PostgresStore) CatalogItem(ctx context.Context, org, kind, id string) (CatalogItem, error) {
	if kind == "locations" {
		row, err := s.queries.GetLocation(ctx, db.GetLocationParams{OrgID: org, ID: id})
		return CatalogItem{row.ID, row.Name, row.Active}, storeError(err)
	}
	if kind == "job-roles" {
		row, err := s.queries.GetJobRole(ctx, db.GetJobRoleParams{OrgID: org, ID: id})
		return CatalogItem{row.ID, row.Name, row.Active}, storeError(err)
	}
	return CatalogItem{}, ErrNotFound
}
func (s *PostgresStore) SaveCatalogItem(ctx context.Context, org, kind, id string, input CatalogInput) (CatalogItem, error) {
	if kind != "locations" && kind != "job-roles" {
		return CatalogItem{}, ErrNotFound
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return CatalogItem{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	creating := id == ""
	var previous any
	if !creating {
		if kind == "locations" {
			row, err := q.LockStaffLocation(ctx, db.LockStaffLocationParams{OrgID: org, ID: id})
			if err != nil {
				return CatalogItem{}, storeError(err)
			}
			previous = CatalogItem{row.ID, row.Name, row.Active}
		} else {
			row, err := q.LockStaffJobRole(ctx, db.LockStaffJobRoleParams{OrgID: org, ID: id})
			if err != nil {
				return CatalogItem{}, storeError(err)
			}
			previous = CatalogItem{row.ID, row.Name, row.Active}
		}
		if !input.Active {
			linked, err := q.CatalogHasPublishedShifts(ctx, db.CatalogHasPublishedShiftsParams{OrgID: org, Kind: kind, ID: id})
			if err != nil {
				return CatalogItem{}, storeError(err)
			}
			if linked {
				return CatalogItem{}, ErrPublishedAssignment
			}
		}
	}
	var result CatalogItem
	if kind == "locations" {
		if creating {
			row, writeErr := q.CreateLocation(ctx, db.CreateLocationParams{OrgID: org, Name: input.Name, Active: input.Active})
			err = writeErr
			result = CatalogItem{row.ID, row.Name, row.Active}
		} else {
			row, writeErr := q.UpdateLocation(ctx, db.UpdateLocationParams{OrgID: org, ID: id, Name: input.Name, Active: input.Active})
			err = writeErr
			result = CatalogItem{row.ID, row.Name, row.Active}
		}
	} else {
		if creating {
			row, writeErr := q.CreateJobRole(ctx, db.CreateJobRoleParams{OrgID: org, Name: input.Name, Active: input.Active})
			err = writeErr
			result = CatalogItem{row.ID, row.Name, row.Active}
		} else {
			row, writeErr := q.UpdateJobRole(ctx, db.UpdateJobRoleParams{OrgID: org, ID: id, Name: input.Name, Active: input.Active})
			err = writeErr
			result = CatalogItem{row.ID, row.Name, row.Active}
		}
	}
	if err != nil {
		return CatalogItem{}, storeError(err)
	}
	entity := "location"
	if kind == "job-roles" {
		entity = "job_role"
	}
	action := entity + ".updated"
	if creating {
		action = entity + ".created"
	}
	if err = audit.Record(ctx, q, org, entity, result.ID, result.Name, action, previous, result); err != nil {
		return CatalogItem{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CatalogItem{}, storeError(err)
	}
	return result, nil
}
