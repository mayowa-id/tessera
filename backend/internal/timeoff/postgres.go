package timeoff

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"tessera/backend/internal/audit"
	"tessera/backend/internal/db"
)

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool} }
func stamp(t time.Time) pgtype.Timestamptz               { return pgtype.Timestamptz{Time: t, Valid: true} }
func manager(role string) bool                           { return role == "admin" || role == "manager" }
func record(id, employee, name, user, kind, status string, start, end pgtype.Timestamptz, reason, note string, created, updated pgtype.Timestamptz) Record {
	return Record{id, employee, name, user, kind, status, start.Time, end.Time, reason, note, created.Time, updated.Time}
}
func get(ctx context.Context, q *db.Queries, org, id string) (Record, error) {
	r, e := q.GetTimeOff(ctx, db.GetTimeOffParams{OrgID: org, ID: id})
	return record(r.ID, r.EmployeeID, r.EmployeeName, r.UserID, r.Kind, r.Status, r.StartsAt, r.EndsAt, r.Reason, r.DecisionNote, r.CreatedAt, r.UpdatedAt), e
}
func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23503" || pgErr.Code == "23514") {
		return ErrInvalid
	}
	return err
}
func (s *PostgresStore) List(ctx context.Context, org, user string, team bool, f Filter) ([]Record, error) {
	q := db.New(s.pool)
	actor, err := q.TimeOffActor(ctx, db.TimeOffActorParams{OrgID: org, UserID: user})
	if err != nil {
		return nil, mapError(err)
	}
	owner := user
	if team {
		if !manager(actor.Role) {
			return nil, ErrForbidden
		}
		owner = ""
	}
	rows, err := q.PageTimeOff(ctx, db.PageTimeOffParams{OrgID: org, OwnerID: owner, EmployeeID: f.EmployeeID, StatusFilter: f.Status, FromTime: stamp(f.From), ToTime: stamp(f.To), PageLimit: f.Limit + 1, PageOffset: f.Offset})
	result := []Record{}
	for _, r := range rows {
		result = append(result, record(r.ID, r.EmployeeID, r.EmployeeName, r.UserID, r.Kind, r.Status, r.StartsAt, r.EndsAt, r.Reason, r.DecisionNote, r.CreatedAt, r.UpdatedAt))
	}
	return result, mapError(err)
}
func conflicts(ctx context.Context, q *db.Queries, org, employee string, start, end time.Time) error {
	rows, err := q.TimeOffAssignmentConflicts(ctx, db.TimeOffAssignmentConflictsParams{OrgID: org, EmployeeID: employee, FromTime: stamp(start), ToTime: stamp(end)})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	shifts := []ShiftConflict{}
	for _, r := range rows {
		shifts = append(shifts, ShiftConflict{r.ID, r.RosterID, r.State, r.WeekStart.Time.Format(time.DateOnly), r.LocationName, r.StartsAt.Time, r.EndsAt.Time})
	}
	return &Conflict{shifts}
}

// Reasons and decision notes are visible only in the time-off screen, not the activity log.
func snapshot(r Record) any {
	return map[string]any{"id": r.ID, "employee_id": r.EmployeeID, "kind": r.Kind, "status": r.Status, "starts_at": r.StartsAt, "ends_at": r.EndsAt}
}
func (s *PostgresStore) Create(ctx context.Context, org, user, employee string, in Input) (Record, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if (in.Kind != "leave" && in.Kind != "unavailable") || !in.End.After(in.Start) || in.End.Sub(in.Start) > 366*24*time.Hour || !in.End.After(time.Now()) || utf8.RuneCountInString(in.Reason) > 500 {
		return Record{}, ErrInvalid
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	if err = q.SerializeScheduling(ctx, org); err != nil {
		return Record{}, err
	}
	actor, err := q.TimeOffActor(ctx, db.TimeOffActorParams{OrgID: org, UserID: user})
	if err != nil {
		return Record{}, mapError(err)
	}
	if employee != "" && !manager(actor.Role) {
		return Record{}, ErrForbidden
	}
	if employee == "" {
		linked, e := q.LinkedEmployee(ctx, db.LinkedEmployeeParams{OrgID: org, UserID: user})
		if e != nil {
			return Record{}, mapError(e)
		}
		employee = linked.ID
	}
	active, err := q.SchedulingEmployee(ctx, db.SchedulingEmployeeParams{OrgID: org, ID: employee})
	if err != nil {
		return Record{}, mapError(err)
	}
	if !active {
		return Record{}, ErrInvalid
	}
	status := "pending"
	action := "leave.requested"
	if in.Kind == "unavailable" {
		status = "approved"
		action = "availability.added"
		if err = conflicts(ctx, q, org, employee, in.Start, in.End); err != nil {
			var conflict *Conflict
			if errors.As(err, &conflict) && !manager(actor.Role) {
				// Personal availability must not reveal unpublished shift details.
				return Record{}, &Conflict{}
			}
			return Record{}, err
		}
	}
	id, err := q.CreateTimeOff(ctx, db.CreateTimeOffParams{OrgID: org, EmployeeID: employee, Kind: in.Kind, Status: status, StartsAt: stamp(in.Start), EndsAt: stamp(in.End), Reason: in.Reason, CreatedBy: user})
	if err != nil {
		return Record{}, mapError(err)
	}
	after, err := get(ctx, q, org, id)
	if err != nil {
		return Record{}, mapError(err)
	}
	if err = audit.Record(ctx, q, org, "time_off", id, after.EmployeeName, action, nil, snapshot(after)); err != nil {
		return Record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Record{}, mapError(err)
	}
	return after, nil
}
func (s *PostgresStore) Decide(ctx context.Context, org, user, id, action, note string) (Record, error) {
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > 500 {
		return Record{}, ErrInvalid
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	if err = q.SerializeScheduling(ctx, org); err != nil {
		return Record{}, err
	}
	actor, err := q.TimeOffActor(ctx, db.TimeOffActorParams{OrgID: org, UserID: user})
	if err != nil {
		return Record{}, mapError(err)
	}
	if action != "cancel" && !manager(actor.Role) {
		return Record{}, ErrForbidden
	}
	before, err := get(ctx, q, org, id)
	if err != nil {
		return Record{}, mapError(err)
	}
	if action == "cancel" && !manager(actor.Role) && before.UserID != user {
		return Record{}, ErrNotFound
	}
	active, err := q.SchedulingEmployee(ctx, db.SchedulingEmployeeParams{OrgID: org, ID: before.EmployeeID})
	if err != nil {
		return Record{}, mapError(err)
	}
	if err = q.LockTimeOff(ctx, db.LockTimeOffParams{OrgID: org, ID: id}); err != nil {
		return Record{}, err
	}
	next := ""
	switch action {
	case "approve", "reject":
		if before.Kind != "leave" || before.Status != "pending" {
			return Record{}, ErrState
		}
		next = "rejected"
		if action == "approve" {
			if !active || !before.EndsAt.After(time.Now()) {
				return Record{}, ErrInvalid
			}
			if err = conflicts(ctx, q, org, before.EmployeeID, before.StartsAt, before.EndsAt); err != nil {
				return Record{}, err
			}
			next = "approved"
		}
	case "cancel":
		if before.Status != "pending" && before.Status != "approved" {
			return Record{}, ErrState
		}
		next = "cancelled"
	default:
		return Record{}, ErrInvalid
	}
	if err = q.DecideTimeOff(ctx, db.DecideTimeOffParams{OrgID: org, ID: id, UserID: user, Status: next, DecisionNote: note}); err != nil {
		return Record{}, mapError(err)
	}
	after, err := get(ctx, q, org, id)
	if err != nil {
		return Record{}, mapError(err)
	}
	auditAction := "leave." + next
	if before.Kind == "unavailable" {
		auditAction = "availability." + next
	}
	if err = audit.Record(ctx, q, org, "time_off", id, before.EmployeeName, auditAction, snapshot(before), snapshot(after)); err != nil {
		return Record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Record{}, mapError(err)
	}
	return after, nil
}
