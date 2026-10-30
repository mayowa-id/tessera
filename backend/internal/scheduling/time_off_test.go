package scheduling

import (
	"testing"
	"time"
)

func TestTimeOffBoundaries(t *testing.T) {
	instant := func(value string) time.Time {
		v, e := time.Parse(time.RFC3339, value)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	proposed := Proposed{Interval: Interval{ShiftID: "shift", Start: instant("2026-11-01T23:00:00Z"), End: instant("2026-11-02T07:00:00Z")}, EmployeeID: "employee"}
	for _, tc := range []struct {
		name, kind, start, end string
		want                   bool
	}{
		{"overnight leave", "leave", "2026-11-02T00:00:00Z", "2026-11-03T00:00:00Z", true},
		{"unavailable overlap", "unavailable", "2026-11-01T22:00:00Z", "2026-11-02T00:00:00Z", true},
		{"ends at shift start", "leave", "2026-11-01T22:00:00Z", "2026-11-01T23:00:00Z", false},
		{"starts at shift end", "leave", "2026-11-02T07:00:00Z", "2026-11-02T10:00:00Z", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := CheckTimeOff(proposed, []Block{{Interval: Interval{Start: instant(tc.start), End: instant(tc.end)}, Kind: tc.kind}})
			if HasHard(result) != tc.want {
				t.Fatalf("violations %+v", result)
			}
			if tc.want && (result[0].EmployeeID != "employee" || result[0].ShiftID != "shift") {
				t.Fatal("missing conflict owner")
			}
		})
	}
	if len(CheckTimeOff(proposed, nil)) != 0 {
		t.Fatal("absence of availability must be unrestricted")
	}
}
func TestRepeatedDSTHourUsesInstants(t *testing.T) {
	parse := func(s string) time.Time { v, _ := time.Parse(time.RFC3339, s); return v }
	// Both are 01:15 in New York on the fall-back day, but different instants.
	p := Proposed{Interval: Interval{Start: parse("2026-11-01T01:15:00-05:00"), End: parse("2026-11-01T01:45:00-05:00")}}
	b := Block{Interval: Interval{Start: parse("2026-11-01T01:15:00-04:00"), End: parse("2026-11-01T01:45:00-04:00")}, Kind: "leave"}
	if len(CheckTimeOff(p, []Block{b})) != 0 {
		t.Fatal("different occurrences must not overlap")
	}
}
