package scheduling

import (
	"strconv"
	"time"
	_ "time/tzdata"
)

type Interval struct {
	ShiftID    string
	Start, End time.Time
}
type Proposed struct {
	Interval
	EmployeeID        string
	Active, Qualified bool
}
type Policy struct {
	MinimumRest, WeeklyHours time.Duration
	HardWeeklyLimit          bool
}

func DefaultPolicy() Policy { return Policy{MinimumRest: 11 * time.Hour, WeeklyHours: 40 * time.Hour} }

type Violation struct {
	Code           string `json:"code"`
	Severity       string `json:"severity"`
	Message        string `json:"message"`
	RelatedShiftID string `json:"related_shift_id,omitempty"`
	WeekStart      string `json:"week_start,omitempty"`
}

func HasHard(violations []Violation) bool {
	for _, v := range violations {
		if v.Severity == "hard" {
			return true
		}
	}
	return false
}

func WeekStart(instant time.Time, zone *time.Location) time.Time {
	local := instant.In(zone)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	days := (int(midnight.Weekday()) + 6) % 7
	return midnight.AddDate(0, 0, -days)
}
func Window(proposed Interval, zone *time.Location, policy Policy) (time.Time, time.Time) {
	return WeekStart(proposed.Start, zone).Add(-policy.MinimumRest), WeekStart(proposed.End.Add(-time.Nanosecond), zone).AddDate(0, 0, 7).Add(policy.MinimumRest)
}
func Check(proposed Proposed, existing []Interval, zone *time.Location, policy Policy) []Violation {
	violations := []Violation{}
	if !proposed.Active {
		violations = append(violations, Violation{Code: "EMPLOYEE_INACTIVE", Severity: "hard", Message: "The employee is inactive."})
	}
	if !proposed.Qualified {
		violations = append(violations, Violation{Code: "QUALIFICATION_REQUIRED", Severity: "hard", Message: "The employee does not hold the required job qualification."})
	}
	for _, other := range existing {
		if other.ShiftID == proposed.ShiftID {
			continue
		}
		if proposed.Start.Before(other.End) && other.Start.Before(proposed.End) {
			violations = append(violations, Violation{Code: "SHIFT_OVERLAP", Severity: "hard", Message: "The employee already has an overlapping shift.", RelatedShiftID: other.ShiftID})
			continue
		}
		gap := proposed.Start.Sub(other.End)
		if !proposed.Start.After(other.Start) {
			gap = other.Start.Sub(proposed.End)
		}
		if gap >= 0 && gap < policy.MinimumRest {
			violations = append(violations, Violation{Code: "MINIMUM_REST", Severity: "hard", Message: "The employee needs at least " + strconv.FormatFloat(policy.MinimumRest.Hours(), 'f', -1, 64) + " hours between shifts.", RelatedShiftID: other.ShiftID})
		}
	}
	if policy.WeeklyHours > 0 {
		for week := WeekStart(proposed.Start, zone); week.Before(proposed.End); week = week.AddDate(0, 0, 7) {
			end := week.AddDate(0, 0, 7)
			total := clipped(proposed.Interval, week, end)
			for _, other := range existing {
				if other.ShiftID != proposed.ShiftID {
					total += clipped(other, week, end)
				}
			}
			if total > policy.WeeklyHours {
				severity := "warning"
				if policy.HardWeeklyLimit {
					severity = "hard"
				}
				violations = append(violations, Violation{Code: "WEEKLY_HOURS", Severity: severity, Message: "The assignment exceeds the weekly hours policy of " + strconv.FormatFloat(policy.WeeklyHours.Hours(), 'f', -1, 64) + " hours.", WeekStart: week.Format(time.DateOnly)})
			}
		}
	}
	return violations
}
func clipped(interval Interval, start, end time.Time) time.Duration {
	if interval.Start.After(start) {
		start = interval.Start
	}
	if interval.End.Before(end) {
		end = interval.End
	}
	if !end.After(start) {
		return 0
	}
	return end.Sub(start)
}
