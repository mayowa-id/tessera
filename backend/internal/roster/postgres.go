package roster

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"tessera/backend/internal/audit"
	"tessera/backend/internal/db"
	"tessera/backend/internal/scheduling"
)

type PostgresStore struct {
	pool   *pgxpool.Pool
	policy scheduling.Policy
}

func NewPostgresStore(pool *pgxpool.Pool, policy ...scheduling.Policy) *PostgresStore {
	value := scheduling.DefaultPolicy()
	if len(policy) > 0 {
		value = policy[0]
	}
	return &PostgresStore{pool, value}
}
func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func date(value string) pgtype.Date {
	parsed, _ := time.Parse(time.DateOnly, value)
	return pgtype.Date{Time: parsed, Valid: true}
}
func pointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
func shiftRecord(id, roleID, role string, start, end pgtype.Timestamptz, employeeID, employee string) Shift {
	return Shift{id, roleID, role, start.Time, end.Time, pointer(employeeID), pointer(employee)}
}
func loadShift(ctx context.Context, q *db.Queries, org, rosterID, id string) (Shift, error) {
	row, err := q.RosterShift(ctx, db.RosterShiftParams{OrgID: org, RosterID: rosterID, ID: id})
	return shiftRecord(row.ID, row.JobRoleID, row.JobRoleName, row.StartsAt, row.EndsAt, row.EmployeeID, row.EmployeeName), err
}
func loadRoster(ctx context.Context, q *db.Queries, org, id string) (Roster, error) {
	row, err := q.GetRoster(ctx, db.GetRosterParams{OrgID: org, ID: id})
	return Roster{row.ID, row.LocationID, row.LocationName, row.WeekStart.Time.Format(time.DateOnly), row.Timezone, row.State}, err
}
func (s *PostgresStore) List(ctx context.Context, org, week string, limit, offset int32) ([]Roster, error) {
	rows, err := db.New(s.pool).PageRosters(ctx, db.PageRostersParams{OrgID: org, WeekStart: date(week), PageLimit: limit + 1, PageOffset: offset})
	result := []Roster{}
	for _, row := range rows {
		result = append(result, Roster{row.ID, row.LocationID, row.LocationName, row.WeekStart.Time.Format(time.DateOnly), row.Timezone, row.State})
	}
	return result, mapError(err)
}
func (s *PostgresStore) Create(ctx context.Context, org, location, week string) (Roster, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Roster{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	if err = q.SerializeScheduling(ctx, org); err != nil {
		return Roster{}, mapError(err)
	}
	site, err := q.RosterLocation(ctx, db.RosterLocationParams{OrgID: org, ID: location})
	if err != nil {
		return Roster{}, mapError(err)
	}
	zone, err := time.LoadLocation(site.Timezone)
	if err != nil {
		return Roster{}, ErrInvalid
	}
	start, err := time.ParseInLocation(time.DateOnly, week, zone)
	if err != nil || start.Weekday() != time.Monday {
		return Roster{}, ErrInvalid
	}
	id, err := q.CreateRoster(ctx, db.CreateRosterParams{OrgID: org, LocationID: location, WeekStart: date(week), Timezone: site.Timezone})
	if err != nil {
		return Roster{}, mapError(err)
	}
	result, err := loadRoster(ctx, q, org, id)
	if err != nil {
		return Roster{}, mapError(err)
	}
	if err = audit.Record(ctx, q, org, "roster", id, result.LocationName, "roster.created", nil, result); err != nil {
		return Roster{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Roster{}, mapError(err)
	}
	return result, nil
}
func (s *PostgresStore) Get(ctx context.Context, org, id string) (Detail, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Detail{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	roster, err := loadRoster(ctx, q, org, id)
	if err != nil {
		return Detail{}, mapError(err)
	}
	rows, err := q.RosterShifts(ctx, db.RosterShiftsParams{OrgID: org, RosterID: id})
	if err != nil {
		return Detail{}, mapError(err)
	}
	shifts := []Shift{}
	for _, row := range rows {
		shifts = append(shifts, shiftRecord(row.ID, row.JobRoleID, row.JobRoleName, row.StartsAt, row.EndsAt, row.EmployeeID, row.EmployeeName))
	}
	if err = tx.Commit(ctx); err != nil {
		return Detail{}, mapError(err)
	}
	return Detail{roster, shifts}, nil
}
func (s *PostgresStore) Change(ctx context.Context, org, rosterID string, command Command) (Result, error) {
	for attempt := 0; attempt < 3; attempt++ {
		result, err := s.change(ctx, org, rosterID, command)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "40P01" || pgErr.Code == "40001") {
			continue
		}
		return result, mapError(err)
	}
	return Result{}, &Conflict{[]scheduling.Violation{{Code: "CONCURRENT_CHANGE", Severity: "hard", Message: "The schedule changed concurrently. Please try again."}}}
}
func (s *PostgresStore) change(ctx context.Context, org, rosterID string, command Command) (Result, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	// All scheduling writers take this lock before discovering affected employees.
	if err = q.SerializeScheduling(ctx, org); err != nil {
		return Result{}, err
	}
	roster, err := loadRoster(ctx, q, org, rosterID)
	if err != nil {
		return Result{}, err
	}
	var current Shift
	if command.Action != "create" {
		current, err = loadShift(ctx, q, org, rosterID, command.ShiftID)
		if err != nil {
			return Result{}, err
		}
	}
	employeeIDs := []string{}
	if current.EmployeeID != nil {
		employeeIDs = append(employeeIDs, *current.EmployeeID)
	}
	if command.Action == "assign" && command.EmployeeID != "" && (current.EmployeeID == nil || command.EmployeeID != *current.EmployeeID) {
		employeeIDs = append(employeeIDs, command.EmployeeID)
	}
	sort.Strings(employeeIDs)
	employees := map[string]bool{}
	for _, id := range employeeIDs {
		active, err := q.SchedulingEmployee(ctx, db.SchedulingEmployeeParams{OrgID: org, ID: id})
		if err != nil {
			return Result{}, err
		}
		employees[id] = active
	}
	state, err := q.LockRoster(ctx, db.LockRosterParams{OrgID: org, ID: rosterID})
	if err != nil {
		return Result{}, err
	}
	if state != "draft" {
		return Result{}, ErrDraft
	}
	if command.Action != "create" {
		if err = q.LockShift(ctx, db.LockShiftParams{OrgID: org, ID: command.ShiftID}); err != nil {
			return Result{}, err
		}
	}
	violations := []scheduling.Violation{}
	switch command.Action {
	case "delete":
		err = q.DeleteShift(ctx, db.DeleteShiftParams{OrgID: org, ID: command.ShiftID})
	case "unassign":
		err = q.UnassignShift(ctx, db.UnassignShiftParams{OrgID: org, ShiftID: command.ShiftID})
	case "create", "update", "assign":
		input := command.Input
		employeeID := ""
		if current.EmployeeID != nil {
			employeeID = *current.EmployeeID
		}
		if command.Action == "assign" {
			input = ShiftInput{current.JobRoleID, current.StartsAt, current.EndsAt}
			employeeID = command.EmployeeID
		}
		zone, zoneErr := time.LoadLocation(roster.Timezone)
		if zoneErr != nil {
			return Result{}, ErrInvalid
		}
		week, parseErr := time.ParseInLocation(time.DateOnly, roster.WeekStart, zone)
		if parseErr != nil || !input.End.After(input.Start) || input.End.Sub(input.Start) > 24*time.Hour || input.Start.Before(week) || !input.Start.Before(week.AddDate(0, 0, 7)) {
			return Result{}, ErrInvalid
		}
		active, roleErr := q.SchedulingRole(ctx, db.SchedulingRoleParams{OrgID: org, ID: input.JobRoleID})
		if roleErr != nil {
			return Result{}, roleErr
		}
		if !active {
			return Result{}, ErrInvalid
		}
		if employeeID != "" {
			qualified, qualErr := q.EmployeeQualified(ctx, db.EmployeeQualifiedParams{OrgID: org, EmployeeID: employeeID, JobRoleID: input.JobRoleID})
			if qualErr != nil {
				return Result{}, qualErr
			}
			proposed := scheduling.Proposed{Interval: scheduling.Interval{ShiftID: command.ShiftID, Start: input.Start, End: input.End}, EmployeeID: employeeID, Active: employees[employeeID], Qualified: qualified}
			from, to := scheduling.Window(proposed.Interval, zone, s.policy)
			history, historyErr := q.SchedulingHistory(ctx, db.SchedulingHistoryParams{OrgID: org, EmployeeID: employeeID, ExcludeID: command.ShiftID, WindowStart: timestamp(from), WindowEnd: timestamp(to)})
			if historyErr != nil {
				return Result{}, historyErr
			}
			intervals := []scheduling.Interval{}
			for _, row := range history {
				intervals = append(intervals, scheduling.Interval{ShiftID: row.ID, Start: row.StartsAt.Time, End: row.EndsAt.Time})
			}
			violations = scheduling.Check(proposed, intervals, zone, s.policy)
			blocked, blockErr := timeOffViolations(ctx, q, org, proposed)
			if blockErr != nil {
				return Result{}, blockErr
			}
			violations = append(violations, blocked...)
			if scheduling.HasHard(violations) {
				return Result{}, &Conflict{violations}
			}
		}
		if command.Action == "create" {
			command.ShiftID, err = q.CreateShift(ctx, db.CreateShiftParams{OrgID: org, RosterID: rosterID, JobRoleID: input.JobRoleID, StartsAt: timestamp(input.Start), EndsAt: timestamp(input.End)})
		} else if command.Action == "update" {
			err = q.UpdateShift(ctx, db.UpdateShiftParams{OrgID: org, ID: command.ShiftID, JobRoleID: input.JobRoleID, StartsAt: timestamp(input.Start), EndsAt: timestamp(input.End)})
		} else {
			err = q.AssignShift(ctx, db.AssignShiftParams{OrgID: org, ShiftID: command.ShiftID, EmployeeID: employeeID})
		}
	default:
		return Result{}, ErrInvalid
	}
	if err != nil {
		return Result{}, err
	}
	var result Shift
	if command.Action != "delete" {
		result, err = loadShift(ctx, q, org, rosterID, command.ShiftID)
		if err != nil {
			return Result{}, err
		}
	}
	var before, after any
	if command.Action != "create" {
		before = current
	}
	if command.Action != "delete" {
		after = result
	}
	label := result.JobRoleName
	if label == "" {
		label = current.JobRoleName
	}
	actions := map[string]string{"create": "shift.created", "update": "shift.updated", "assign": "shift.assigned", "unassign": "shift.unassigned", "delete": "shift.deleted"}
	if err = audit.Record(ctx, q, org, "shift", command.ShiftID, label, actions[command.Action], before, after); err != nil {
		return Result{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return Result{result, violations}, nil
}
func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrDuplicate
		case "23503", "23514":
			return ErrInvalid
		case "23P01":
			return &Conflict{[]scheduling.Violation{{Code: "SHIFT_OVERLAP", Severity: "hard", Message: "The employee already has an overlapping shift."}}}
		}
	}
	return err
}
