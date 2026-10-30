package scheduling

import (
	"testing"
	"time"
)

func instant(value string) time.Time {
	result, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return result
}
func TestSchedulingRules(t *testing.T) {
	zone := time.FixedZone("Lagos", 3600)
	proposed := Proposed{Interval: Interval{ShiftID: "new", Start: instant("2026-11-02T09:00:00+01:00"), End: instant("2026-11-02T17:00:00+01:00")}, Active: true, Qualified: true}
	for _, tc := range []struct {
		name, start, end string
		policy           Policy
		code             string
	}{
		{"overlap", "2026-11-02T16:00:00+01:00", "2026-11-02T20:00:00+01:00", DefaultPolicy(), "SHIFT_OVERLAP"},
		{"adjacent intervals do not overlap", "2026-11-02T17:00:00+01:00", "2026-11-02T20:00:00+01:00", Policy{}, ""},
		{"adjacent violates configured rest", "2026-11-02T17:00:00+01:00", "2026-11-02T20:00:00+01:00", DefaultPolicy(), "MINIMUM_REST"},
		{"exact rest allowed", "2026-11-01T14:00:00+01:00", "2026-11-01T22:00:00+01:00", DefaultPolicy(), ""},
		{"weekly warning", "2026-11-03T09:00:00+01:00", "2026-11-03T17:00:00+01:00", Policy{WeeklyHours: 15 * time.Hour}, "WEEKLY_HOURS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := Check(proposed, []Interval{{ShiftID: "other", Start: instant(tc.start), End: instant(tc.end)}}, zone, tc.policy)
			if tc.code == "" {
				if len(result) != 0 {
					t.Fatalf("unexpected %+v", result)
				}
				return
			}
			if len(result) != 1 || result[0].Code != tc.code {
				t.Fatalf("got %+v", result)
			}
		})
	}
	invalid := proposed
	invalid.Active = false
	invalid.Qualified = false
	if result := Check(invalid, nil, zone, DefaultPolicy()); len(result) != 2 || !HasHard(result) {
		t.Fatalf("employee/qualification rules: %+v", result)
	}
}
func TestWeeklyClippingAndDST(t *testing.T) {
	zone, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	proposed := Proposed{Interval: Interval{Start: instant("2026-03-08T01:00:00-05:00"), End: instant("2026-03-08T04:00:00-04:00")}, Active: true, Qualified: true}
	if result := Check(proposed, nil, zone, Policy{WeeklyHours: 2 * time.Hour}); len(result) != 0 {
		t.Fatalf("DST added a nonexistent hour: %+v", result)
	}
	proposed.Interval = Interval{Start: instant("2026-11-08T23:00:00-05:00"), End: instant("2026-11-09T02:00:00-05:00")}
	result := Check(proposed, nil, zone, Policy{WeeklyHours: time.Hour})
	if len(result) != 1 || result[0].WeekStart != "2026-11-09" || result[0].Severity != "warning" {
		t.Fatalf("week clipping: %+v", result)
	}
	result = Check(proposed, nil, zone, Policy{WeeklyHours: time.Hour, HardWeeklyLimit: true})
	if !HasHard(result) {
		t.Fatal("hard policy ignored")
	}
}
