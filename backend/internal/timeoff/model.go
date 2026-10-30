package timeoff

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound  = errors.New("time off or employee not found")
	ErrForbidden = errors.New("manager access is required")
	ErrInvalid   = errors.New("invalid time off")
	ErrState     = errors.New("this request cannot make that transition")
)

type Record struct {
	ID           string    `json:"id"`
	EmployeeID   string    `json:"employee_id"`
	EmployeeName string    `json:"employee_name"`
	UserID       string    `json:"-"`
	Kind         string    `json:"kind"`
	Status       string    `json:"status"`
	StartsAt     time.Time `json:"starts_at"`
	EndsAt       time.Time `json:"ends_at"`
	Reason       string    `json:"reason"`
	DecisionNote string    `json:"decision_note"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type Input struct {
	Kind   string    `json:"kind"`
	Start  time.Time `json:"starts_at"`
	End    time.Time `json:"ends_at"`
	Reason string    `json:"reason"`
}
type Filter struct {
	From, To           time.Time
	Status, EmployeeID string
	Limit, Offset      int32
}
type ShiftConflict struct {
	ID           string    `json:"id"`
	RosterID     string    `json:"roster_id"`
	State        string    `json:"state"`
	WeekStart    string    `json:"week_start"`
	LocationName string    `json:"location_name"`
	StartsAt     time.Time `json:"starts_at"`
	EndsAt       time.Time `json:"ends_at"`
}
type Conflict struct{ Shifts []ShiftConflict }

func (c *Conflict) Error() string {
	if len(c.Shifts) == 0 {
		return "This time conflicts with an assigned shift. Ask your manager to review it before recording unavailable time."
	}
	return "This time overlaps assigned shifts. Resolve the assignments before blocking this time. Published schedules cannot be edited in this release."
}

type Store interface {
	List(context.Context, string, string, bool, Filter) ([]Record, error)
	Create(context.Context, string, string, string, Input) (Record, error)
	Decide(context.Context, string, string, string, string, string) (Record, error)
}
