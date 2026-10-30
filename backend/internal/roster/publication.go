package roster

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"tessera/backend/internal/audit"
	"tessera/backend/internal/db"
	"tessera/backend/internal/scheduling"
)

func (s *PostgresStore) Transition(ctx context.Context, org, id, action string) (LifecycleResult, error) {
	for attempt := 0; attempt < 3; attempt++ {
		result, err := s.transition(ctx, org, id, action)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "40P01" || pgErr.Code == "40001") {
			continue
		}
		return result, mapError(err)
	}
	return LifecycleResult{}, &Conflict{[]scheduling.Violation{{Code: "CONCURRENT_CHANGE", Severity: "hard", Message: "The roster changed concurrently. Please try again."}}}
}
func loadDetail(ctx context.Context, q *db.Queries, org, id string) (Detail, error) {
	roster, err := loadRoster(ctx, q, org, id)
	if err != nil {
		return Detail{}, err
	}
	rows, err := q.RosterShifts(ctx, db.RosterShiftsParams{OrgID: org, RosterID: id})
	if err != nil {
		return Detail{}, err
	}
	shifts := []Shift{}
	for _, row := range rows {
		shifts = append(shifts, shiftRecord(row.ID, row.JobRoleID, row.JobRoleName, row.StartsAt, row.EndsAt, row.EmployeeID, row.EmployeeName))
	}
	return Detail{roster, shifts}, nil
}
func (s *PostgresStore) transition(ctx context.Context, org, id, action string) (LifecycleResult, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return LifecycleResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	if err = q.SerializeScheduling(ctx, org); err != nil {
		return LifecycleResult{}, err
	}
	before, err := loadDetail(ctx, q, org, id)
	if err != nil {
		return LifecycleResult{}, err
	}
	employees := map[string]bool{}
	employeeIDs := []string{}
	for _, shift := range before.Shifts {
		if shift.EmployeeID != nil {
			if _, exists := employees[*shift.EmployeeID]; !exists {
				employees[*shift.EmployeeID] = false
				employeeIDs = append(employeeIDs, *shift.EmployeeID)
			}
		}
	}
	sort.Strings(employeeIDs)
	for _, employeeID := range employeeIDs {
		active, err := q.SchedulingEmployee(ctx, db.SchedulingEmployeeParams{OrgID: org, ID: employeeID})
		if err != nil {
			return LifecycleResult{}, err
		}
		employees[employeeID] = active
	}
	state, err := q.LockRoster(ctx, db.LockRosterParams{OrgID: org, ID: id})
	if err != nil {
		return LifecycleResult{}, err
	}
	zone, err := time.LoadLocation(before.Roster.Timezone)
	if err != nil {
		return LifecycleResult{}, ErrInvalid
	}
	week, err := time.ParseInLocation(time.DateOnly, before.Roster.WeekStart, zone)
	if err != nil {
		return LifecycleResult{}, ErrInvalid
	}
	violations := []scheduling.Violation{}
	next := ""
	switch action {
	case "publish":
		if state != "draft" {
			return LifecycleResult{}, ErrTransition
		}
		violations, err = s.validatePublication(ctx, q, org, before, employees, zone, week)
		if err != nil {
			return LifecycleResult{}, err
		}
		if scheduling.HasHard(violations) {
			return LifecycleResult{}, &Conflict{violations}
		}
		next = "published"
	case "archive":
		if state != "published" {
			return LifecycleResult{}, ErrTransition
		}
		ended := week.AddDate(0, 0, 7)
		for _, shift := range before.Shifts {
			if shift.EndsAt.After(ended) {
				ended = shift.EndsAt
			}
		}
		if time.Now().Before(ended) {
			return LifecycleResult{}, ErrNotEnded
		}
		next = "archived"
	default:
		return LifecycleResult{}, ErrTransition
	}
	if err = q.SetRosterState(ctx, db.SetRosterStateParams{OrgID: org, ID: id, State: next}); err != nil {
		return LifecycleResult{}, err
	}
	after, err := loadDetail(ctx, q, org, id)
	if err != nil {
		return LifecycleResult{}, err
	}
	if err = audit.Record(ctx, q, org, "roster", id, after.Roster.LocationName, "roster."+next, before.Roster, after); err != nil {
		return LifecycleResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return LifecycleResult{}, err
	}
	return LifecycleResult{after, violations}, nil
}
func (s *PostgresStore) validatePublication(ctx context.Context, q *db.Queries, org string, detail Detail, employees map[string]bool, zone *time.Location, week time.Time) ([]scheduling.Violation, error) {
	violations := []scheduling.Violation{}
	if len(detail.Shifts) == 0 {
		return []scheduling.Violation{{Code: "EMPTY_ROSTER", Severity: "hard", Message: "Add at least one assigned shift before publishing."}}, nil
	}
	if _, err := q.RosterLocation(ctx, db.RosterLocationParams{OrgID: org, ID: detail.Roster.LocationID}); errors.Is(err, pgx.ErrNoRows) {
		violations = append(violations, scheduling.Violation{Code: "LOCATION_INACTIVE", Severity: "hard", Message: "Reactivate this location before publishing."})
	} else if err != nil {
		return nil, err
	}
	first, last := detail.Shifts[0].StartsAt, detail.Shifts[0].EndsAt
	for _, shift := range detail.Shifts {
		if shift.StartsAt.Before(first) {
			first = shift.StartsAt
		}
		if shift.EndsAt.After(last) {
			last = shift.EndsAt
		}
	}
	from, to := scheduling.Window(scheduling.Interval{Start: first, End: last}, zone, s.policy)
	history := map[string][]scheduling.Interval{}
	roles := map[string]bool{}
	qualifications := map[string]bool{}
	for _, shift := range detail.Shifts {
		if shift.StartsAt.Before(week) || !shift.StartsAt.Before(week.AddDate(0, 0, 7)) {
			violations = append(violations, scheduling.Violation{Code: "SHIFT_OUTSIDE_WEEK", Severity: "hard", Message: "A shift starts outside the roster week.", ShiftID: shift.ID})
		}
		roleActive, known := roles[shift.JobRoleID]
		if !known {
			active, err := q.SchedulingRole(ctx, db.SchedulingRoleParams{OrgID: org, ID: shift.JobRoleID})
			if err != nil {
				return nil, err
			}
			roleActive = active
			roles[shift.JobRoleID] = active
		}
		if !roleActive {
			violations = append(violations, scheduling.Violation{Code: "ROLE_INACTIVE", Severity: "hard", Message: "A required job role is inactive.", ShiftID: shift.ID})
		}
		if shift.EmployeeID == nil {
			violations = append(violations, scheduling.Violation{Code: "UNASSIGNED_SHIFT", Severity: "hard", Message: "Assign an employee to every shift before publishing.", ShiftID: shift.ID})
			continue
		}
		employeeID := *shift.EmployeeID
		if _, loaded := history[employeeID]; !loaded {
			rows, err := q.SchedulingHistory(ctx, db.SchedulingHistoryParams{OrgID: org, EmployeeID: employeeID, ExcludeID: "00000000-0000-0000-0000-000000000000", WindowStart: timestamp(from), WindowEnd: timestamp(to)})
			if err != nil {
				return nil, err
			}
			history[employeeID] = []scheduling.Interval{}
			for _, row := range rows {
				history[employeeID] = append(history[employeeID], scheduling.Interval{ShiftID: row.ID, Start: row.StartsAt.Time, End: row.EndsAt.Time})
			}
		}
		key := employeeID + ":" + shift.JobRoleID
		qualified, known := qualifications[key]
		if !known {
			value, err := q.EmployeeQualified(ctx, db.EmployeeQualifiedParams{OrgID: org, EmployeeID: employeeID, JobRoleID: shift.JobRoleID})
			if err != nil {
				return nil, err
			}
			qualified = value
			qualifications[key] = value
		}
		violations = append(violations, scheduling.Check(scheduling.Proposed{Interval: scheduling.Interval{ShiftID: shift.ID, Start: shift.StartsAt, End: shift.EndsAt}, EmployeeID: employeeID, Active: employees[employeeID], Qualified: qualified}, history[employeeID], zone, s.policy)...)
	}
	unique := []scheduling.Violation{}
	seen := map[string]bool{}
	for _, violation := range violations {
		key := violation.Code + ":" + violation.EmployeeID + ":" + violation.ShiftID + ":" + violation.RelatedShiftID
		if violation.Code == "WEEKLY_HOURS" {
			key = violation.Code + ":" + violation.EmployeeID + ":" + violation.WeekStart
		}
		if !seen[key] {
			unique = append(unique, violation)
			seen[key] = true
		}
	}
	return unique, nil
}
func (s *PostgresStore) MyShifts(ctx context.Context, org, user string, from, to time.Time, limit, offset int32) ([]MyShift, error) {
	rows, err := db.New(s.pool).MyPublishedShifts(ctx, db.MyPublishedShiftsParams{OrgID: org, UserID: user, FromTime: timestamp(from), ToTime: timestamp(to), PageLimit: limit + 1, PageOffset: offset})
	result := []MyShift{}
	for _, row := range rows {
		result = append(result, MyShift{row.ID, row.RosterID, row.LocationName, row.JobRoleName, row.StartsAt.Time, row.EndsAt.Time, row.Timezone, row.State})
	}
	return result, mapError(err)
}
