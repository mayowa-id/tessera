package staff

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Employee{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries.WithTx(tx)
	creating := id == ""
	if !creating {
		if _, err = q.LockEmployee(ctx, db.LockEmployeeParams{OrgID: org, ID: id}); err != nil {
			return Employee{}, storeError(err)
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
	switch kind {
	case "locations":
		if id == "" {
			row, err := s.queries.CreateLocation(ctx, db.CreateLocationParams{OrgID: org, Name: input.Name, Active: input.Active})
			return CatalogItem{row.ID, row.Name, row.Active}, storeError(err)
		}
		row, err := s.queries.UpdateLocation(ctx, db.UpdateLocationParams{OrgID: org, ID: id, Name: input.Name, Active: input.Active})
		return CatalogItem{row.ID, row.Name, row.Active}, storeError(err)
	case "job-roles":
		if id == "" {
			row, err := s.queries.CreateJobRole(ctx, db.CreateJobRoleParams{OrgID: org, Name: input.Name, Active: input.Active})
			return CatalogItem{row.ID, row.Name, row.Active}, storeError(err)
		}
		row, err := s.queries.UpdateJobRole(ctx, db.UpdateJobRoleParams{OrgID: org, ID: id, Name: input.Name, Active: input.Active})
		return CatalogItem{row.ID, row.Name, row.Active}, storeError(err)
	}
	return CatalogItem{}, ErrNotFound
}
