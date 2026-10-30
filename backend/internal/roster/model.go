package roster

import (
	"context"
	"errors"
	"tessera/backend/internal/scheduling"
	"time"
)

var (
	ErrNotFound  = errors.New("record not found")
	ErrDuplicate = errors.New("roster already exists")
	ErrDraft     = errors.New("only draft rosters can change")
	ErrInvalid   = errors.New("invalid roster or shift")
)

type Conflict struct{ Violations []scheduling.Violation }

func (c *Conflict) Error() string { return "The shift conflicts with scheduling rules." }

type Roster struct {
	ID           string `json:"id"`
	LocationID   string `json:"location_id"`
	LocationName string `json:"location_name"`
	WeekStart    string `json:"week_start"`
	Timezone     string `json:"timezone"`
	State        string `json:"state"`
}
type Shift struct {
	ID           string    `json:"id"`
	JobRoleID    string    `json:"job_role_id"`
	JobRoleName  string    `json:"job_role_name"`
	StartsAt     time.Time `json:"starts_at"`
	EndsAt       time.Time `json:"ends_at"`
	EmployeeID   *string   `json:"employee_id"`
	EmployeeName *string   `json:"employee_name"`
}
type Detail struct {
	Roster Roster  `json:"roster"`
	Shifts []Shift `json:"shifts"`
}
type Result struct {
	Shift      Shift                  `json:"shift"`
	Violations []scheduling.Violation `json:"violations"`
}
type ShiftInput struct {
	JobRoleID  string
	Start, End time.Time
}
type Command struct {
	Action, ShiftID, EmployeeID string
	Input                       ShiftInput
}
type Store interface {
	List(context.Context, string, string, int32, int32) ([]Roster, error)
	Create(context.Context, string, string, string) (Roster, error)
	Get(context.Context, string, string) (Detail, error)
	Change(context.Context, string, string, Command) (Result, error)
}
